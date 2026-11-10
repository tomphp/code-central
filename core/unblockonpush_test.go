package core_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/store"
)

// Unblock on push (code-central-298): the rule's edge cases that are
// awkward through the Factory. The Factory harness covers the rest.

// gatedBlocker puts acme under Merge + Push with CI Tracking and Unblock
// on push on, and makes Ticket n Accepted with commit abc123 pushed now.
func (f fixture) gatedBlocker(t *testing.T, n int) {
	t.Helper()
	f.gatedBlockerWith(t, n, true)
}

// gatedBlockerWith is gatedBlocker with Unblock on push set to
// unblockOnPush.
func (f fixture) gatedBlockerWith(t *testing.T, n int, unblockOnPush bool) {
	t.Helper()
	err := f.store.Update(func(w store.Writer) error {
		p, _ := w.Project("acme")
		p.Landing = store.Landing{Method: store.LandingMerge, Push: true, Remote: "origin"}
		p.CI = store.CI{Tracking: true, Provider: store.CIGitHubActions, Repository: "acme/web", UnblockOnPush: unblockOnPush}
		return w.PutProject(p)
	})
	if err != nil {
		t.Fatal(err)
	}
	err = f.store.Update(func(w store.Writer) error {
		tk, _ := w.Ticket("acme", n)
		tk.State = store.Accepted
		tk.PushedCommits = []store.PushedCommit{{Commit: "abc123", Pushed: f.clock.Now()}}
		return w.PutTicket("acme", tk)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// noticesOf are the change notices sent while do runs.
func (f fixture) noticesOf(t *testing.T, do func()) map[events.Event]bool {
	t.Helper()
	notices, stop := f.core.Events().Subscribe()
	defer stop()
	do()
	got := map[events.Event]bool{}
	for len(notices) > 0 {
		got[<-notices] = true
	}
	return got
}

// reportRun reports, as the CI Watcher, a run of ci.yml on abc123
// tracked against ticket.
func (f fixture) reportRun(t *testing.T, ticket string, status store.CIRunStatus, conclusion store.CIConclusion) {
	t.Helper()
	started := f.clock.Now()
	run := core.CIRunView{ID: 1, Attempt: 1, Workflow: "ci.yml", Name: "CI", Branch: "main", Commit: "abc123",
		Status: status, Conclusion: conclusion, Started: &started, URL: "https://github.com/acme/web/actions/runs/1", Ticket: ticket}
	if status == store.CICompleted {
		run.Finished = &started
	}
	if _, err := f.core.ReportCIRuns(ctx, "acme", core.ReportCIRunsInput{Runs: []core.CIRunView{run}}); err != nil {
		t.Fatal(err)
	}
}

func TestUnderUnblockOnPushAGatedBlockerWithNoCIResultAfterPruningBlocks(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready, store.Ready) // acme-2, acme-3
	f.addBlocker(t, "acme-2", "acme-3")
	f.gatedBlocker(t, 3)
	f.reportRun(t, "acme-3", store.CICompleted, store.CISuccess)
	if v := f.get(t, "acme-2"); v.Blocked || !v.Claimable {
		t.Fatalf("blocked %v claimable %v, want acme-3 cleared with its CI passed", v.Blocked, v.Claimable)
	}

	f.clock.Advance(31 * 24 * time.Hour)
	if v := f.get(t, "acme-2"); !v.Blocked || v.Claimable {
		t.Fatalf("blocked %v claimable %v, want acme-3 blocking once its runs are pruned", v.Blocked, v.Claimable)
	}
}

func TestUnderUnblockOnPushACancelledBlockersGatedReplacementClearsAndBlocksAgain(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready, store.Ready, store.Ready) // acme-2..acme-4
	f.addBlocker(t, "acme-2", "acme-3")
	f.cancel(t, core.CancelInput{Ticket: "acme-3", Replacements: []core.Ref{"acme-4"}})
	f.gatedBlocker(t, 4)
	if v := f.get(t, "acme-2"); v.Blocked || !v.Claimable {
		t.Fatalf("blocked %v claimable %v, want the Replacement acme-4 cleared while its CI runs", v.Blocked, v.Claimable)
	}

	got := f.noticesOf(t, func() { f.reportRun(t, "acme-4", store.CICompleted, store.CIFailure) })
	if !got[events.Event{Type: events.TicketChanged, Project: "acme", ID: "acme-2"}] {
		t.Fatalf("notices %v, want ticket.changed for acme-2, whose Blocker's CI result changed", got)
	}

	_, err := f.core.Claim(ctx, "acme", core.ClaimInput{Ticket: "acme-2"})
	var e *core.Error
	if !errors.As(err, &e) || e.Code != core.ActionNotAllowed || !strings.Contains(e.Message, "acme-4 (Accepted, CI failed, replacing Cancelled acme-3)") {
		t.Fatalf("claim: %v, want it refused naming acme-4 with its CI failed", err)
	}
}

func TestACIRunReportThatLeavesTheCIResultAloneSendsNoNoticeToDependants(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready, store.Ready) // acme-2, acme-3
	f.addBlocker(t, "acme-2", "acme-3")
	f.gatedBlocker(t, 3)
	f.reportRun(t, "acme-3", store.CIQueued, "")

	notices, stop := f.core.Events().Subscribe()
	defer stop()
	f.reportRun(t, "acme-3", store.CIInProgress, "")
	for len(notices) > 0 {
		if e := <-notices; e.ID == "acme-2" {
			t.Fatalf("notice %v for acme-2, though acme-3's CI result is still running", e)
		}
	}
}

// A dependant's Blockers show a gated Blocker's CI result whichever way
// Unblock on push is set (code-central-299), so its change notices it.
func TestWithUnblockOnPushOffADependantIsNoticedWhenItsBlockersCIResultChanges(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready, store.Ready) // acme-2, acme-3
	f.addBlocker(t, "acme-2", "acme-3")
	f.gatedBlockerWith(t, 3, false)

	got := f.noticesOf(t, func() { f.reportRun(t, "acme-3", store.CICompleted, store.CISuccess) })
	if !got[events.Event{Type: events.TicketChanged, Project: "acme", ID: "acme-2"}] {
		t.Fatalf("notices %v, want ticket.changed for acme-2, whose Blocker's CI result changed", got)
	}
	if b := f.get(t, "acme-2").Blockers[0]; b.CIResult != core.CIPassed || b.Cleared {
		t.Fatalf("blocker %+v, want acme-3 with its CI passed, not cleared", b)
	}
}
