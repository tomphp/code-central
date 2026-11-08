package factory_test

import (
	"strings"
	"testing"

	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// answeredTicket runs the Factory with args on a Ticket whose first turn
// posts a Round, and answers it. It returns the Ticket, its first run and
// the Factory, with the next run scripted to finish.
func answeredTicket(t *testing.T, h *factorytest.Harness, args ...string) (string, factorytest.Call, *factorytest.Running) {
	t.Helper()
	id := h.ReadyTicket("Ask first")
	h.Script(id, factorytest.PostRound("Which colour?"), factorytest.Result("success"))
	r := h.Start(args...)
	h.WaitForState(id, store.NeedsAnswer)
	r.WaitForOutput(id + ": turn ended")
	h.Script(id, factorytest.Finish("Painted it blue."), factorytest.Result("success"))
	h.Answer(id, "Blue")
	return id, h.Calls()[0], r
}

func TestAnAnsweredClaimIsPickedUpAndItsSessionResumedWithTheAnswers(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id, first, r := answeredTicket(t, h, "--model", "opus")

	r.Poll()
	tk := h.WaitForState(id, store.InReview)
	calls := h.Calls()
	if len(calls) != 2 {
		t.Fatalf("claude ran %d times, want twice", len(calls))
	}
	c := calls[1]
	if s, _ := c.Flag("--resume"); s != first.Session || c.Has("--session-id") || !c.Has("-p") {
		t.Errorf("args %q: want -p --resume %s", c.Args, first.Session)
	}
	if c.Claim != first.Claim || c.Dir != h.Worktree(id) {
		t.Errorf("resumed under Claim %s in %s; want the same Claim %s in the worktree", c.Claim, c.Dir, first.Claim)
	}
	if m, _ := c.Flag("--permission-mode"); m != "auto" {
		t.Errorf("--permission-mode %q, want auto", m)
	}
	if p, _ := c.Flag("--permission-prompts"); p != "none" {
		t.Errorf("--permission-prompts %q, want none", p)
	}
	if m, _ := c.Flag("--model"); m != "opus" {
		t.Errorf("--model %q, want opus", m)
	}
	if !strings.Contains(c.Prompt, "Carry on from the answers") || !strings.Contains(c.Prompt, "Blue") {
		t.Errorf("prompt %q: want it to carry on from the answers, giving them", c.Prompt)
	}
	if n := len(tk.Claims); n != 1 {
		t.Errorf("%s has %d Claims; a resume keeps the one Claim", id, n)
	}
}

func TestAResumeIsInBypassModeWithBypassPermissions(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id, _, r := answeredTicket(t, h, "--bypass-permissions")
	r.Poll()
	h.WaitForState(id, store.InReview)
	if m, _ := h.Calls()[1].Flag("--permission-mode"); m != "bypassPermissions" {
		t.Errorf("--permission-mode %q, want bypassPermissions", m)
	}
}

func TestWhenASlotFreesAnAnsweredResumeGoesBeforeANewClaim(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	asks := h.ReadyTicket("Ask first")
	busy := h.ReadyTicket("Busy")
	h.Script(asks, factorytest.PostRound("Which colour?"), factorytest.Result("success"))
	h.Script(busy, factorytest.Wait(), factorytest.Finish("Done."), factorytest.Result("success"))

	r := h.Start("--parallel", "1")
	h.WaitForState(asks, store.NeedsAnswer)
	r.WaitForOutput(asks + ": turn ended")
	r.Poll()
	h.WaitFor(busy+"'s turn", func() bool { return len(callsFor(h, busy)) == 1 })

	h.Script(asks, factorytest.Wait())
	h.Answer(asks, "Blue")
	waiting := h.ReadyTicket("Waiting")
	r.Poll()
	if n := len(callsFor(h, asks)); n != 1 {
		t.Fatalf("%s resumed with no slot free", asks)
	}

	h.Continue(busy)
	h.WaitForState(busy, store.InReview)
	r.WaitForOutput(busy + ": turn ended")
	r.Poll()
	h.WaitFor(asks+"'s resume", func() bool { return len(callsFor(h, asks)) == 2 })
	if s := h.Ticket(waiting).State; s != store.Ready {
		t.Errorf("%s is %s; the resume should have taken the one slot", waiting, s)
	}
}

func TestAResumeWaitsAPollWhileAnotherProcessHoldsTheSession(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id, first, r := answeredTicket(t, h)
	release := h.HoldSession(first.Session)

	r.Poll()
	if s := h.Ticket(id).State; s != store.Answered || len(h.Calls()) != 1 {
		t.Fatalf("%s is %s after %d runs; want it left Answered while another process holds the session", id, s, len(h.Calls()))
	}
	r.WaitForOutput("session " + first.Session + " is held by pid")

	release()
	r.Poll()
	h.WaitForState(id, store.InReview)
}
