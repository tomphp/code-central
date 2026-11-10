package frontdoor_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/store"
)

// The CI view (code-central-258): GET /api/ci reads one Project's CI
// Runs, or every active Project's, with each CI Watcher's status; and CI
// Runs finished more than 30 days ago are pruned.

func readCI(t *testing.T, srv *codecentraltest.Server, query string) core.CIOverview {
	t.Helper()
	status, b := httpDo(t, srv, httpCall{method: "GET", path: "/api/ci" + query})
	if status != http.StatusOK {
		t.Fatalf("GET /api/ci%s: status %d: %s", query, status, b)
	}
	var v core.CIOverview
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("not the CI view's shape: %v: %s", err, b)
	}
	return v
}

// after is the time d after the test clock's start.
func after(d time.Duration) *time.Time {
	t := codecentraltest.Start.Add(d)
	return &t
}

// started is a run of workflow tracked against acme-3, started at
// started and still in progress.
func started(id int64, workflow string, started time.Duration) core.CIRunView {
	run := ciRun(id, workflow, "abc123", store.CIInProgress, "")
	run.Started = after(started)
	return run
}

// finished is a run of workflow tracked against acme-3 that ran from
// started to finished and ended with conclusion.
func finished(id int64, workflow string, started, finished time.Duration, conclusion store.CIConclusion) core.CIRunView {
	run := ciRun(id, workflow, "abc123", store.CICompleted, conclusion)
	run.Started, run.Finished = after(started), after(finished)
	return run
}

// runKeys are the view's runs as project/workflow#attempt.
func runKeys(v core.CIOverview) []string {
	out := []string{}
	for _, r := range v.Runs {
		out = append(out, r.Project+"/"+r.Workflow+"#"+strconv.Itoa(r.Attempt))
	}
	return out
}

func TestTheCIViewListsInProgressRunsFirstThenTheLastSevenDaysFinished(t *testing.T) {
	srv := newServer(t, pushedAccepted)
	reportRuns(t, srv,
		finished(1, "old.yml", 0, time.Hour, store.CISuccess),
		finished(2, "lint.yml", 0, 2*24*time.Hour, store.CIFailure),
		finished(3, "ci.yml", 0, 3*24*time.Hour, store.CISuccess),
		started(4, "deploy.yml", 7*24*time.Hour),
		started(5, "e2e.yml", 7*24*time.Hour+time.Hour),
	)
	// A re-run is the run's next attempt: only the latest is listed.
	rerun := finished(2, "lint.yml", 0, 7*24*time.Hour+2*time.Hour, store.CISuccess)
	rerun.Attempt = 2
	reportRuns(t, srv, rerun)
	srv.Clock.Advance(7*24*time.Hour + 3*time.Hour)

	v := readCI(t, srv, "?project=acme")
	want := []string{"acme/e2e.yml#1", "acme/deploy.yml#1", "acme/lint.yml#2", "acme/ci.yml#1"}
	if got := runKeys(v); !slices.Equal(got, want) {
		t.Fatalf("runs %v, want %v: in progress newest first, then finished in the last 7 days newest first", got, want)
	}
	if r := v.Runs[2]; r.Ticket != "acme-3" || r.Conclusion != store.CISuccess || r.Commit != "abc123" || r.URL == "" {
		t.Fatalf("row %+v, want everything the CI Watcher reported of it", r)
	}
	if len(v.Projects) != 1 || v.Projects[0].Project != "acme" || !v.Projects[0].Tracking || v.Projects[0].Repository != "acme/web" ||
		v.Projects[0].Watcher.State != core.WatcherNotReported {
		t.Fatalf("projects %+v, want acme's CI Watcher, not reported yet", v.Projects)
	}
}

func TestTheCIViewMarksIgnoredRunsAndRunsTrackedAgainstACIRule(t *testing.T) {
	srv := newServer(t, pushedAccepted)
	httpOK(t, srv, addCIRule(`{"workflow":"ci.yml","branch":"main"}`))
	ruled := finished(2, "ci.yml", 0, time.Minute, store.CIFailure)
	ruled.Ticket, ruled.Rule, ruled.Commit = "", "acme-4", "fff999"
	reportRuns(t, srv, finished(1, "nightly.yml", 0, 2*time.Minute, store.CISuccess), ruled)

	v := readCI(t, srv, "?project=acme")
	if len(v.Runs) != 2 || !v.Runs[0].Ignored || v.Runs[0].Workflow != "nightly.yml" {
		t.Fatalf("runs %+v, want nightly.yml first, ignored", v.Runs)
	}
	if r := v.Runs[1]; r.Rule != "acme-4" || r.Ticket != "" || r.Ignored {
		t.Fatalf("row %+v, want tracked against the CI Rule on acme-4", r)
	}
}

func TestTheCIViewForAllProjectsHasEveryActiveProjectWithCITrackingOn(t *testing.T) {
	srv := newServer(t, pushedAccepted)
	reportRuns(t, srv, started(1, "ci.yml", 0))
	httpOK(t, srv, httpCall{"POST", "/api/p/other/set_ci_tracking", `{"tracking":true,"repository":"acme/other"}`})
	httpOK(t, srv, httpCall{"POST", "/api/p/other/add_ci_rule", `{"workflow":"ci.yml","branch":"main"}`})
	ruled := started(2, "ci.yml", time.Minute)
	ruled.Ticket, ruled.Rule = "", "other-1"
	must(t, func() error {
		_, err := srv.Client.ReportCIRuns(context.Background(), "other", core.ReportCIRunsInput{Runs: []core.CIRunView{ruled}})
		return err
	})
	must(t, func() error {
		_, err := srv.Client.ReportCIWatcherStatus(context.Background(), "other", core.ReportWatcherStatusInput{State: core.WatcherWatching})
		return err
	})
	must(t, func() error {
		_, err := srv.Core.CreateProject(context.Background(), core.CreateProjectInput{Key: "quiet"})
		return err
	})

	v := readCI(t, srv, "")
	var projects []string
	for _, p := range v.Projects {
		projects = append(projects, p.Project+":"+string(p.Watcher.State))
	}
	if want := []string{"acme:not_reported", "other:watching"}; !slices.Equal(projects, want) {
		t.Fatalf("projects %v, want %v: quiet has CI Tracking off", projects, want)
	}
	if got := runKeys(v); !slices.Equal(got, []string{"other/ci.yml#1", "acme/ci.yml#1"}) {
		t.Fatalf("runs %v, want both Projects' runs, newest started first", got)
	}

	// An Archived Project's CI Watcher is stopped: All Projects leaves it
	// out, though its own read still has its runs.
	must(t, func() error {
		_, err := srv.Core.ArchiveProject(core.WithActor(context.Background(), core.Human), "other")
		return err
	})
	if got := runKeys(readCI(t, srv, "")); !slices.Equal(got, []string{"acme/ci.yml#1"}) {
		t.Fatalf("runs %v, want other's left out once Archived", got)
	}
	if got := runKeys(readCI(t, srv, "?project=other")); !slices.Equal(got, []string{"other/ci.yml#1"}) {
		t.Fatalf("other's runs %v, want its run", got)
	}
}

func TestTheCIViewOfAProjectWithCITrackingOffSaysSo(t *testing.T) {
	srv := newServer(t)
	v := readCI(t, srv, "?project=acme")
	if len(v.Projects) != 1 || v.Projects[0].Tracking || len(v.Runs) != 0 {
		t.Fatalf("view %+v, want acme with CI Tracking off and no runs", v)
	}
}

func TestTheCIViewRefusesAnUnknownProjectOrParameter(t *testing.T) {
	srv := newServer(t)
	httpRejected(t, srv, httpCall{method: "GET", path: "/api/ci?project=nope"}, core.NotFound)
	httpRejected(t, srv, httpCall{method: "GET", path: "/api/ci?days=7"}, core.Invalid)
}

func TestCIRunsFinishedMoreThanThirtyDaysAgoArePruned(t *testing.T) {
	srv := newServer(t, pushedAccepted)
	reportRuns(t, srv, finished(1, "ci.yml", 0, time.Hour, store.CISuccess), started(2, "slow.yml", 0))

	srv.Clock.Advance(30*24*time.Hour + 30*time.Minute)
	if tk := ticket3(t, srv); len(tk.CIRuns) != 2 {
		t.Fatalf("runs %+v, want both kept until 30 days after it finished", tk.CIRuns)
	}
	srv.Clock.Advance(time.Hour)
	// The CI Watcher's next report prunes the store; the reads leave the
	// run out from the moment it is due.
	tk := ticket3(t, srv)
	if len(tk.CIRuns) != 1 || tk.CIRuns[0].Workflow != "slow.yml" {
		t.Fatalf("runs %+v, want only the unfinished one", tk.CIRuns)
	}
	reportRuns(t, srv, started(3, "next.yml", 31*24*time.Hour))
	var stored []store.CIRun
	must(t, func() error {
		return srv.Store.View(func(r store.Reader) error { stored = r.CIRuns("acme"); return nil })
	})
	if len(stored) != 2 {
		t.Fatalf("stored runs %+v, want ci.yml pruned from the store", stored)
	}
	if got := runKeys(readCI(t, srv, "?project=acme")); !slices.Equal(got, []string{"acme/next.yml#1", "acme/slow.yml#1"}) {
		t.Fatalf("runs %v, want ci.yml pruned", got)
	}
}

func TestATicketWhoseRunsWerePrunedHasNoCIResult(t *testing.T) {
	srv := newServer(t, pushedAccepted)
	reportRuns(t, srv, finished(1, "ci.yml", 0, time.Hour, store.CISuccess))
	srv.Clock.Advance(31 * 24 * time.Hour)
	tk := ticket3(t, srv)
	if tk.PushedCommit == nil || tk.CIResult != "" || tk.CIRuns != nil {
		t.Fatalf("pushed %+v, ci result %q, runs %+v: want the pushed commit kept, with no result and no runs", tk.PushedCommit, tk.CIResult, tk.CIRuns)
	}
}

func TestReportedRunsSendTheCIViewAChangeNotice(t *testing.T) {
	srv := newServer(t, pushedAccepted)
	httpOK(t, srv, addCIRule(`{"workflow":"ci.yml","branch":"main"}`))
	sub, stop := srv.Core.Events().Subscribe()
	defer stop()
	ruled := started(1, "ci.yml", 0)
	ruled.Ticket, ruled.Rule = "", "acme-4"
	reportRuns(t, srv, ruled)
	awaitNotice(t, sub, "ci.changed on acme", func(e events.Event) bool { return e.Type == events.CIChanged && e.Project == "acme" })
}

// awaitNotice waits up to 5 seconds for a notice on sub that matches,
// failing the test, naming want, when none comes.
func awaitNotice(t *testing.T, sub <-chan events.Event, want string, matches func(events.Event) bool) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case e := <-sub:
			if matches(e) {
				return
			}
		case <-deadline:
			t.Fatalf("no %s notice", want)
		}
	}
}

// The CI view's running read (code-central-276): GET /api/ci?running=true
// has only the CI Runs that count as running towards a Ticket's CI result.

func TestTheRunningReadHasTheUnfinishedRunsCountingTowardsATicketsCIResult(t *testing.T) {
	srv := newServer(t, pushedAccepted)
	httpOK(t, srv, addCIRule(`{"workflow":"ci.yml","branch":"main"}`))
	queued := started(2, "queued.yml", 2*time.Minute)
	queued.Status = store.CIQueued
	ruled := started(5, "ci.yml", 5*time.Minute)
	ruled.Ticket, ruled.Rule = "", "acme-4"
	// A re-run's latest attempt replaces its earlier ones: one still
	// going, the other finished.
	rerunGoing := started(6, "rerun.yml", 6*time.Minute)
	rerunGoing.Attempt = 2
	rerunDone := finished(7, "redone.yml", 7*time.Minute, 8*time.Minute, store.CISuccess)
	rerunDone.Attempt = 2
	reportRuns(t, srv,
		started(1, "ci.yml", time.Minute),
		queued,
		finished(3, "lint.yml", 3*time.Minute, 4*time.Minute, store.CISuccess),
		started(4, "nightly.yml", 4*time.Minute),
		ruled,
		started(6, "rerun.yml", 0),
		rerunGoing,
		started(7, "redone.yml", 0),
		rerunDone,
	)

	v := readCI(t, srv, "?running=true")
	want := []string{"acme/rerun.yml#2", "acme/queued.yml#1", "acme/ci.yml#1"}
	if got := runKeys(v); !slices.Equal(got, want) {
		t.Fatalf("runs %v, want %v: queued and in progress, each at its latest attempt, no CI Rule's or Ignored workflow's run, newest started first", got, want)
	}
	if len(v.Projects) != 1 || v.Projects[0].Project != "acme" || !v.Projects[0].Tracking {
		t.Fatalf("projects %+v, want acme's, as without running", v.Projects)
	}
	if got := runKeys(readCI(t, srv, "?running=true&project=acme")); !slices.Equal(got, want) {
		t.Fatalf("acme's runs %v, want %v", got, want)
	}
	if got := runKeys(readCI(t, srv, "?running=false&project=acme")); len(got) != 7 {
		t.Fatalf("runs %v, want every run's latest attempt without the filter", got)
	}
}

func TestTheRunningReadLeavesOutRunsOnATicketsEarlierPushedCommit(t *testing.T) {
	srv := newServer(t, pushedAccepted)
	reportRuns(t, srv, started(1, "ci.yml", 0), ciRun(2, "old.yml", "fff999", store.CIInProgress, ""))
	if got := runKeys(readCI(t, srv, "?running=true")); !slices.Equal(got, []string{"acme/ci.yml#1"}) {
		t.Fatalf("runs %v, want only abc123's run: fff999 isn't acme-3's pushed commit", got)
	}

	pushCommit(t, srv, "def456")
	if got := runKeys(readCI(t, srv, "?running=true")); len(got) != 0 {
		t.Fatalf("runs %v, want abc123's run dropped once def456 is pushed", got)
	}
	reportRuns(t, srv, ciRun(3, "ci.yml", "def456", store.CIQueued, ""))
	if got := runKeys(readCI(t, srv, "?running=true")); !slices.Equal(got, []string{"acme/ci.yml#1"}) || readCI(t, srv, "?running=true").Runs[0].ID != 3 {
		t.Fatalf("runs %v, want def456's run", got)
	}
}

func TestTheRunningReadLeavesOutRunsOfATicketWithNoPushedCommit(t *testing.T) {
	srv := newServer(t, trackedAccepted)
	reportRuns(t, srv, started(1, "ci.yml", 0))
	if got := runKeys(readCI(t, srv, "?running=true")); len(got) != 0 {
		t.Fatalf("runs %v, want none: acme-3 has no pushed commit", got)
	}
}

func TestTheRunningReadForAllProjectsHasEveryActiveProjectWithCITrackingOn(t *testing.T) {
	srv := newServer(t, pushedAccepted, pushedInOther)
	reportRuns(t, srv, started(1, "ci.yml", 0))
	going := ciRun(2, "e2e.yml", "fed321", store.CIInProgress, "")
	going.Ticket, going.Started = "other-2", after(time.Minute)
	must(t, func() error {
		_, err := srv.Client.ReportCIRuns(context.Background(), "other", core.ReportCIRunsInput{Runs: []core.CIRunView{going}})
		return err
	})

	if got := runKeys(readCI(t, srv, "?running=true")); !slices.Equal(got, []string{"other/e2e.yml#1", "acme/ci.yml#1"}) {
		t.Fatalf("runs %v, want both Projects' runs, newest started first", got)
	}
	if got := runKeys(readCI(t, srv, "?project=other&running=true")); !slices.Equal(got, []string{"other/e2e.yml#1"}) {
		t.Fatalf("other's runs %v, want its run alone", got)
	}

	// The Ticket's State doesn't matter: Done, its run still counts.
	must(t, func() error {
		_, err := srv.Client.Land(context.Background(), "other", core.LandInput{Ticket: "other-2", Commit: "fed321"})
		return err
	})
	if got := runKeys(readCI(t, srv, "?project=other&running=true")); !slices.Equal(got, []string{"other/e2e.yml#1"}) {
		t.Fatalf("other's runs %v, want its run, other-2 Done", got)
	}
	must(t, func() error {
		_, err := srv.Core.ArchiveProject(core.WithActor(context.Background(), core.Human), "other")
		return err
	})
	if got := runKeys(readCI(t, srv, "?running=true")); !slices.Equal(got, []string{"acme/ci.yml#1"}) {
		t.Fatalf("runs %v, want other's left out once Archived", got)
	}
	httpOK(t, srv, setCITracking(`{"tracking":false}`))
	if v := readCI(t, srv, "?running=true"); len(v.Runs) != 0 || len(v.Projects) != 0 {
		t.Fatalf("view %+v, want nothing once acme's CI Tracking is off", v)
	}
}

func TestTheRunningReadRefusesAValueOtherThanTrueOrFalse(t *testing.T) {
	srv := newServer(t)
	e := httpRejected(t, srv, httpCall{method: "GET", path: "/api/ci?running=yes"}, core.Invalid)
	if e.Field != "running" {
		t.Fatalf("field %q, want running", e.Field)
	}
}

// pushedInOther turns CI Tracking and Merge + Push on for other, and
// reports its Accepted Ticket other-2's pushed commit, fed321.
func pushedInOther(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	ctx := context.Background()
	human := core.WithActor(ctx, core.Human)
	httpOK(t, srv, httpCall{"POST", "/api/p/other/set_ci_tracking", `{"tracking":true,"repository":"acme/other"}`})
	must(t, func() error {
		_, err := srv.Core.SetLandingMethod(human, "other", core.SetLandingMethodInput{Method: store.LandingMerge, Push: true, Remote: "origin"})
		return err
	})
	must(t, func() error {
		_, err := srv.Core.CreateSpec(ctx, "other", core.CreateSpecInput{Title: "Search"})
		return err
	})
	must(t, func() error {
		_, err := srv.Core.CreateTicket(ctx, "other", core.CreateTicketInput{Spec: "1", Title: "Search box", State: "Ready"})
		return err
	})
	claim, err := srv.Core.Claim(ctx, "other", core.ClaimInput{})
	if err != nil {
		t.Fatal(err)
	}
	must(t, func() error {
		_, err := srv.Core.Finish(ctx, "other", core.FinishInput{Ticket: "2", ClaimID: claim.ClaimID, Summary: "Done.", Branch: "other-2", Commit: "fed321"})
		return err
	})
	must(t, func() error { _, err := srv.Core.Accept(human, "other", core.TicketInput{Ticket: "2"}); return err })
	must(t, func() error {
		_, err := srv.Client.ReportPushedCommit(ctx, "other", core.ReportPushedCommitInput{Ticket: "other-2", Commit: "fed321"})
		return err
	})
}
