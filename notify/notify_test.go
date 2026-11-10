package notify_test

import (
	"context"
	"testing"
	"time"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/notify"
	"github.com/tomoram/code-central/store"
)

var t0 = time.Date(2026, 11, 7, 9, 30, 0, 0, time.UTC)

var ctx = context.Background()

const base = "http://localhost:7733"

// fakeSender records every Notification on a channel.
type fakeSender chan notify.Notification

func (s fakeSender) Send(n notify.Notification) error {
	s <- n
	return nil
}

type fixture struct {
	t     *testing.T
	core  *core.Core
	clock *clock.Fake
	sent  fakeSender
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	clk := clock.NewFake(t0)
	return &fixture{t: t, core: core.New(store.NewMemory(), clk, &ids.Sequence{}), clock: clk, sent: make(fakeSender, 16)}
}

// start runs notify until the test ends. Its timers are the Silent tick
// and, while events are gathered, the window.
func (f *fixture) start() {
	f.t.Helper()
	c, cancel := context.WithCancel(ctx)
	f.t.Cleanup(cancel)
	notify.Start(c, notify.Config{Source: f.core, Clock: f.clock, Sender: f.sent, BaseURL: base})
}

// project creates Project key with Spec <key>-1 and n Ready Tickets
// <key>-2 onwards.
func (f *fixture) project(key string, n int) {
	f.t.Helper()
	must(f.t)(f.core.CreateProject(ctx, core.CreateProjectInput{Key: key}))
	must(f.t)(f.core.CreateSpec(ctx, key, core.CreateSpecInput{Title: "Login", Body: "body\n"}))
	for i := range n {
		must(f.t)(f.core.CreateTicket(ctx, key, core.CreateTicketInput{Spec: "1", Title: "Task " + string(rune('A'+i)), State: store.Ready}))
	}
}

// claim claims ticket in project and returns the Claim ID.
func (f *fixture) claim(project, ticket string) string {
	f.t.Helper()
	res := must(f.t)(f.core.Claim(ctx, project, core.ClaimInput{Ticket: core.Ref(ticket), SessionID: "s-" + ticket}))
	return res.(core.ClaimResult).ClaimID
}

func (f *fixture) round(project, ticket string) {
	f.t.Helper()
	claim := f.claim(project, ticket)
	must(f.t)(f.core.PostRound(ctx, project, core.PostRoundInput{Ticket: core.Ref(ticket), ClaimID: claim,
		Questions: []core.QuestionInput{{Title: "Which?", Body: "A or B?"}}}))
}

func (f *fixture) failure(project, ticket string) {
	f.t.Helper()
	claim := f.claim(project, ticket)
	must(f.t)(f.core.ReportFailure(ctx, project, core.ReportFailureInput{Ticket: core.Ref(ticket), ClaimID: claim,
		WhatBroke: "tests", Why: "flaky", WhatToFix: "fix", WhereWorkStands: "branch"}))
}

// accepted finishes ticket in project on its own branch and accepts it.
func (f *fixture) accepted(project, ticket string) {
	f.t.Helper()
	claim := f.claim(project, ticket)
	must(f.t)(f.core.Finish(ctx, project, core.FinishInput{Ticket: core.Ref(ticket), ClaimID: claim, Summary: "done", Branch: project + "-" + ticket, Commit: "abc"}))
	must(f.t)(f.core.Accept(core.WithActor(ctx, core.Human), project, core.TicketInput{Ticket: core.Ref(ticket)}))
}

// closeWindow waits for notify to be holding events (Silent tick plus
// window timer) and moves the clock past the window.
func (f *fixture) closeWindow() {
	f.t.Helper()
	f.clock.BlockUntil(2)
	f.clock.Advance(notify.Window)
}

func (f *fixture) want(n notify.Notification) {
	f.t.Helper()
	select {
	case got := <-f.sent:
		if got != n {
			f.t.Fatalf("sent %+v\nwant %+v", got, n)
		}
	case <-time.After(5 * time.Second):
		f.t.Fatalf("nothing sent; want %+v", n)
	}
}

func (f *fixture) wantNothing() {
	f.t.Helper()
	select {
	case got := <-f.sent:
		f.t.Fatalf("unexpected Notification %+v", got)
	default:
	}
}

// must fails the test on a non-nil error from a call's last result.
func must(t *testing.T) func(any, error) any {
	return func(v any, err error) any {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
}

func TestANewRoundSendsOneNotificationNamingTheTicketAfterTheWindow(t *testing.T) {
	f := newFixture(t)
	f.project("acme", 1)
	f.start()
	f.round("acme", "2")
	f.closeWindow()
	f.want(notify.Notification{
		Title: "acme-2 has a new Round", Body: "Task A",
		URL: base + "/?view=inbox&ticket=acme-2",
	})
}

func TestANewFailureReportSendsOneNotification(t *testing.T) {
	f := newFixture(t)
	f.project("acme", 1)
	f.start()
	f.failure("acme", "2")
	f.closeWindow()
	f.want(notify.Notification{
		Title: "acme-2 has a new Failure Report", Body: "Task A",
		URL: base + "/?view=inbox&ticket=acme-2",
	})
}

func TestABurstSendsOneSummary(t *testing.T) {
	f := newFixture(t)
	f.project("acme", 2)
	f.project("beta", 2)
	f.start()
	f.round("acme", "2")
	f.round("acme", "3")
	f.round("beta", "2")
	f.failure("beta", "3")
	f.closeWindow()
	f.want(notify.Notification{
		Title: "Code Central", Body: "3 new Rounds, 1 failure across 2 Projects",
		URL: base + "/?view=inbox",
	})
}

func TestASummaryInOneProjectNamesIt(t *testing.T) {
	f := newFixture(t)
	f.project("acme", 2)
	f.start()
	f.round("acme", "2")
	f.failure("acme", "3")
	f.closeWindow()
	f.want(notify.Notification{Title: "Code Central", Body: "1 new Round, 1 failure in acme", URL: base + "/?view=inbox"})
}

// The next window holds only what happened after the last one closed.
func TestEachWindowStartsEmpty(t *testing.T) {
	f := newFixture(t)
	f.project("acme", 2)
	f.start()
	f.round("acme", "2")
	f.closeWindow()
	f.want(notify.Notification{Title: "acme-2 has a new Round", Body: "Task A", URL: base + "/?view=inbox&ticket=acme-2"})
	f.round("acme", "3")
	f.closeWindow()
	f.want(notify.Notification{Title: "acme-3 has a new Round", Body: "Task B", URL: base + "/?view=inbox&ticket=acme-3"})
}

// In Review and Answered count toward Needs Me but no agent is waiting,
// so they gather nothing: the window that follows holds only the Round.
func TestInReviewAndAnsweredNeverNotify(t *testing.T) {
	f := newFixture(t)
	f.project("acme", 3)
	f.start()
	claim := f.claim("acme", "2")
	must(t)(f.core.Finish(ctx, "acme", core.FinishInput{Ticket: "2", ClaimID: claim, Summary: "done"}))
	f.round("acme", "3")
	f.closeWindow()
	f.want(notify.Notification{Title: "acme-3 has a new Round", Body: "Task B", URL: base + "/?view=inbox&ticket=acme-3"})

	must(t)(f.core.Answer(core.WithActor(ctx, core.Human), "acme", core.AnswerInput{Ticket: "3", Question: 1, Text: "A"}))
	f.round("acme", "4")
	f.closeWindow()
	f.want(notify.Notification{Title: "acme-4 has a new Round", Body: "Task C", URL: base + "/?view=inbox&ticket=acme-4"})
}

func TestAcceptedButNotLandedNeverNotifies(t *testing.T) {
	f := newFixture(t)
	f.project("acme", 2)
	f.start()
	f.accepted("acme", "2")
	f.clock.BlockUntil(1)
	f.clock.Advance(core.NotLandedAfter + notify.Tick)
	f.clock.BlockUntil(1)
	f.clock.Advance(notify.Tick)
	f.round("acme", "3")
	f.closeWindow()
	f.want(notify.Notification{Title: "acme-3 has a new Round", Body: "Task B", URL: base + "/?view=inbox&ticket=acme-3"})
}

func TestToMergeNeverNotifies(t *testing.T) {
	f := newFixture(t)
	f.project("acme", 2)
	must(t)(f.core.SetLandingMethod(core.WithActor(ctx, core.Human), "acme", core.SetLandingMethodInput{Method: store.LandingBranch}))
	f.start()
	f.accepted("acme", "2")
	f.clock.BlockUntil(1)
	f.clock.Advance(notify.Tick)
	f.round("acme", "3")
	f.closeWindow()
	f.want(notify.Notification{Title: "acme-3 has a new Round", Body: "Task B", URL: base + "/?view=inbox&ticket=acme-3"})
}

func TestAClaimTurningSilentNotifiesOncePerCrossing(t *testing.T) {
	f := newFixture(t)
	f.project("acme", 2)
	f.start()
	f.claim("acme", "2")
	f.clock.BlockUntil(1) // the Silent tick
	f.clock.Advance(core.SilentAfter)
	f.closeWindow()
	silent := notify.Notification{Title: "acme-2 has a Silent Claim", Body: "Task A", URL: base + "/?view=inbox&ticket=acme-2"}
	f.want(silent)

	// Still Silent on later ticks: nothing more.
	for range 3 {
		f.clock.BlockUntil(1)
		f.clock.Advance(notify.Tick)
	}
	f.round("acme", "3")
	f.closeWindow()
	f.want(notify.Notification{Title: "acme-3 has a new Round", Body: "Task B", URL: base + "/?view=inbox&ticket=acme-3"})

	// A ping ends the silence; crossing the mark again notifies again.
	f.core.Ping(ctx, "s-2")
	f.clock.BlockUntil(1)
	f.clock.Advance(notify.Tick)
	f.clock.BlockUntil(1)
	f.clock.Advance(core.SilentAfter)
	f.closeWindow()
	f.want(silent)
}

// tickActive moves the clock on by one tick of d, then pings ticket's
// session so its Claim stays active.
func (f *fixture) tickActive(ticket string, d time.Duration) {
	f.t.Helper()
	f.clock.BlockUntil(1)
	f.clock.Advance(d)
	f.core.Ping(ctx, "s-"+ticket)
}

func TestAClaimTurningLongNotifiesOncePerClaim(t *testing.T) {
	f := newFixture(t)
	f.project("acme", 2)
	f.start()
	f.claim("acme", "2")
	for range 3 {
		f.tickActive("2", 20*time.Minute)
	}
	f.tickActive("2", time.Minute) // past core.LongAfter without a move
	f.closeWindow()
	f.want(notify.Notification{Title: "acme-2 has a Long Claim", Body: "Task A", URL: base + "/?view=inbox&ticket=acme-2"})

	// Still Long on later ticks, then Silent, then Long again: the Silent
	// crossing notifies, the Claim never notifies as Long again.
	for range 3 {
		f.tickActive("2", notify.Tick)
	}
	f.clock.BlockUntil(1)
	f.clock.Advance(core.SilentAfter)
	f.closeWindow()
	f.want(notify.Notification{Title: "acme-2 has a Silent Claim", Body: "Task A", URL: base + "/?view=inbox&ticket=acme-2"})
	f.tickActive("2", notify.Tick)
	f.tickActive("2", notify.Tick)
	f.round("acme", "3")
	f.closeWindow()
	f.want(notify.Notification{Title: "acme-3 has a new Round", Body: "Task B", URL: base + "/?view=inbox&ticket=acme-3"})
}

func TestStartupSendsOneSummaryWhenNeedsMeIsNotEmpty(t *testing.T) {
	f := newFixture(t)
	f.project("acme", 3)
	f.project("beta", 1)
	f.round("acme", "2")
	f.failure("acme", "3")
	f.claim("beta", "2")
	f.clock.Advance(core.SilentAfter) // beta-2 is already Silent at startup
	f.start()
	f.want(notify.Notification{Title: "Code Central", Body: "3 Tickets need you across 2 Projects", URL: base + "/?view=inbox"})

	// beta-2 went into the summary, so it never notifies on its own.
	f.clock.BlockUntil(1)
	f.clock.Advance(notify.Tick)
	f.round("acme", "4")
	f.closeWindow()
	f.want(notify.Notification{Title: "acme-4 has a new Round", Body: "Task C", URL: base + "/?view=inbox&ticket=acme-4"})
}

func TestStartupSendsNothingWhenNeedsMeIsEmpty(t *testing.T) {
	f := newFixture(t)
	f.project("acme", 1)
	f.start()
	f.wantNothing()
}

func TestStartupSummaryForOneTicket(t *testing.T) {
	f := newFixture(t)
	f.project("acme", 1)
	f.round("acme", "2")
	f.start()
	f.want(notify.Notification{Title: "Code Central", Body: "1 Ticket needs you in acme", URL: base + "/?view=inbox"})
}

// ciWatcher reports the status of key's CI Watcher, turning CI Tracking
// on first if it is off.
func (f *fixture) ciWatcher(key string, state core.CIWatcherState, message string) {
	f.t.Helper()
	p := must(f.t)(f.core.GetProject(ctx, key)).(core.ProjectView)
	if !p.CI.Tracking {
		must(f.t)(f.core.SetCITracking(core.WithActor(ctx, core.Human), key, core.SetCITrackingInput{Tracking: true, Repository: "acme/web"}))
	}
	must(f.t)(f.core.ReportCIWatcherStatus(ctx, key, core.ReportCIWatcherStatusInput{State: state, Message: message, Fix: "Run gh auth login."}))
}

func TestACIWatcherGoingIntoAnErrorNotifiesOnceNotOncePerPoll(t *testing.T) {
	f := newFixture(t)
	f.project("acme", 1)
	f.start()
	f.ciWatcher("acme", core.CIWatcherWatching, "")
	f.ciWatcher("acme", core.CIWatcherError, "gh isn't logged in")
	f.ciWatcher("acme", core.CIWatcherError, "gh isn't logged in")
	f.closeWindow()
	inError := notify.Notification{Title: "acme's CI Watcher is in error", Body: "gh isn't logged in", URL: base + "/?view=project-options&project=acme"}
	f.want(inError)

	// Another error while in error is no new one; nor is it when the tick
	// looks again.
	f.ciWatcher("acme", core.CIWatcherError, "gh can't read acme/web")
	f.clock.Advance(notify.Tick)
	f.round("acme", "2")
	f.closeWindow()
	f.want(notify.Notification{Title: "acme-2 has a new Round", Body: "Task A", URL: base + "/?view=inbox&ticket=acme-2"})

	// Back to Watching, then into an error again: one more.
	f.ciWatcher("acme", core.CIWatcherWatching, "")
	f.ciWatcher("acme", core.CIWatcherError, "gh isn't logged in")
	f.closeWindow()
	f.want(inError)
	f.wantNothing()
}

// Turning CI Tracking off and on again forgets the status, but an error
// after it still notifies.
func TestACIWatcherErrorAfterCITrackingIsTurnedBackOnNotifies(t *testing.T) {
	f := newFixture(t)
	f.project("acme", 0)
	f.start()
	f.ciWatcher("acme", core.CIWatcherError, "gh isn't logged in")
	f.closeWindow()
	f.want(notify.Notification{Title: "acme's CI Watcher is in error", Body: "gh isn't logged in", URL: base + "/?view=project-options&project=acme"})
	must(t)(f.core.SetCITracking(core.WithActor(ctx, core.Human), "acme", core.SetCITrackingInput{Tracking: false, Repository: "acme/web"}))
	f.ciWatcher("acme", core.CIWatcherError, "gh isn't logged in")
	f.closeWindow()
	f.want(notify.Notification{Title: "acme's CI Watcher is in error", Body: "gh isn't logged in", URL: base + "/?view=project-options&project=acme"})
}

// An error that clears before notify looks still notifies, with its own
// message: the tick catches it when its notice was missed.
func TestACIWatcherErrorThatClearedBeforeNotifyLookedStillNotifies(t *testing.T) {
	f := newFixture(t)
	f.project("acme", 0)
	f.ciWatcher("acme", core.CIWatcherWatching, "")
	f.start()
	f.ciWatcher("acme", core.CIWatcherError, "gh isn't logged in")
	f.ciWatcher("acme", core.CIWatcherWatching, "")
	f.closeWindow()
	f.want(notify.Notification{Title: "acme's CI Watcher is in error", Body: "gh isn't logged in", URL: base + "/?view=project-options&project=acme"})
	f.wantNothing()
}

// pushed turns CI Tracking and Merge + Push on for project and reports
// commit as ticket's pushed commit; ticket must be Accepted.
func (f *fixture) pushed(project, ticket, commit string) {
	f.t.Helper()
	human := core.WithActor(ctx, core.Human)
	if p := must(f.t)(f.core.GetProject(ctx, project)).(core.ProjectView); !p.CI.Tracking {
		must(f.t)(f.core.SetCITracking(human, project, core.SetCITrackingInput{Tracking: true, Repository: "acme/web"}))
		must(f.t)(f.core.SetLandingMethod(human, project, core.SetLandingMethodInput{Method: store.LandingMerge, Push: true, Remote: "origin"}))
	}
	must(f.t)(f.core.ReportPushedCommit(ctx, project, core.ReportPushedCommitInput{Ticket: core.Ref(ticket), Commit: commit}))
}

// ciRun reports one run of ci.yml on commit, tracked against ticket, as
// the CI Watcher.
func (f *fixture) ciRun(project, ticket, commit string, id int64, status store.CIRunStatus, conclusion store.CIConclusion) {
	f.t.Helper()
	must(f.t)(f.core.ReportCIRuns(ctx, project, core.ReportCIRunsInput{Runs: []core.CIRunView{{ID: id, Attempt: 1, Workflow: "ci.yml",
		Commit: commit, Status: status, Conclusion: conclusion, Ticket: project + "-" + ticket}}}))
}

func TestATicketGoingToCIFailedNotifiesOnce(t *testing.T) {
	f := newFixture(t)
	f.project("acme", 2)
	f.accepted("acme", "2")
	f.pushed("acme", "2", "abc123")
	f.start()
	f.ciRun("acme", "2", "abc123", 1, store.CIInProgress, "")
	f.ciRun("acme", "2", "abc123", 1, store.CICompleted, store.CIFailure)
	// Another failed run on the same commit is the same CI failed.
	f.ciRun("acme", "2", "abc123", 2, store.CICompleted, store.CIFailure)
	f.closeWindow()
	f.want(notify.Notification{Title: "acme-2's CI failed", Body: "Task A", URL: base + "/?view=inbox&ticket=acme-2"})

	// Nor does the tick's look say it again.
	f.clock.Advance(notify.Tick)
	f.round("acme", "3")
	f.closeWindow()
	f.want(notify.Notification{Title: "acme-3 has a new Round", Body: "Task B", URL: base + "/?view=inbox&ticket=acme-3"})
	f.wantNothing()
}

// A Ticket already CI failed at startup goes into the startup summary
// rather than notifying on its own.
func TestATicketAlreadyCIFailedAtStartupIsInTheStartupSummary(t *testing.T) {
	f := newFixture(t)
	f.project("acme", 2)
	f.accepted("acme", "2")
	f.pushed("acme", "2", "abc123")
	f.ciRun("acme", "2", "abc123", 1, store.CICompleted, store.CIFailure)
	f.start()
	f.want(notify.Notification{Title: "Code Central", Body: "1 Ticket needs you in acme", URL: base + "/?view=inbox"})
	f.clock.BlockUntil(1)
	f.clock.Advance(notify.Tick)
	f.round("acme", "3")
	f.closeWindow()
	f.want(notify.Notification{Title: "acme-3 has a new Round", Body: "Task B", URL: base + "/?view=inbox&ticket=acme-3"})
	f.wantNothing()
}
