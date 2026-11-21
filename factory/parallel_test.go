package factory_test

import (
	"github.com/tomoram/code-central/domain"
	"testing"

	"github.com/tomoram/code-central/factory/factorytest"
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
	h := factorytest.New(t, factorytest.FakeGit())
	a, b, c := h.ReadyTicket("A"), h.ReadyTicket("B"), h.ReadyTicket("C")
	for _, id := range []string{a, b, c} {
		h.Script(id, factorytest.Wait(), factorytest.Finish("Done."), factorytest.Result("success"))
	}

	r := h.Start("--parallel", "2")
	h.WantCalls(2)
	r.Poll()
	if s := h.Ticket(c).State; s != domain.Ready || len(h.Calls()) != 2 {
		t.Fatalf("with two turns running, %s is %s and claude ran %d times; want it left Ready", c, s, len(h.Calls()))
	}

	h.Continue(a)
	r.Settle()
	h.WantState(a, domain.InReview)
	r.WantOutput(a + " turn ended")
	r.Poll()
	if n := len(callsFor(h, c)); n != 1 {
		t.Errorf("claude ran %d times for %s, want once: the freed slot is filled", n, c)
	}
}

func TestAClaimParkedInNeedsAnswerDoesNotTakeASlot(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	a, b := h.ReadyTicket("A"), h.ReadyTicket("B")
	h.Script(a, factorytest.PostRound("Which colour?"), factorytest.Result("success"))
	h.Script(b, factorytest.Wait())

	r := h.Start("--parallel", "1")
	h.WantState(a, domain.NeedsAnswer)
	r.WantOutput(a + " turn ended")
	r.Poll()
	if n := len(callsFor(h, b)); n != 1 {
		t.Errorf("claude ran %d times for %s, want once: the parked claim takes no slot", n, b)
	}
}
