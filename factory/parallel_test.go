package factory_test

import (
	"testing"

	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// callsFor is the claude runs for ticket, oldest first.
func callsFor(h *factorytest.Harness, ticket string) []factorytest.Call {
	var out []factorytest.Call
	for _, c := range h.Calls() {
		if c.Ticket == ticket {
			out = append(out, c)
		}
	}
	return out
}

func TestParallelCapsTheTurnsRunningAtOnce(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	a, b, c := h.ReadyTicket("A"), h.ReadyTicket("B"), h.ReadyTicket("C")
	for _, id := range []string{a, b, c} {
		h.Script(id, factorytest.Wait(), factorytest.Finish("Done."), factorytest.Result("success"))
	}

	r := h.Start("--parallel", "2")
	h.WaitFor("two turns", func() bool { return len(h.Calls()) == 2 })
	r.Poll()
	if s := h.Ticket(c).State; s != store.Ready || len(h.Calls()) != 2 {
		t.Fatalf("with two turns running, %s is %s and claude ran %d times; want it left Ready", c, s, len(h.Calls()))
	}

	h.Continue(a)
	h.WaitForState(a, store.InReview)
	r.WaitForOutput(a + " turn ended")
	r.Poll()
	h.WaitFor(c+"'s turn", func() bool { return len(callsFor(h, c)) == 1 })
}

func TestAClaimParkedInNeedsAnswerDoesNotTakeASlot(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	a, b := h.ReadyTicket("A"), h.ReadyTicket("B")
	h.Script(a, factorytest.PostRound("Which colour?"), factorytest.Result("success"))
	h.Script(b, factorytest.Wait())

	r := h.Start("--parallel", "1")
	h.WaitForState(a, store.NeedsAnswer)
	r.WaitForOutput(a + " turn ended")
	r.Poll()
	h.WaitFor(b+"'s turn", func() bool { return len(callsFor(h, b)) == 1 })
}
