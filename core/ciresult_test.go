package core_test

import (
	"slices"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// A Ticket's CI result under the CI gate (code-central-430): passed if any
// of its pushed commits since it last moved back to Ready passed, else its
// latest pushed commit's, through core's public operations.

// ciResultWorld is acme under landing with CI Tracking on, and acme-2
// Accepted.
func ciResultWorld(t *testing.T, landing store.Landing) fixture {
	t.Helper()
	f := newFixture(t)
	f.work(t, store.Ready) // acme-2
	err := f.store.Update(func(w store.Writer) error {
		p, _ := w.Project("acme")
		p.Landing = landing
		p.CI = store.CI{Tracking: true, Provider: store.CIGitHubActions, Repository: "acme/web"}
		return w.PutProject(p)
	})
	if err != nil {
		t.Fatal(err)
	}
	f.moveTo(t, 2, store.Accepted)
	return f
}

// gate is Merge + Push, under which the CI gate holds.
var gate = store.Landing{Method: store.LandingMerge, Push: true, Remote: "origin"}

// pushed reports, as the Factory, commit as ticket's pushed commit, a
// minute after the last report.
func (f fixture) pushed(t *testing.T, ticket, commit string) {
	t.Helper()
	f.clock.Advance(time.Minute)
	if _, err := f.core.ReportPushedCommit(ctx, "acme", core.ReportPushedCommitInput{Ticket: core.Ref(ticket), Commit: commit, Branch: "main"}); err != nil {
		t.Fatal(err)
	}
}

// ciRun reports, as the CI Watcher, attempt 1 of run id of workflow on
// commit tracked against ticket: one shared run per commit, as the real
// CI Watcher reports.
func (f fixture) ciRun(t *testing.T, id int64, workflow, commit, ticket string, status store.CIRunStatus, conclusion store.CIConclusion) {
	t.Helper()
	f.ciAttempt(t, id, 1, workflow, commit, ticket, status, conclusion)
}

func (f fixture) ciAttempt(t *testing.T, id int64, attempt int, workflow, commit, ticket string, status store.CIRunStatus, conclusion store.CIConclusion) {
	t.Helper()
	started := f.clock.Now()
	run := core.CIRunView{ID: id, Attempt: attempt, Workflow: workflow, Name: workflow, Branch: "main", Commit: commit,
		Status: status, Conclusion: conclusion, Started: &started, URL: "https://github.com/acme/web/actions/runs/1", Ticket: ticket}
	if status == store.CICompleted {
		run.Finished = &started
	}
	if _, err := f.core.ReportCIRuns(ctx, "acme", core.ReportCIRunsInput{Runs: []core.CIRunView{run}}); err != nil {
		t.Fatal(err)
	}
}

// wantCI fails unless ticket's CI result is result, from commit.
func (f fixture) wantCI(t *testing.T, ticket string, result core.CIResult, commit string) core.TicketView {
	t.Helper()
	v := f.getTicket(t, ticket)
	if v.CIResult != result || v.CICommit != commit {
		t.Fatalf("%s: CI result %q from %q, want %q from %q", ticket, v.CIResult, v.CICommit, result, commit)
	}
	return v
}

func TestUnderTheCIGateALaterPushedCommitThatPassedMakesTheCIResultPassed(t *testing.T) {
	f := ciResultWorld(t, gate)
	f.pushed(t, "acme-2", "aaa111")
	f.ciRun(t, 1, "ci.yml", "aaa111", "acme-2", store.CICompleted, store.CIFailure)
	f.pushed(t, "acme-2", "bbb222")
	f.wantCI(t, "acme-2", core.CIRunning, "bbb222")

	f.ciRun(t, 2, "ci.yml", "bbb222", "acme-2", store.CICompleted, store.CISuccess)
	if v := f.wantCI(t, "acme-2", core.CIPassed, "bbb222"); v.NeedsMeReason != "" || v.CIFailureOptions != nil {
		t.Fatalf("reason %q, options %v, want nothing waiting on the human", v.NeedsMeReason, v.CIFailureOptions)
	}
	rows, err := f.core.FindTickets(ctx, "acme", core.FindTicketsInput{State: []store.State{store.Accepted}})
	if err != nil || len(rows) != 1 || rows[0].CIResult != core.CIPassed {
		t.Fatalf("rows %+v (%v), want acme-2's row passed", rows, err)
	}
}

func TestUnderTheCIGateAnEarlierPushedCommitThatPassesAfterALaterOneFailedMakesItPassed(t *testing.T) {
	f := ciResultWorld(t, gate)
	f.pushed(t, "acme-2", "aaa111")
	f.ciRun(t, 1, "ci.yml", "aaa111", "acme-2", store.CIInProgress, "")
	f.pushed(t, "acme-2", "bbb222")
	f.ciRun(t, 2, "ci.yml", "bbb222", "acme-2", store.CICompleted, store.CIFailure)
	if v := f.wantCI(t, "acme-2", core.CIFailed, "bbb222"); v.NeedsMeReason != core.ReasonCIFailed {
		t.Fatalf("reason %q, want CI failed", v.NeedsMeReason)
	}

	f.ciRun(t, 1, "ci.yml", "aaa111", "acme-2", store.CICompleted, store.CISuccess)
	if v := f.wantCI(t, "acme-2", core.CIPassed, "aaa111"); v.NeedsMeReason != "" {
		t.Fatalf("reason %q, want none once a pushed commit passed", v.NeedsMeReason)
	}
}

func TestUnderTheCIGateACommitPushedBeforeTheLastSendBackNeverCounts(t *testing.T) {
	f := ciResultWorld(t, gate)
	f.pushed(t, "acme-2", "aaa111")
	f.ciRun(t, 1, "ci.yml", "aaa111", "acme-2", store.CICompleted, store.CIFailure)
	f.clock.Advance(time.Minute)
	if _, err := f.core.SendBack(human, "acme", core.SendBackInput{Ticket: "acme-2", Note: "Fix CI."}); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Minute)
	f.moveTo(t, 2, store.Accepted)
	f.pushed(t, "acme-2", "bbb222")
	f.ciRun(t, 2, "ci.yml", "bbb222", "acme-2", store.CICompleted, store.CIFailure)

	f.ciAttempt(t, 1, 2, "ci.yml", "aaa111", "acme-2", store.CICompleted, store.CISuccess)
	f.wantCI(t, "acme-2", core.CIFailed, "bbb222")
}

func TestUnderTheCIGateAPassingRunDoesNotPassACommitWhoseOtherWorkflowFailedOrRuns(t *testing.T) {
	for _, c := range []struct {
		status     store.CIRunStatus
		conclusion store.CIConclusion
		want       core.CIResult
	}{
		{store.CICompleted, store.CIFailure, core.CIFailed},
		{store.CIInProgress, "", core.CIRunning},
	} {
		t.Run(string(c.want), func(t *testing.T) {
			f := ciResultWorld(t, gate)
			f.pushed(t, "acme-2", "aaa111")
			f.ciRun(t, 1, "ci.yml", "aaa111", "acme-2", store.CICompleted, store.CISuccess)
			f.ciRun(t, 2, "lint.yml", "aaa111", "acme-2", c.status, c.conclusion)
			f.pushed(t, "acme-2", "bbb222")
			f.ciRun(t, 3, "ci.yml", "bbb222", "acme-2", store.CICompleted, store.CIFailure)
			f.wantCI(t, "acme-2", core.CIFailed, "bbb222")

			f.ciAttempt(t, 3, 2, "ci.yml", "bbb222", "acme-2", store.CICompleted, store.CISuccess)
			f.ciRun(t, 4, "lint.yml", "bbb222", "acme-2", c.status, c.conclusion)
			f.wantCI(t, "acme-2", c.want, "bbb222")
		})
	}
}

func TestOutsideTheCIGateTheCIResultIsTheLatestPushedCommits(t *testing.T) {
	for name, landing := range map[string]store.Landing{
		"branch and push": {Method: store.LandingBranch, Push: true, Remote: "origin"},
		"pull request":    {Method: store.LandingPullRequest, Remote: "origin"},
	} {
		t.Run(name, func(t *testing.T) {
			f := ciResultWorld(t, landing)
			f.pushed(t, "acme-2", "aaa111")
			f.ciRun(t, 1, "ci.yml", "aaa111", "acme-2", store.CIInProgress, "")
			f.pushed(t, "acme-2", "bbb222")
			f.ciRun(t, 2, "ci.yml", "bbb222", "acme-2", store.CICompleted, store.CIFailure)
			f.ciRun(t, 1, "ci.yml", "aaa111", "acme-2", store.CICompleted, store.CISuccess)
			f.wantCI(t, "acme-2", core.CIFailed, "bbb222")
		})
	}
}

// waiting is the CI Watcher's waiting list as ticket@commit.
func (f fixture) waiting(t *testing.T) []string {
	t.Helper()
	w, err := f.core.GetCIWatch(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, c := range w.Waiting {
		out = append(out, c.Ticket+"@"+c.Commit)
	}
	return out
}

func TestUnderTheCIGateTheCIWatcherWaitsOnEveryRunningPushedCommitOnce(t *testing.T) {
	f := ciResultWorld(t, gate)
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Tb"}) // acme-3
	f.moveTo(t, 3, store.Accepted)
	f.pushed(t, "acme-2", "old000")
	f.ciRun(t, 1, "ci.yml", "old000", "acme-2", store.CICompleted, store.CIFailure)
	f.clock.Advance(time.Minute)
	if _, err := f.core.SendBack(human, "acme", core.SendBackInput{Ticket: "acme-2", Note: "Fix CI."}); err != nil {
		t.Fatal(err)
	}
	f.ciAttempt(t, 1, 2, "ci.yml", "old000", "acme-2", store.CIInProgress, "")
	f.moveTo(t, 2, store.Accepted)
	f.pushed(t, "acme-2", "aaa111")
	f.pushed(t, "acme-2", "bbb222")
	f.pushed(t, "acme-3", "bbb222")
	f.pushed(t, "acme-2", "ccc333")
	f.ciRun(t, 2, "ci.yml", "ccc333", "acme-2", store.CICompleted, store.CIFailure)
	f.clock.Advance(10 * time.Minute)

	if got, want := f.waiting(t), []string{"acme-2@aaa111", "acme-2@bbb222"}; !slices.Equal(got, want) {
		t.Fatalf("waiting %v, want %v: each running commit since the send-back once, and not the finished one", got, want)
	}
}

func TestOutsideTheCIGateTheCIWatcherWaitsOnTheLatestPushedCommitOnly(t *testing.T) {
	f := ciResultWorld(t, store.Landing{Method: store.LandingBranch, Push: true, Remote: "origin"})
	f.pushed(t, "acme-2", "aaa111")
	f.pushed(t, "acme-2", "bbb222")
	f.clock.Advance(10 * time.Minute)
	if got, want := f.waiting(t), []string{"acme-2@bbb222"}; !slices.Equal(got, want) {
		t.Fatalf("waiting %v, want %v", got, want)
	}
}

func TestUnderTheCIGateARunOnAnEarlierPushedCommitCountsAsRunning(t *testing.T) {
	f := ciResultWorld(t, gate)
	f.pushed(t, "acme-2", "aaa111")
	f.ciRun(t, 1, "ci.yml", "aaa111", "acme-2", store.CIInProgress, "")
	f.pushed(t, "acme-2", "bbb222")
	v, err := f.core.CIOverview(ctx, core.CIOverviewInput{Project: "acme", Running: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Runs) != 1 || v.Runs[0].Commit != "aaa111" {
		t.Fatalf("running %+v, want aaa111's run, which can still pass acme-2", v.Runs)
	}
}
