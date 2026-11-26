package factory_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"

	"github.com/tomoram/code-central/factory/factorytest"
)

// A claim whose answer is lost (Spec code-central-779): the Factory claims
// again at once, with no tick, so a Ticket Code Central has already put in
// its hands is never left In Progress under a Claim nobody holds.

const claimPath = "/api/p/acme/claim"

func TestAClaimWhoseAnswerIsLostIsClaimedAgainAndRuns(t *testing.T) {
	t.Parallel()
	h := wakingRig(t)
	id := h.ReadyTicket("Ask first")
	h.Script(id, factorytest.PostRound("Which colour?"), factorytest.Result("success"))
	h.LoseAnswer("/claim", 1)
	r := h.Start()

	h.WantState(id, domain.NeedsAnswer)
	r.WantOutput(id + " turn ended")
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
	h := wakingRig(t)
	id := h.ReadyTicket("Lost on the way")
	h.LoseRequest("/claim", 1)
	r := h.Start()

	h.WantState(id, domain.InReview)
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
	h := wakingRig(t)
	// Every try but the last is cut before Code Central sees it, so the
	// other session has from the first cut to the last try to claim.
	h.LoseRequest("/claim", 3)
	r := h.Start()
	// The Ticket wakes a poll, whose first try is cut and which then waits
	// out the claim retry pause: Settle doesn't move the clock past it, so
	// the other session claims between the first try and the rest.
	id := h.ReadyTicket("Taken elsewhere")
	r.Settle()
	if n := h.Refused(claimPath); n != 1 {
		t.Fatalf("%d claims cut before the other session claimed, want 1: the Factory waits to claim again", n)
	}
	if _, err := h.Server.Client.Claim(context.Background(), h.Project, api.ClaimInput{Ticket: api.Ref(id), SessionID: "elsewhere"}); err != nil {
		t.Fatal(err)
	}

	// The poll passes the pause, and the last try reaches Code Central.
	r.Poll()
	r.WantOutput("claim: action_not_allowed")
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
	if tk.State != domain.InProgress {
		t.Errorf("%s is %s, want In Progress under the other session's Claim", id, tk.State)
	}
}

// Every try lost, the Factory sends the claim 4 times in all, the first
// and 3 more, and starts no turn.
func TestAClaimLostOnEveryTryIsSentFourTimes(t *testing.T) {
	t.Parallel()
	h := wakingRig(t)
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
	if s := h.Ticket(id).State; s != domain.Ready {
		t.Errorf("%s is %s, want Ready: no try reached Code Central", id, s)
	}
}

// Lost on every try within a poll, the claim is remembered: each later
// poll sends it once more, with no pause, until Code Central answers,
// and the turn runs under the one Claim the first try made.
func TestAClaimLostOnEveryTryIsClaimedAgainAtLaterPolls(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("Lost for a while")
	// The first poll's 4 tries and the next poll's one.
	h.LoseAnswer("/claim", 5)
	r := h.Start()
	if n := h.Refused(claimPath); n != 4 || len(h.Calls()) != 0 {
		t.Fatalf("after the first poll %d claims cut and claude ran %d times, want 4 and none", n, len(h.Calls()))
	}

	r.Poll()
	if n := h.Refused(claimPath); n != 5 || len(h.Calls()) != 0 {
		t.Fatalf("after the second poll %d claims cut and claude ran %d times, want 5 and none", n, len(h.Calls()))
	}
	r.Poll()
	h.WantState(id, domain.InReview)
	if n := len(h.Ticket(id).Claims); n != 1 {
		t.Errorf("%s has %d Claims, want 1: claiming again gets the same Claim", id, n)
	}
	out := r.Stdout()
	for _, want := range []string{"the answer was lost", id + ": claim: the answer came back on claiming it again"} {
		if n := strings.Count(out, want); n != 1 {
			t.Errorf("%q printed %d times, want once; stdout:\n%s", want, n, out)
		}
	}
}

// A claim whose answer is lost holds its --parallel slot until Code
// Central answers, so once it does the Factory runs no more turns than
// it has slots.
func TestALostClaimHoldsItsSlot(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	a, b, c := h.ReadyTicket("A"), h.ReadyTicket("B"), h.ReadyTicket("C")
	for _, id := range []string{a, b, c} {
		h.Script(id, factorytest.Wait(), factorytest.Finish("Done."), factorytest.Result("success"))
	}
	h.LoseAnswer("/claim", 5)
	r := h.Start("--parallel", "2")

	r.Poll()
	if n := len(callsFor(h, b)); n != 1 {
		t.Fatalf("%s ran %d times at the second poll, want once", b, n)
	}
	if s := h.Ticket(c).State; s != domain.Ready {
		t.Fatalf("with %s's claim lost and %s running, %s is %s; want it left Ready", a, b, c, s)
	}
	r.Poll()
	if n := len(callsFor(h, a)); n != 1 {
		t.Fatalf("%s ran %d times at the third poll, want once", a, n)
	}
	r.Poll()
	if s := h.Ticket(c).State; s != domain.Ready || len(h.Calls()) != 2 {
		t.Errorf("with two turns running, %s is %s and claude ran %d times; want it left Ready", c, s, len(h.Calls()))
	}
	if n := len(h.Ticket(a).Claims); n != 1 {
		t.Errorf("%s has %d Claims, want 1", a, n)
	}
}

// Code Central's refusal of a remembered claim, at a later poll, is an
// answer: the Factory drops it, freeing its slot, and sends it no more.
func TestALostClaimRefusedAtALaterPollIsDropped(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	a, b := h.ReadyTicket("Taken elsewhere"), h.ReadyTicket("Next")
	h.LoseRequest("/claim", 4)
	r := h.Start("--parallel", "1")
	if _, err := h.Server.Client.Claim(context.Background(), h.Project, api.ClaimInput{Ticket: api.Ref(a), SessionID: "elsewhere"}); err != nil {
		t.Fatal(err)
	}

	r.Poll()
	h.WantState(b, domain.InReview)
	r.Poll()
	if n := strings.Count(r.Stdout(), a+": claim: action_not_allowed"); n != 1 {
		t.Errorf("the refusal printed %d times, want once; stdout:\n%s", n, r.Stdout())
	}
	tk := h.Ticket(a)
	if len(tk.Claims) != 1 || tk.Claims[0].SessionID != "elsewhere" || len(callsFor(h, a)) != 0 {
		t.Errorf("%s has Claims %+v and ran %d times, want only the other session's and none", a, tk.Claims, len(callsFor(h, a)))
	}
}

// A draining Factory claims nothing new, but still claims again a claim
// whose answer was lost: its Ticket may be In Progress already.
func TestADrainingFactoryStillClaimsALostClaimAgain(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id, later := h.ReadyTicket("Lost"), h.ReadyTicket("Later")
	// The first poll's 4 tries and the drain's first.
	h.LoseAnswer("/claim", 5)
	r := h.Start("--parallel", "2")

	r.Interrupt()
	r.Settle()
	r.WantOutput("draining: claiming " + id + " again")
	r.Tick()
	if code := r.Exited(); code != 0 {
		t.Errorf("exit code %d, want 0", code)
	}
	tk := h.Ticket(id)
	if tk.State != domain.InReview || len(tk.Claims) != 1 {
		t.Errorf("%s is %s with %d Claims, want In Review with 1", id, tk.State, len(tk.Claims))
	}
	if s := h.Ticket(later).State; s != domain.Ready || len(h.Calls()) != 1 {
		t.Errorf("%s is %s after %d runs; a draining Factory claims nothing new", later, s, len(h.Calls()))
	}
}

// A stopping Factory abandons a claim whose answer was lost.
func TestAStoppingFactoryAbandonsALostClaim(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("Lost")
	h.LoseAnswer("/claim", 4)
	r := h.Start()

	if code := r.Stop(); code != 0 {
		t.Errorf("exit code %d, want 0", code)
	}
	if n := h.Refused(claimPath); n != 4 || len(h.Calls()) != 0 {
		t.Errorf("%d claims cut and claude ran %d times, want 4 and none", n, len(h.Calls()))
	}
	if s := h.Ticket(id).State; s != domain.InProgress {
		t.Errorf("%s is %s, want In Progress under the Claim the stop left", id, s)
	}
}

// A claim lost before it reached Code Central leaves its Ticket Ready,
// but the Factory doesn't choose it as new work under another session:
// it waits for the lost claim, and claims the next Ticket meanwhile.
func TestALostClaimsTicketIsNotChosenAgainAsNewWork(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	a, b := h.ReadyTicket("Lost on the way"), h.ReadyTicket("Next")
	for _, id := range []string{a, b} {
		h.Script(id, factorytest.Wait(), factorytest.Finish("Done."), factorytest.Result("success"))
	}
	// The first poll's 4 tries and the next poll's one.
	h.LoseRequest("/claim", 5)
	r := h.Start("--parallel", "2")

	r.Poll()
	if n := len(callsFor(h, b)); n != 1 {
		t.Fatalf("%s ran %d times at the second poll, want once", b, n)
	}
	if s := h.Ticket(a).State; s != domain.Ready || len(callsFor(h, a)) != 0 {
		t.Fatalf("%s is %s after %d runs, want Ready and none: its claim is still lost", a, s, len(callsFor(h, a)))
	}
	r.Poll()
	if n := len(callsFor(h, a)); n != 1 {
		t.Fatalf("%s ran %d times at the third poll, want once", a, n)
	}
	if n := len(h.Ticket(a).Claims); n != 1 {
		t.Errorf("%s has %d Claims, want 1", a, n)
	}
	if strings.Contains(r.Stdout(), "action_not_allowed") {
		t.Errorf("a claim was refused; stdout:\n%s", r.Stdout())
	}
}

func TestTheFactoryNamesItselfOnItsClaimSoItIsAFactoryClaim(t *testing.T) {
	t.Parallel()
	h := wakingRig(t)
	id := h.ReadyTicket("Claimed by the Factory")
	h.Start()

	h.WantState(id, domain.InReview)
	if claims := h.Ticket(id).Claims; len(claims) != 1 || !claims[0].FactoryClaim {
		t.Errorf("%s Claims = %+v, want one Factory Claim", id, claims)
	}
}
