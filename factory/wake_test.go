package factory_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// The Factory's wake (Spec code-central-743): no test here ticks after
// start-up unless it says so, so whatever the Factory does it does on a
// wake from next_factory_wake.

const wakePath = "/api/p/acme/factory/next-wake"

// wakingHarness is a harness that passes the Factory its wakes.
func wakingHarness(t *testing.T) *factorytest.Harness {
	h := factorytest.New(t)
	h.PassWakes()
	return h
}

func TestAnApprovedTicketIsClaimedOnAWake(t *testing.T) {
	t.Parallel()
	h := wakingHarness(t)
	id := h.NewTicket(core.CreateTicketInput{Title: "Approve me", State: store.Draft})
	r := h.Start()
	r.WaitForLongPoll()

	if _, err := h.Human().Approve(context.Background(), h.Project, core.TicketInput{Ticket: core.Ref(id)}); err != nil {
		t.Fatal(err)
	}
	h.WaitForState(id, store.InReview)
}

// A Cancelled Blocker is followed through its Replacements, so cancelling
// it for one already Done clears it.
func TestATicketWhoseLastBlockerIsCancelledIsClaimedOnAWake(t *testing.T) {
	t.Parallel()
	h := wakingHarness(t)
	blocker := h.NewTicket(core.CreateTicketInput{Title: "Not wanted", State: store.Draft})
	done := h.NewTicket(core.CreateTicketInput{Title: "Done instead", State: store.Draft})
	ctx := context.Background()
	if _, err := h.Human().Move(ctx, h.Project, core.MoveInput{Ticket: core.Ref(done), ToState: store.Done}); err != nil {
		t.Fatal(err)
	}
	id := h.NewTicket(core.CreateTicketInput{Title: "Waits", State: store.Ready, BlockedBy: []core.Ref{core.Ref(blocker)}})
	r := h.Start()
	r.WaitForLongPoll()

	if _, err := h.Human().Cancel(ctx, h.Project, core.CancelInput{Ticket: core.Ref(blocker), Replacements: []core.Ref{core.Ref(done)}}); err != nil {
		t.Fatal(err)
	}
	h.WaitForState(id, store.InReview)
}

// Accepting wakes the Factory to land the Ticket, and its landing wakes it
// again to claim the Ticket that waited on it.
func TestAnAcceptedTicketLandsAndItsBlockedTicketIsClaimedOnWakes(t *testing.T) {
	t.Parallel()
	h := wakingHarness(t)
	blocker := h.ReadyTicket("First")
	h.Finished(blocker, blocker, h.CommitOn(blocker, "first.txt", "first\n"))
	id := h.NewTicket(core.CreateTicketInput{Title: "Second", State: store.Ready, BlockedBy: []core.Ref{core.Ref(blocker)}})
	r := h.Start()
	r.WaitForLongPoll()

	h.Accept(blocker)
	h.WaitForState(blocker, store.Done)
	h.WaitForState(id, store.InReview)
}

func TestAnsweringARoundsLastQuestionResumesTheSessionOnAWake(t *testing.T) {
	t.Parallel()
	h := wakingHarness(t)
	id := h.ReadyTicket("Ask first")
	h.Script(id, factorytest.PostRound("Which colour?"), factorytest.Result("success"))
	r := h.Start()
	h.WaitForState(id, store.NeedsAnswer)
	r.WaitForOutput(id + " turn ended")
	h.Script(id, factorytest.Finish("Painted it blue."), factorytest.Result("success"))
	r.WaitForLongPoll()

	h.Answer(id, "Blue")
	h.WaitForState(id, store.InReview)
	if n := len(h.Calls()); n != 2 {
		t.Errorf("claude ran %d times, want twice: the turn and its resume", n)
	}
}

func TestCancellingARunningClaimStopsItsTurnOnAWake(t *testing.T) {
	t.Parallel()
	h := wakingHarness(t)
	id := h.ReadyTicket("Not wanted")
	h.Script(id, factorytest.Wait())
	r := h.Start()
	h.WaitFor("the turn", func() bool { return len(h.Calls()) == 1 })
	r.WaitForLongPoll()

	if _, err := h.Human().Cancel(context.Background(), h.Project, core.CancelInput{Ticket: core.Ref(id)}); err != nil {
		t.Fatal(err)
	}
	h.WaitFor("the turn to be stopped", func() bool { return slices.Equal(h.Signals(id), stopSignals()) })
}

// Every slot full or paused, the Factory keeps its long-poll open, and a
// wake still runs the poll: it lands, reconciles and resumes while it
// can't claim.
func TestWithEverySlotFullAWakeStillRunsThePoll(t *testing.T) {
	t.Parallel()
	h := wakingHarness(t)
	running := h.ReadyTicket("Running")
	h.Script(running, factorytest.Wait())
	r := h.Start("--parallel", "1")
	h.WaitFor("the turn", func() bool { return len(h.Calls()) == 1 })
	id := h.ReadyTicket("Done elsewhere")
	h.Finished(id, id, h.CommitOn(id, "elsewhere.txt", "elsewhere\n"))
	r.WaitForLongPoll()

	h.Accept(id)
	h.WaitForState(id, store.Done)
}

func TestWhilePausedAWakeStillRunsThePoll(t *testing.T) {
	t.Parallel()
	h := wakingHarness(t)
	cut := h.ReadyTicket("Cut off")
	h.Script(cut, factorytest.APIError("rate_limit"), factorytest.Result("success"), factorytest.Exit(1))
	r := h.Start()
	r.WaitForOutput("waiting on rate_limit")
	r.WaitForLongPoll()

	if _, err := h.Human().Cancel(context.Background(), h.Project, core.CancelInput{Ticket: core.Ref(cut)}); err != nil {
		t.Fatal(err)
	}
	h.WaitFor("the cut-off Claim to be dropped", func() bool { return len(stateTickets(t, h)) == 0 })
}

// A comment or set_session is nothing the poll acts on: the long-poll
// isn't woken, so a Ticket a poll would claim waits for the next tick.
// The Ticket is made while the long-poll fails, so no wake comes of it.
func TestACommentOrSetSessionDoesNotWakeThePoll(t *testing.T) {
	t.Parallel()
	h := wakingHarness(t)
	h.SetFailing("/factory/next-wake")
	r := h.Start()
	id := h.ReadyTicket("Claim on a poll")
	other := h.ReadyTicket("Held elsewhere")
	ctx := context.Background()
	held, err := h.Server.Client.Claim(ctx, h.Project, core.ClaimInput{Ticket: core.Ref(other)})
	if err != nil {
		t.Fatal(err)
	}
	h.SetFailing("")
	r.WaitForLongPoll()

	if _, err := h.Human().Comment(ctx, h.Project, core.CommentInput{Ticket: core.Ref(id), Body: "Mind the tests."}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Server.Client.SetSession(ctx, h.Project, core.SetSessionInput{Ticket: core.Ref(other), ClaimID: held.ClaimID, SessionID: "elsewhere"}); err != nil {
		t.Fatal(err)
	}
	if n := h.Server.Core.WaitingFactoryWakes(h.Project); n != 1 {
		t.Errorf("%d next_factory_wake calls still waiting, want the Factory's: nothing woke it", n)
	}
	if s := h.Ticket(id).State; s != store.Ready {
		t.Errorf("%s is %s with no poll since it was made, want Ready", id, s)
	}
	r.Poll()
	r.WaitForClaimed(id, store.InReview)
}

func TestAFailingLongPollWarnsOnceAndTheTimedPollCarriesOn(t *testing.T) {
	t.Parallel()
	h := wakingHarness(t)
	h.SetFailing("/factory/next-wake")
	r := h.Start()
	h.WaitFor("the long-poll to fail again after its pause", func() bool { return h.Refused(wakePath) >= 2 })
	if n := strings.Count(r.Stdout(), "warning: can't wait for a wake from Code Central"); n != 1 {
		t.Errorf("warned %d times, want once for the run of failures; stdout:\n%s", n, r.Stdout())
	}

	id := h.ReadyTicket("Claim on a tick")
	r.Poll()
	r.WaitForClaimed(id, store.InReview)
}

func TestAnOlderCodeCentralWithoutTheLongPollGetsTheTimedPollOnly(t *testing.T) {
	t.Parallel()
	h := wakingHarness(t)
	h.SetMissing("/factory/next-wake")
	r := h.Start()
	r.WaitForOutput("warning: Code Central has no next_factory_wake: polling every 30s only")

	id := h.ReadyTicket("Claim on a tick")
	r.Poll()
	r.WaitForClaimed(id, store.InReview)
	if n := h.Refused(wakePath); n != 1 {
		t.Errorf("next_factory_wake was called %d times, want once: long-polling stops for the run", n)
	}
	if n := strings.Count(r.Stdout(), "has no next_factory_wake"); n != 1 {
		t.Errorf("warned %d times, want once; stdout:\n%s", n, r.Stdout())
	}
}

// The first Ctrl-C closes the long-poll, and the draining Factory claims
// nothing more.
func TestADrainingFactoryClaimsNothingOnAWake(t *testing.T) {
	t.Parallel()
	h := wakingHarness(t)
	running := h.ReadyTicket("Nearly done")
	h.Script(running, factorytest.Wait(), factorytest.Finish("Done."), factorytest.Result("success"))
	r := h.Start()
	h.WaitFor("the turn", func() bool { return len(h.Calls()) == 1 })
	r.WaitForLongPoll()

	r.Interrupt()
	r.WaitForOutput("draining: waiting on 1 running turn(s)")
	h.WaitFor("the long-poll to close", func() bool { return h.Server.Core.WaitingFactoryWakes(h.Project) == 0 })
	id := h.ReadyTicket("Too late")
	h.Continue(running)
	if code := r.Exited(); code != 0 {
		t.Errorf("exit code %d, want 0", code)
	}
	if s := h.Ticket(id).State; s != store.Ready || len(h.Calls()) != 1 {
		t.Errorf("%s is %s and claude ran %d times; a draining Factory claims nothing", id, s, len(h.Calls()))
	}
}

// A turn ending frees a --parallel slot, so it wakes the poll loop itself,
// with the long-poll held back and no tick: the next Ticket is claimed at
// once, however the turn ended.
func TestAnEndingTurnFillsItsFreedSlotAtOnce(t *testing.T) {
	t.Parallel()
	for name, end := range map[string][]factorytest.Step{
		"finished": {factorytest.Finish("Done."), factorytest.Result("success")},
		"failed":   {factorytest.Say("Partway."), factorytest.Result("error_during_execution")},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := factorytest.New(t)
			first := h.ReadyTicket("First")
			second := h.ReadyTicket("Second")
			h.Script(first, append([]factorytest.Step{factorytest.Wait()}, end...)...)
			h.Start("--parallel", "1")
			h.WaitFor("the first turn", func() bool { return len(h.Calls()) == 1 })
			if s := h.Ticket(second).State; s != store.Ready {
				t.Fatalf("%s is %s while the only slot is full, want Ready", second, s)
			}

			h.Continue(first)
			h.WaitForState(second, store.InReview)
		})
	}
}

// A draining Factory ignores the local wake: it claims nothing when a
// turn ends.
func TestADrainingFactoryClaimsNothingWhenATurnEnds(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	running := h.ReadyTicket("Nearly done")
	h.Script(running, factorytest.Wait(), factorytest.Finish("Done."), factorytest.Result("success"))
	r := h.Start("--parallel", "1")
	h.WaitFor("the turn", func() bool { return len(h.Calls()) == 1 })
	id := h.ReadyTicket("Too late")

	r.Interrupt()
	r.WaitForOutput("draining: waiting on 1 running turn(s)")
	h.Continue(running)
	if code := r.Exited(); code != 0 {
		t.Errorf("exit code %d, want 0", code)
	}
	if s := h.Ticket(id).State; s != store.Ready || len(h.Calls()) != 1 {
		t.Errorf("%s is %s and claude ran %d times; a draining Factory claims nothing", id, s, len(h.Calls()))
	}
}
