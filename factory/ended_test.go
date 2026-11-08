package factory_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// stateTickets is the Tickets of the Claims in the Factory's state file.
func stateTickets(t *testing.T, h *factorytest.Harness) []string {
	t.Helper()
	b, err := os.ReadFile(h.StateFile())
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
	h := factorytest.New(t)
	id := h.ReadyTicket("Stopped")
	h.Script(id, factorytest.Wait())
	r := h.Start()
	h.WaitForState(id, store.InProgress)
	h.WaitFor("the turn", func() bool { return len(h.Calls()) == 1 })

	if _, err := h.Human().Release(context.Background(), h.Project, core.ReleaseInput{Ticket: core.Ref(id)}); err != nil {
		t.Fatal(err)
	}
	h.Script(id, factorytest.Finish("Done."), factorytest.Result("success"))
	r.Poll()

	wantStopped(t, h, id)
	r.WaitForOutput(fmt.Sprintf(`%s turn ended (implementation): "Stopped": exit %d`, id, stoppedExit()))
	if ended := h.Calls()[0].Claim; slices.Contains(stateClaims(t, h), ended) {
		t.Errorf("state file still holds the ended Claim %s", ended)
	}
}

// stateClaims is the Claim IDs in the Factory's state file.
func stateClaims(t *testing.T, h *factorytest.Harness) []string {
	t.Helper()
	b, err := os.ReadFile(h.StateFile())
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
	h := factorytest.New(t)
	id := h.ReadyTicket("Not wanted")
	h.Script(id, factorytest.Wait())
	r := h.Start()
	h.WaitFor("the turn", func() bool { return len(h.Calls()) == 1 })

	if _, err := h.Human().Cancel(context.Background(), h.Project, core.CancelInput{Ticket: core.Ref(id)}); err != nil {
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
	h := factorytest.New(t)
	id := h.ReadyTicket("Try again")
	h.Script(id,
		factorytest.WriteFile("wip.txt", "wip\n"),
		factorytest.Git("add", "wip.txt"),
		factorytest.Git("commit", "-q", "-m", "wip"),
		factorytest.WriteFile("committed", ""),
		factorytest.Wait())
	r := h.Start()
	h.WaitFor("the first turn's commit", func() bool {
		_, err := os.Stat(filepath.Join(h.Worktree(id), "committed"))
		return err == nil
	})

	if _, err := h.Human().Release(context.Background(), h.Project, core.ReleaseInput{Ticket: core.Ref(id)}); err != nil {
		t.Fatal(err)
	}
	h.Script(id, factorytest.NeedFile("wip.txt"), factorytest.Finish("Done."), factorytest.Result("success"))
	r.Poll()
	tk := h.WaitForState(id, store.InReview)

	calls := h.Calls()
	if len(calls) != 2 {
		t.Fatalf("claude ran %d times, want twice", len(calls))
	}
	first, second := calls[0], calls[1]
	if second.Dir != h.Worktree(id) || second.Claim == first.Claim || second.Session == first.Session {
		t.Errorf("second run %+v; want the same worktree under a new Claim and session (first %+v)", second, first)
	}
	if !second.Has("--session-id") {
		t.Errorf("args %q: a fresh Claim starts a new session", second.Args)
	}
	if n := len(tk.Claims); n != 2 {
		t.Errorf("%s has %d Claims, want 2", id, n)
	}
	if got := stateTickets(t, h); !slices.Equal(got, []string{id}) {
		t.Errorf("state file holds %q, want only the new Claim on %s", got, id)
	}
}
