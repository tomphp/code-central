package core_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/store"
)

// A Blocker that is Accepted and waiting on CI blocks until it lands,
// whatever its CI result (ADR 0019), so the Blocker rule reads no CI Runs.

// gatedBlocker puts acme under Merge + Push with CI Tracking on, and makes
// Ticket n Accepted with commit abc123 pushed now.
func (f fixture) gatedBlocker(t *testing.T, n int) {
	t.Helper()
	err := f.store.Update(func(w store.Writer) error {
		p, _ := w.Project("acme")
		p.Landing = store.Landing{Method: store.LandingMerge, Push: true, Remote: "origin"}
		p.CI = store.CI{Tracking: true, Provider: store.CIGitHubActions, Repository: "acme/web"}
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

func TestATicketBlockedByAnAcceptedTicketStaysBlockedWhateverItsCIResult(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready, store.Ready) // acme-2, acme-3
	f.addBlocker(t, "acme-2", "acme-3")
	f.gatedBlocker(t, 3)
	for _, c := range []struct {
		status     store.CIRunStatus
		conclusion store.CIConclusion
	}{{store.CIInProgress, ""}, {store.CICompleted, store.CISuccess}, {store.CICompleted, store.CIFailure}} {
		f.reportRun(t, "acme-3", c.status, c.conclusion)
		if v := f.get(t, "acme-2"); !v.Blocked || v.Claimable {
			t.Fatalf("blocked %v claimable %v, want acme-3 blocking with its run %s %s", v.Blocked, v.Claimable, c.status, c.conclusion)
		}
	}
	_, err := f.core.Claim(ctx, "acme", core.ClaimInput{Ticket: "acme-2"})
	var e *core.Error
	if !errors.As(err, &e) || e.Code != core.ActionNotAllowed || !strings.Contains(e.Message, "acme-3 (Accepted)") {
		t.Fatalf("claim: %v, want it refused naming acme-3 (Accepted)", err)
	}
}

func TestACIRunReportSendsNoNoticeToDependantsOfTheTicket(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready, store.Ready) // acme-2, acme-3
	f.addBlocker(t, "acme-2", "acme-3")
	f.gatedBlocker(t, 3)

	got := f.noticesOf(t, func() { f.reportRun(t, "acme-3", store.CICompleted, store.CISuccess) })
	if !got[events.Event{Type: events.TicketChanged, Project: "acme", ID: "acme-3"}] {
		t.Fatalf("notices %v, want ticket.changed for acme-3", got)
	}
	if got[events.Event{Type: events.TicketChanged, Project: "acme", ID: "acme-2"}] {
		t.Fatalf("notices %v, want none for acme-2, whose Blocker's CI result it doesn't show", got)
	}
}
