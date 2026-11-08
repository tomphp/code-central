package factory_test

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

func TestARestartReconcilesEachSavedClaimAgainstCodeCentral(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	cutOff := h.ReadyTicket("Cut off")
	answered := h.ReadyTicket("Answered while down")
	waiting := h.ReadyTicket("Still waiting")
	finished := h.ReadyTicket("Finished while down")
	cancelled := h.ReadyTicket("Cancelled while down")
	for _, id := range []string{cutOff, finished, cancelled} {
		h.Script(id, factorytest.Wait())
	}
	for _, id := range []string{answered, waiting} {
		h.Script(id, factorytest.PostRound("Which colour?"), factorytest.Result("success"))
	}
	r := h.Start("--parallel", "5")
	h.WaitForState(answered, store.NeedsAnswer)
	h.WaitForState(waiting, store.NeedsAnswer)
	h.WaitFor("every turn", func() bool { return len(h.Calls()) == 5 })
	r.Signal(os.Interrupt)
	r.Signal(os.Interrupt)
	r.Exited()
	first := map[string]factorytest.Call{}
	for _, c := range h.Calls() {
		first[c.Ticket] = c
	}

	// While the Factory is down: one Round is answered, one Ticket is
	// finished by its session, one is Cancelled, and someone else claims
	// a Ticket.
	h.Answer(answered, "Blue")
	ctx := context.Background()
	if _, err := h.Server.Client.Finish(ctx, h.Project, core.FinishInput{Ticket: core.Ref(finished), ClaimID: first[finished].Claim, Summary: "Done."}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Human().Cancel(ctx, h.Project, core.CancelInput{Ticket: core.Ref(cancelled)}); err != nil {
		t.Fatal(err)
	}
	notOurs := h.ReadyTicket("Someone else's")
	if _, err := h.Server.Client.Claim(ctx, h.Project, core.ClaimInput{Ticket: core.Ref(notOurs)}); err != nil {
		t.Fatal(err)
	}
	h.Script(answered, factorytest.Finish("Painted it blue."), factorytest.Result("success"))
	h.Script(cutOff, factorytest.Finish("Done."), factorytest.Result("success"))
	hold := h.HoldSession(first[cutOff].Session)

	// --parallel 1: the Answered resume takes the one slot first.
	r = h.Start("--parallel", "1")
	h.WaitForState(answered, store.InReview)
	if got := stateTickets(t, h); !slices.Contains(got, cutOff) || !slices.Contains(got, waiting) || slices.Contains(got, finished) || slices.Contains(got, cancelled) {
		t.Errorf("state file holds %q; want %s and %s kept, %s and %s dropped", got, cutOff, waiting, finished, cancelled)
	}
	c := lastCall(h, answered)
	if s, _ := c.Flag("--resume"); s != first[answered].Session || !strings.Contains(c.Prompt, "Blue") {
		t.Errorf("%s resumed with %q; want its session resumed with the answers", answered, c.Args)
	}
	if n := len(callsFor(h, cutOff)); n != 1 {
		t.Fatalf("%s ran again before the Answered resume ended", cutOff)
	}

	// The cut-off session waits a poll while another process holds it.
	r.WaitForOutput(answered + ": turn ended")
	r.Poll()
	r.WaitForOutput("session " + first[cutOff].Session + " is held by pid")
	hold()
	r.Poll()
	h.WaitForState(cutOff, store.InReview)
	c = lastCall(h, cutOff)
	if s, _ := c.Flag("--resume"); s != first[cutOff].Session || c.Claim != first[cutOff].Claim || !strings.Contains(c.Prompt, "Your last turn was cut off; carry on.") {
		t.Errorf("%s resumed with %q; want its session resumed as cut off under its Claim", cutOff, c.Args)
	}

	if s := h.Ticket(waiting).State; s != store.NeedsAnswer || len(callsFor(h, waiting)) != 1 {
		t.Errorf("%s is %s after %d runs; want it left waiting on its Round", waiting, s, len(callsFor(h, waiting)))
	}
	if n := len(callsFor(h, finished)) + len(callsFor(h, cancelled)) + len(callsFor(h, notOurs)); n != 2 {
		t.Errorf("claude ran %d times for the finished, Cancelled and someone else's Tickets; want only their first runs", n)
	}
	if tk := h.Ticket(notOurs); tk.State != store.InProgress || len(tk.Claims) != 1 {
		t.Errorf("%s is %s with %d Claims; a Claim not in the state file is never adopted", notOurs, tk.State, len(tk.Claims))
	}
}

// lastCall is the newest run of the fake claude for ticket.
func lastCall(h *factorytest.Harness, ticket string) factorytest.Call {
	calls := callsFor(h, ticket)
	if len(calls) == 0 {
		return factorytest.Call{}
	}
	return calls[len(calls)-1]
}

func TestALostStateFileLosesItsClaimsButNotTheFactory(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	held := h.ReadyTicket("Held")
	h.Script(held, factorytest.Wait())
	r := h.Start()
	h.WaitFor("the turn", func() bool { return len(h.Calls()) == 1 })
	r.Signal(os.Interrupt)
	r.Signal(os.Interrupt)
	r.Exited()
	if err := os.Remove(h.StateFile()); err != nil {
		t.Fatal(err)
	}

	next := h.ReadyTicket("Next")
	h.Start()
	h.WaitForState(next, store.InReview)
	if n := len(callsFor(h, held)); n != 1 {
		t.Errorf("%s ran %d times; with the state file lost its Claim is never adopted", held, n)
	}
	if got := stateTickets(t, h); !slices.Equal(got, []string{next}) {
		t.Errorf("state file holds %q, want only the new Claim on %s", got, next)
	}
}
