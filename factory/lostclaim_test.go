package factory_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// A claim whose answer is lost (Spec code-central-779): the Factory claims
// again at once, with no tick, so a Ticket Code Central has already put in
// its hands is never left In Progress under a Claim nobody holds.

const claimPath = "/api/p/acme/claim"

func TestAClaimWhoseAnswerIsLostIsClaimedAgainAndRuns(t *testing.T) {
	t.Parallel()
	h := wakingHarness(t)
	id := h.ReadyTicket("Ask first")
	h.Script(id, factorytest.PostRound("Which colour?"), factorytest.Result("success"))
	h.LoseAnswer("/claim", 1)
	r := h.Start()

	h.WaitForState(id, store.NeedsAnswer)
	r.WaitForOutput(id + " turn ended")
	out := r.Stdout()
	for _, want := range []string{id + ": claim: the answer was lost (", "): claiming it again", id + ": claim: the answer came back on claiming it again"} {
		if n := strings.Count(out, want); n != 1 {
			t.Errorf("%q printed %d times, want once; stdout:\n%s", want, n, out)
		}
	}
	if n := len(h.Ticket(id).Claims); n != 1 {
		t.Errorf("%s has %d Claims, want 1: claiming again gets the same Claim", id, n)
	}
	if n := h.Refused(claimPath); n != 1 {
		t.Errorf("%d claims cut, want 1", n)
	}
}

func TestAClaimThatNeverArrivedIsClaimedAgainAndRuns(t *testing.T) {
	t.Parallel()
	h := wakingHarness(t)
	id := h.ReadyTicket("Lost on the way")
	h.LoseRequest("/claim", 1)
	r := h.Start()

	h.WaitForState(id, store.InReview)
	if n := len(h.Ticket(id).Claims); n != 1 {
		t.Errorf("%s has %d Claims, want 1", id, n)
	}
	if !strings.Contains(r.Stdout(), id+": claim: the answer came back on claiming it again") {
		t.Errorf("no recovered line; stdout:\n%s", r.Stdout())
	}
}

// Another session claims the Ticket while the Factory waits to claim it
// again: Code Central's refusal is an answer, so the Factory drops the
// claim and runs no turn.
func TestAClaimAnotherSessionTakesBeforeTheRetryIsDropped(t *testing.T) {
	t.Parallel()
	h := wakingHarness(t)
	id := h.ReadyTicket("Taken elsewhere")
	// Every try but the last is cut before Code Central sees it, so the
	// other session has from the first cut to the last try to claim.
	h.LoseRequest("/claim", 3)
	// Start returns after the first poll, which makes every try, so the
	// other session claims alongside it.
	claimed := make(chan error, 1)
	go func() {
		for h.Refused(claimPath) == 0 {
			time.Sleep(5 * time.Millisecond)
		}
		_, err := h.Server.Client.Claim(context.Background(), h.Project, core.ClaimInput{Ticket: core.Ref(id), SessionID: "elsewhere"})
		claimed <- err
	}()
	r := h.Start()
	if err := <-claimed; err != nil {
		t.Fatal(err)
	}

	r.WaitForOutput("claim: action_not_allowed")
	tk := h.Ticket(id)
	if len(tk.Claims) != 1 || tk.Claims[0].SessionID != "elsewhere" {
		t.Errorf("%s has Claims %+v, want only the other session's", id, tk.Claims)
	}
	if n := len(h.Calls()); n != 0 {
		t.Errorf("claude ran %d times, want none: the claim was refused", n)
	}
	if strings.Contains(r.Stdout(), "the answer came back") {
		t.Errorf("printed a recovered line for a refused claim; stdout:\n%s", r.Stdout())
	}
	if tk.State != store.InProgress {
		t.Errorf("%s is %s, want In Progress under the other session's Claim", id, tk.State)
	}
}

// Every try lost, the Factory sends the claim 4 times in all, the first
// and 3 more, and starts no turn.
func TestAClaimLostOnEveryTryIsSentFourTimes(t *testing.T) {
	t.Parallel()
	h := wakingHarness(t)
	id := h.ReadyTicket("Never through")
	h.LoseRequest("/claim", 5)
	r := h.Start()

	if n := h.Refused(claimPath); n != 4 {
		t.Errorf("%d claims sent, want 4", n)
	}
	if n := strings.Count(r.Stdout(), "the answer was lost"); n != 1 {
		t.Errorf("lost line printed %d times, want once; stdout:\n%s", n, r.Stdout())
	}
	if strings.Contains(r.Stdout(), "the answer came back") || len(h.Calls()) != 0 {
		t.Errorf("a claim lost on every try was recovered; stdout:\n%s", r.Stdout())
	}
	if s := h.Ticket(id).State; s != store.Ready {
		t.Errorf("%s is %s, want Ready: no try reached Code Central", id, s)
	}
}
