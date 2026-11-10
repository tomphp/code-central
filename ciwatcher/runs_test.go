package ciwatcher_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/ciwatcher"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/internal/fakegh"
	"github.com/tomoram/code-central/store"
)

// pushed makes acme-2 an Accepted Ticket whose pushed commit is commit,
// under Merge + Push, as the Factory would report it.
func (w *watching) pushed(commit string) string {
	w.t.Helper()
	ctx := context.Background()
	human := core.WithActor(ctx, core.Human)
	c := w.srv.Core
	must(w.t, func() error {
		_, err := c.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingMerge, Push: true, Remote: "origin"})
		return err
	})
	must(w.t, func() error { _, err := c.CreateSpec(ctx, "acme", core.CreateSpecInput{Title: "Login"}); return err })
	must(w.t, func() error {
		_, err := c.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "Login form", State: store.Ready})
		return err
	})
	res, err := c.Claim(ctx, "acme", core.ClaimInput{Ticket: "2"})
	if err != nil {
		w.t.Fatal(err)
	}
	must(w.t, func() error {
		_, err := c.Finish(ctx, "acme", core.FinishInput{Ticket: "2", ClaimID: res.ClaimID, Summary: "Done.", Branch: "acme-2", Commit: commit})
		return err
	})
	must(w.t, func() error { _, err := c.Accept(human, "acme", core.TicketInput{Ticket: "2"}); return err })
	must(w.t, func() error {
		_, err := w.srv.Client.ReportPushedCommit(ctx, "acme", core.ReportPushedCommitInput{Ticket: "acme-2", Commit: commit})
		return err
	})
	return "acme-2"
}

func must(t *testing.T, f func() error) {
	t.Helper()
	if err := f(); err != nil {
		t.Fatal(err)
	}
}

// ghRun is one workflow run as GitHub's REST API lists it.
type ghRun struct {
	id                         int64
	attempt                    int
	workflow, status, conclude string
	updated                    string
}

// runsAnswer scripts GitHub's answer to listing commit's runs.
func (w *watching) runsAnswer(commit string, runs ...ghRun) {
	w.t.Helper()
	var items []string
	for _, r := range runs {
		conclusion := "null"
		if r.conclude != "" {
			conclusion = fmt.Sprintf("%q", r.conclude)
		}
		items = append(items, fmt.Sprintf(`{"id":%d,"run_attempt":%d,"name":"Build %s","path":".github/workflows/%s","head_branch":"main","head_sha":%q,`+
			`"status":%q,"conclusion":%s,"run_started_at":"2026-01-05T09:01:00Z","updated_at":%q,"html_url":"https://github.com/acme/web/actions/runs/%d"}`,
			r.id, r.attempt, r.workflow, r.workflow, commit, r.status, conclusion, r.updated, r.id))
	}
	w.h.Answer(fakegh.Answer{Args: []string{"api", "repos/acme/web/actions/runs?head_sha=" + commit + "&per_page=100"},
		Stdout: `{"total_count":` + fmt.Sprint(len(runs)) + `,"workflow_runs":[` + strings.Join(items, ",") + `]}`})
}

// jobsAnswer scripts GitHub's answer to listing a run's jobs, each a
// name and its conclusion, "" while it runs.
func (w *watching) jobsAnswer(id int64, attempt int, jobs ...[2]string) {
	w.t.Helper()
	var items []string
	for i, j := range jobs {
		status, conclusion := "completed", fmt.Sprintf("%q", j[1])
		if j[1] == "" {
			status, conclusion = "in_progress", "null"
		}
		items = append(items, fmt.Sprintf(`{"name":%q,"status":%q,"conclusion":%s,"html_url":"https://github.com/acme/web/actions/runs/%d/job/%d"}`, j[0], status, conclusion, id, i+1))
	}
	w.h.Answer(fakegh.Answer{Args: []string{"api", fmt.Sprintf("repos/acme/web/actions/runs/%d/attempts/%d/jobs?per_page=100", id, attempt)},
		Stdout: `{"jobs":[` + strings.Join(items, ",") + `]}`})
}

// ciResult waits for ticket's CI result to be want, with runs reported,
// and returns the Ticket.
func (w *watching) ciResult(ticket string, want core.CIResult) core.TicketView {
	w.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		tk, err := w.srv.Client.GetTicket(context.Background(), "acme", core.Ref(ticket))
		if err != nil {
			w.t.Fatal(err)
		}
		if tk.CIResult == want && len(tk.CIRuns) > 0 {
			return tk
		}
		if time.Now().After(deadline) {
			w.t.Fatalf("%s's ci result %q with runs %+v, want %q", ticket, tk.CIResult, tk.CIRuns, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// listed counts the fake gh's calls listing commit's runs.
func (w *watching) listed(commit string) int {
	n := 0
	for _, c := range w.h.Calls() {
		if len(c.Args) == 2 && strings.Contains(c.Args[1], "head_sha="+commit) {
			n++
		}
	}
	return n
}

func TestTheCIWatcherReportsAPushedCommitsRunsAgainstItsTicket(t *testing.T) {
	w := newWatching(t)
	id := w.pushed("abc123")
	w.runsAnswer("abc123",
		ghRun{id: 11, attempt: 1, workflow: "ci.yml", status: "completed", conclude: "success", updated: "2026-01-05T09:05:00Z"},
		ghRun{id: 12, attempt: 1, workflow: "nightly.yml", status: "completed", conclude: "failure", updated: "2026-01-05T09:04:00Z"})
	w.jobsAnswer(11, 1, [2]string{"test", "success"}, [2]string{"lint", "skipped"})
	w.jobsAnswer(12, 1, [2]string{"build", "failure"})
	human := core.WithActor(context.Background(), core.Human)
	must(t, func() error {
		_, err := w.srv.Core.SetCITracking(human, "acme", core.SetCITrackingInput{Tracking: true, Repository: "acme/web", IgnoredWorkflows: []string{"nightly.yml"}})
		return err
	})
	w.start(w.h.Bin)

	tk := w.ciResult(id, core.CIPassed)
	if len(tk.CIRuns) != 2 {
		t.Fatalf("runs = %+v, want both reported", tk.CIRuns)
	}
	ci, nightly := tk.CIRuns[0], tk.CIRuns[1]
	if ci.ID != 11 || ci.Workflow != "ci.yml" || ci.Name != "Build ci.yml" || ci.Branch != "main" || ci.Commit != "abc123" || ci.Ticket != id ||
		ci.Status != store.CICompleted || ci.Conclusion != store.CISuccess || ci.URL != "https://github.com/acme/web/actions/runs/11" ||
		ci.Started == nil || ci.Finished == nil || !ci.Finished.Equal(time.Date(2026, 1, 5, 9, 5, 0, 0, time.UTC)) || ci.Ignored {
		t.Fatalf("ci.yml's run = %+v", ci)
	}
	if len(ci.Jobs) != 2 || ci.Jobs[0] != (core.CIJobView{Name: "test", Conclusion: store.CISuccess, URL: "https://github.com/acme/web/actions/runs/11/job/1"}) ||
		ci.Jobs[1].Conclusion != store.CISkipped {
		t.Fatalf("ci.yml's jobs = %+v", ci.Jobs)
	}
	if !nightly.Ignored || nightly.Conclusion != store.CIFailure {
		t.Fatalf("nightly.yml's run = %+v, want its failure kept and marked ignored", nightly)
	}
}

func TestTheCIWatcherReportsARunAgainOnlyOnceItChanges(t *testing.T) {
	w := newWatching(t)
	id := w.pushed("abc123")
	w.runsAnswer("abc123", ghRun{id: 11, attempt: 1, workflow: "ci.yml", status: "in_progress", updated: "2026-01-05T09:02:00Z"})
	w.jobsAnswer(11, 1, [2]string{"test", ""})
	w.start(w.h.Bin)
	tk := w.ciResult(id, core.CIRunning)
	if tk.CIRuns[0].Status != store.CIInProgress || tk.CIRuns[0].Jobs[0].Conclusion != "" {
		t.Fatalf("run = %+v, want in progress with its job unfinished", tk.CIRuns[0])
	}

	// Unchanged, its jobs aren't fetched again.
	w.waitingPoll()
	w.waitingPoll()
	jobs := 0
	for _, c := range w.h.Calls() {
		if len(c.Args) == 2 && strings.Contains(c.Args[1], "/jobs") {
			jobs++
		}
	}
	if jobs != 1 {
		t.Fatalf("fetched the jobs %d times, want once for an unchanged run", jobs)
	}

	w.runsAnswer("abc123", ghRun{id: 11, attempt: 1, workflow: "ci.yml", status: "completed", conclude: "failure", updated: "2026-01-05T09:06:00Z"})
	w.jobsAnswer(11, 1, [2]string{"test", "failure"})
	w.waitingPoll()
	tk = w.ciResult(id, core.CIFailed)
	if tk.CIRuns[0].Jobs[0].Conclusion != store.CIFailure {
		t.Fatalf("run = %+v, want its job failed", tk.CIRuns[0])
	}
}

// waitingPoll waits for the loop to wait out its interval while a pushed
// commit waits, then ends it.
func (w *watching) waitingPoll() {
	w.t.Helper()
	n := w.listed("abc123")
	w.srv.Clock.BlockUntil(1)
	w.srv.Clock.Advance(ciwatcher.WaitingInterval)
	deadline := time.Now().Add(10 * time.Second)
	for w.listed("abc123") == n {
		if time.Now().After(deadline) {
			w.t.Fatal("no poll after the waiting interval")
		}
		time.Sleep(10 * time.Millisecond)
	}
	w.srv.Clock.BlockUntil(1)
}

func TestTheCIWatcherPollsEvery30SecondsWhileAPushedCommitWaitsAnd2MinutesOtherwise(t *testing.T) {
	w := newWatching(t)
	w.start(w.h.Bin)
	w.status(core.CIWatcherWatching)

	// Nothing waits: 30 seconds on, the loop still waits.
	w.srv.Clock.BlockUntil(1)
	w.srv.Clock.Advance(ciwatcher.WaitingInterval)
	if n := w.srv.Clock.Waiters(); n != 1 {
		t.Fatalf("%d timers waiting 30 seconds in, want the 2-minute one still", n)
	}
	w.pushed("abc123")
	w.runsAnswer("abc123", ghRun{id: 11, attempt: 1, workflow: "ci.yml", status: "queued", updated: "2026-01-05T09:02:00Z"})
	w.jobsAnswer(11, 1)
	w.srv.Clock.Advance(ciwatcher.Interval - ciwatcher.WaitingInterval)
	deadline := time.Now().Add(10 * time.Second)
	for w.listed("abc123") == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the pushed commit's runs were never listed")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Something waits: every 30 seconds.
	w.waitingPoll()
	w.waitingPoll()
}
