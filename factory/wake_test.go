package factory_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"

	"github.com/tomoram/code-central/factory"
	"github.com/tomoram/code-central/factory/factorytest"
)

// The Factory's wake (Spec code-central-743): no test here ticks after
// start-up unless it says so, so whatever the Factory does it does on a
// wake from next_factory_wake.

const wakePath = "/api/p/acme/factory/next-wake"

// wakingRig is a rig that passes the Factory its wakes.
func wakingRig(t *testing.T) *factorytest.Rig {
	h := factorytest.New(t, factorytest.FakeGit())
	h.PassWakes()
	return h
}

func TestAnApprovedTicketIsClaimedOnAWake(t *testing.T) {
	t.Parallel()
	h := wakingRig(t)
	id := h.NewTicket(api.CreateTicketInput{Title: "Approve me", State: domain.Draft})
	r := h.Start()

	if _, err := h.Human().Approve(context.Background(), h.Project, api.TicketInput{Ticket: api.Ref(id)}); err != nil {
		t.Fatal(err)
	}
	r.Settle()
	h.WantState(id, domain.InReview)
}

// A Cancelled Blocker is followed through its Replacements, so cancelling
// it for one already Done clears it.
func TestATicketWhoseLastBlockerIsCancelledIsClaimedOnAWake(t *testing.T) {
	t.Parallel()
	h := wakingRig(t)
	blocker := h.NewTicket(api.CreateTicketInput{Title: "Not wanted", State: domain.Draft})
	done := h.NewTicket(api.CreateTicketInput{Title: "Done instead", State: domain.Draft})
	ctx := context.Background()
	if _, err := h.Human().Move(ctx, h.Project, api.MoveInput{Ticket: api.Ref(done), ToState: domain.Done}); err != nil {
		t.Fatal(err)
	}
	id := h.NewTicket(api.CreateTicketInput{Title: "Waits", State: domain.Ready, BlockedBy: []api.Ref{api.Ref(blocker)}})
	r := h.Start()

	if _, err := h.Human().Cancel(ctx, h.Project, api.CancelInput{Ticket: api.Ref(blocker), Replacements: []api.Ref{api.Ref(done)}}); err != nil {
		t.Fatal(err)
	}
	r.Settle()
	h.WantState(id, domain.InReview)
}

// Accepting wakes the Factory to land the Ticket, and its landing wakes it
// again to claim the Ticket that waited on it.
func TestAnAcceptedTicketLandsAndItsBlockedTicketIsClaimedOnWakes(t *testing.T) {
	t.Parallel()
	h := wakingRig(t)
	blocker := h.ReadyTicket("First")
	h.Finished(blocker, blocker, h.CommitOn(blocker, "first.txt", "first\n"))
	id := h.NewTicket(api.CreateTicketInput{Title: "Second", State: domain.Ready, BlockedBy: []api.Ref{api.Ref(blocker)}})
	r := h.Start()

	h.Accept(blocker)
	r.Settle()
	h.WantState(blocker, domain.Done)
	h.WantState(id, domain.InReview)
}

// The claim's answer may be lost on its way back, as in CI run
// 43772431843: the Factory claims again with no tick.
func TestAnsweringARoundsLastQuestionResumesTheSessionOnAWake(t *testing.T) {
	t.Parallel()
	for name, lost := range map[string]int{"answer kept": 0, "answer lost": 1} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := wakingRig(t)
			id := h.ReadyTicket("Ask first")
			h.Script(id, factorytest.PostRound("Which colour?"), factorytest.Result("success"))
			h.LoseAnswer("/claim", lost)
			r := h.Start()
			h.WantState(id, domain.NeedsAnswer)
			r.WantOutput(id + " turn ended")
			h.Script(id, factorytest.Finish("Painted it blue."), factorytest.Result("success"))

			h.Answer(id, "Blue")
			r.Settle()
			h.WantState(id, domain.InReview)
			if n := len(h.Calls()); n != 2 {
				t.Errorf("claude ran %d times, want twice: the turn and its resume", n)
			}
		})
	}
}

func TestCancellingARunningClaimStopsItsTurnOnAWake(t *testing.T) {
	t.Parallel()
	h := wakingRig(t)
	id := h.ReadyTicket("Not wanted")
	h.Script(id, factorytest.Wait())
	r := h.Start()
	h.WantCalls(1)

	if _, err := h.Human().Cancel(context.Background(), h.Project, api.CancelInput{Ticket: api.Ref(id)}); err != nil {
		t.Fatal(err)
	}
	r.PassKillGrace()
	r.Settle()
	wantStopped(t, h, id)
}

// Every slot full or paused, the Factory keeps its long-poll open, and a
// wake still runs the poll: it lands, reconciles and resumes while it
// can't claim.
func TestWithEverySlotFullAWakeStillRunsThePoll(t *testing.T) {
	t.Parallel()
	h := wakingRig(t)
	running := h.ReadyTicket("Running")
	h.Script(running, factorytest.Wait())
	r := h.Start("--parallel", "1")
	h.WantCalls(1)
	id := h.ReadyTicket("Done elsewhere")
	h.Finished(id, id, h.CommitOn(id, "elsewhere.txt", "elsewhere\n"))
	r.Settle()

	h.Accept(id)
	r.Settle()
	h.WantState(id, domain.Done)
}

func TestWhilePausedAWakeStillRunsThePoll(t *testing.T) {
	t.Parallel()
	h := wakingRig(t)
	cut := h.ReadyTicket("Cut off")
	h.Script(cut, factorytest.APIError("rate_limit"), factorytest.Result("success"), factorytest.Exit(1))
	r := h.Start()
	r.WantOutput("waiting on rate_limit")

	if _, err := h.Human().Cancel(context.Background(), h.Project, api.CancelInput{Ticket: api.Ref(cut)}); err != nil {
		t.Fatal(err)
	}
	r.Settle()
	if got := stateTickets(t, h); len(got) != 0 {
		t.Errorf("state file holds %q, want the cut-off Claim dropped", got)
	}
}

// A comment or set_session is nothing the poll acts on: the long-poll
// isn't woken, so a Ticket a poll would claim waits for the next tick.
// The Ticket is made while the long-poll fails, so no wake comes of it.
func TestACommentOrSetSessionDoesNotWakeThePoll(t *testing.T) {
	t.Parallel()
	h := wakingRig(t)
	h.SetFailing("/factory/next-wake")
	r := h.Start()
	id := h.ReadyTicket("Claim on a poll")
	other := h.ReadyTicket("Held elsewhere")
	ctx := context.Background()
	held, err := h.Server.Client.Claim(ctx, h.Project, api.ClaimInput{Ticket: api.Ref(other)})
	if err != nil {
		t.Fatal(err)
	}
	h.SetFailing("")
	// The long-poll's pause after failing is due before the poll is.
	if at, ok := h.Clock.Next(); !ok || !at.Equal(h.Clock.Now().Add(factory.WakeRetry)) {
		t.Fatalf("the next timer is due at %v, want the long-poll's pause after failing", at)
	}
	h.Clock.Advance(factory.WakeRetry)
	r.Settle()

	if _, err := h.Human().Comment(ctx, h.Project, api.CommentInput{Ticket: api.Ref(id), Body: "Mind the tests."}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Server.Client.SetSession(ctx, h.Project, api.SetSessionInput{Ticket: api.Ref(other), ClaimID: held.ClaimID, SessionID: "elsewhere"}); err != nil {
		t.Fatal(err)
	}
	if n := h.Server.Core.Factories().WaitingFactoryWakes(h.Project); n != 1 {
		t.Errorf("%d next_factory_wake calls still waiting, want the Factory's: nothing woke it", n)
	}
	r.Settle()
	if s := h.Ticket(id).State; s != domain.Ready {
		t.Errorf("%s is %s with no poll since it was made, want Ready", id, s)
	}
	r.Poll()
	h.WantState(id, domain.InReview)
}

func TestAFailingLongPollWarnsOnceAndTheTimedPollCarriesOn(t *testing.T) {
	t.Parallel()
	h := wakingRig(t)
	h.SetFailing("/factory/next-wake")
	r := h.Start()
	// The poll's Tick passes the long-poll's pauses after failing.
	r.Poll()
	if n := h.Refused(wakePath); n < 2 {
		t.Errorf("next_factory_wake was refused %d times, want it called again after its pause", n)
	}
	if n := strings.Count(r.Stdout(), "warning: can't wait for a wake from Code Central"); n != 1 {
		t.Errorf("warned %d times, want once for the run of failures; stdout:\n%s", n, r.Stdout())
	}

	id := h.ReadyTicket("Claim on a tick")
	r.Poll()
	h.WantState(id, domain.InReview)
}

func TestAnOlderCodeCentralWithoutTheLongPollGetsTheTimedPollOnly(t *testing.T) {
	t.Parallel()
	h := wakingRig(t)
	h.SetMissing("/factory/next-wake")
	r := h.Start()
	r.WantOutput("warning: Code Central has no next_factory_wake: polling every 30s only")

	id := h.ReadyTicket("Claim on a tick")
	r.Poll()
	h.WantState(id, domain.InReview)
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
	h := wakingRig(t)
	running := h.ReadyTicket("Nearly done")
	h.Script(running, factorytest.Wait(), factorytest.Finish("Done."), factorytest.Result("success"))
	r := h.Start()
	h.WantCalls(1)

	r.Interrupt()
	r.Settle()
	r.WantOutput("draining: waiting on 1 running turn(s)")
	// Code Central lets go of the long-poll once the cancel reaches it.
	h.WaitFor("the long-poll to close", func() bool { return h.Server.Core.Factories().WaitingFactoryWakes(h.Project) == 0 })
	id := h.ReadyTicket("Too late")
	h.Continue(running)
	if code := r.Exited(); code != 0 {
		t.Errorf("exit code %d, want 0", code)
	}
	if s := h.Ticket(id).State; s != domain.Ready || len(h.Calls()) != 1 {
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
			h := factorytest.New(t, factorytest.FakeGit())
			first := h.ReadyTicket("First")
			second := h.ReadyTicket("Second")
			h.Script(first, append([]factorytest.Step{factorytest.Wait()}, end...)...)
			r := h.Start("--parallel", "1")
			h.WantCalls(1)
			if s := h.Ticket(second).State; s != domain.Ready {
				t.Fatalf("%s is %s while the only slot is full, want Ready", second, s)
			}

			h.Continue(first)
			r.Settle()
			h.WantState(second, domain.InReview)
		})
	}
}

// A Ticket whose agent gave up waits for the next timed poll: the wake
// at its turn's end claims the next Ticket, not it again.
func TestAGivenUpTicketWaitsForTheNextTimedPoll(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	gaveUp := h.ReadyTicket("Too hard")
	h.Script(gaveUp, factorytest.CLI("give-up", "{{ticket}}", "--claim", "{{claim}}", "--note", "Too hard."), factorytest.Result("success"))
	next := h.ReadyTicket("Next")
	r := h.Start("--parallel", "1")
	h.WantState(next, domain.InReview)
	if n := len(h.Ticket(gaveUp).Claims); n != 1 {
		t.Fatalf("%s was claimed %d times before a timed poll, want once", gaveUp, n)
	}

	r.Poll()
	if n := len(h.Ticket(gaveUp).Claims); n != 2 {
		t.Errorf("%s was claimed %d times after a timed poll, want again", gaveUp, n)
	}
}

// A Ticket whose agent has given up but whose turn is still running isn't
// claimed again, though it is Ready: no poll, woken or timed, claims it
// before its turn ends and holds it back.
func TestAGivenUpTicketIsntClaimedWhileItsTurnRuns(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	gaveUp := h.ReadyTicket("Too hard")
	h.Script(gaveUp, factorytest.CLI("give-up", "{{ticket}}", "--claim", "{{claim}}", "--note", "Too hard."), factorytest.Wait(), factorytest.Result("success"))
	next := h.ReadyTicket("Next")
	r := h.Start("--parallel", "2")
	h.WantState(gaveUp, domain.Ready)
	r.WantOutput(next + " turn ended")
	r.Poll()
	if n := len(h.Ticket(gaveUp).Claims); n != 1 {
		t.Fatalf("%s was claimed %d times while its turn ran, want once", gaveUp, n)
	}

	h.Continue(gaveUp)
	r.Settle()
	r.WantOutput(gaveUp + " turn ended")
	r.Poll()
	if n := len(h.Ticket(gaveUp).Claims); n != 2 {
		t.Errorf("%s was claimed %d times after its turn ended and a timed poll, want again", gaveUp, n)
	}
}

// A draining Factory ignores the local wake: it claims nothing when a
// turn ends.
func TestADrainingFactoryClaimsNothingWhenATurnEnds(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	running := h.ReadyTicket("Nearly done")
	h.Script(running, factorytest.Wait(), factorytest.Finish("Done."), factorytest.Result("success"))
	r := h.Start("--parallel", "1")
	h.WantCalls(1)
	id := h.ReadyTicket("Too late")

	r.Interrupt()
	r.Settle()
	r.WantOutput("draining: waiting on 1 running turn(s)")
	h.Continue(running)
	if code := r.Exited(); code != 0 {
		t.Errorf("exit code %d, want 0", code)
	}
	if s := h.Ticket(id).State; s != domain.Ready || len(h.Calls()) != 1 {
		t.Errorf("%s is %s and claude ran %d times; a draining Factory claims nothing", id, s, len(h.Calls()))
	}
}
