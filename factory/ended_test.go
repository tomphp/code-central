package factory_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"

	"github.com/tomoram/code-central/factory/factorytest"
)

// stateTickets is the Tickets of the Claims in the Factory's state file.
func stateTickets(t *testing.T, h *factorytest.Rig) []string {
	t.Helper()
	b, err := h.ReadStateFile()
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Claims []struct {
			Ticket string `json:"ticket"`
		} `json:"claims"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
	out := []string{}
	for _, c := range doc.Claims {
		out = append(out, c.Ticket)
	}
	return out
}

func TestAReleasedClaimsTurnIsStoppedAndDropped(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("Stopped")
	h.Script(id, factorytest.Wait())
	r := h.Start()
	h.WantState(id, domain.InProgress)
	h.WantCalls(1)

	if _, err := h.Human().Release(context.Background(), h.Project, api.ReleaseInput{Ticket: api.Ref(id)}); err != nil {
		t.Fatal(err)
	}
	h.Script(id, factorytest.Finish("Done."), factorytest.Result("success"))
	r.Poll()

	wantStopped(t, h, id)
	r.WantOutput(fmt.Sprintf(`%s turn ended (implementation): "Stopped": exit %d`, id, stoppedExit()))
	if ended := h.Calls()[0].Claim; slices.Contains(stateClaims(t, h), ended) {
		t.Errorf("state file still holds the ended Claim %s", ended)
	}
}

// stateClaims is the Claim IDs in the Factory's state file.
func stateClaims(t *testing.T, h *factorytest.Rig) []string {
	t.Helper()
	b, err := h.ReadStateFile()
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Claims []struct {
			ClaimID string `json:"claim_id"`
		} `json:"claims"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
	out := []string{}
	for _, c := range doc.Claims {
		out = append(out, c.ClaimID)
	}
	return out
}

func TestACancelledClaimsTurnIsStoppedAndDropped(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("Not wanted")
	h.Script(id, factorytest.Wait())
	r := h.Start()
	h.WantCalls(1)

	if _, err := h.Human().Cancel(context.Background(), h.Project, api.CancelInput{Ticket: api.Ref(id)}); err != nil {
		t.Fatal(err)
	}
	r.Poll()
	wantStopped(t, h, id)
	if got := stateTickets(t, h); len(got) != 0 {
		t.Errorf("state file holds %q, want the Cancelled Ticket's Claim dropped", got)
	}
}

func TestAReleasedTicketIsClaimedAfreshInItsWorktreeOnItsBranch(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("Try again")
	h.Script(id,
		factorytest.WriteFile("wip.txt", "wip\n"),
		factorytest.Git("add", "wip.txt"),
		factorytest.Git("commit", "-q", "-m", "wip"),
		factorytest.WriteFile("committed", ""),
		factorytest.Wait())
	r := h.Start()
	if _, err := os.Stat(filepath.Join(h.Worktree(id), "committed")); err != nil {
		t.Fatalf("the first turn didn't get as far as its commit: %v", err)
	}

	if _, err := h.Human().Release(context.Background(), h.Project, api.ReleaseInput{Ticket: api.Ref(id)}); err != nil {
		t.Fatal(err)
	}
	// The second turn is held, so the state file is read while its Claim
	// is live: once the turn has ended, the poll its end wakes drops the
	// Claim.
	h.Script(id, factorytest.NeedFile("wip.txt"), factorytest.Wait(), factorytest.Finish("Done."), factorytest.Result("success"))
	r.Poll()

	calls := h.WantCalls(2)
	first, second := calls[0], calls[1]
	if second.Dir != h.Worktree(id) || second.Claim == first.Claim || second.Session == first.Session {
		t.Errorf("second run %+v; want the same worktree under a new Claim and session (first %+v)", second, first)
	}
	if !second.Has("--session-id") {
		t.Errorf("args %q: a fresh Claim starts a new session", second.Args)
	}
	if got := stateTickets(t, h); !slices.Equal(got, []string{id}) {
		t.Errorf("state file holds %q, want only the new Claim on %s", got, id)
	}

	h.Continue(id)
	r.Settle()
	tk := h.WantState(id, domain.InReview)
	if n := len(tk.Claims); n != 2 {
		t.Errorf("%s has %d Claims, want 2", id, n)
	}
}
