package server_test

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/events"
)

// Tracking a pushed Ticket's CI Runs (code-central-256): the Factory's
// report_pushed_commit and the CI Watcher's report_ci_runs, both over
// HTTP only, and the CI result every Ticket read works out from them.
func init() {
	cases["report_pushed_commit"] = opCase{
		setup: trackedAccepted,
		ok:    call{http: reportPushedCommit("3", `{"commit":"abc123"}`)},
		check: ticketAcknowledgement("acme-3", "Accepted", "pushed_commit", "ci_result", "ci_commit"),
		full: func(t *testing.T, out map[string]any) {
			pushed, _ := out["pushed_commit"].(map[string]any)
			if pushed == nil || pushed["commit"] != "abc123" {
				t.Fatalf("pushed_commit = %v, want abc123", out["pushed_commit"])
			}
			field("ci_result", "running")(t, out)
		},
		bad:  call{http: reportPushedCommit("3", `{"commit":" "}`)},
		code: api.Invalid,
	}
	cases["report_ci_runs"] = opCase{
		setup: pushedAccepted,
		ok:    call{http: reportCIRuns(`{"runs":[` + runJSON(1, "ci.yml", "completed", "success") + `]}`)},
		check: acknowledgement("acme", "runs"),
		full: func(t *testing.T, out map[string]any) {
			runs, _ := out["runs"].([]any)
			if len(runs) != 1 {
				t.Fatalf("runs = %v, want the one reported", out["runs"])
			}
			field("ticket", "acme-3")(t, runs[0].(map[string]any))
		},
		bad:  call{http: reportCIRuns(`{"runs":[` + runJSON(1, "ci.yml", "done", "") + `]}`)},
		code: api.Invalid,
	}
}

func reportPushedCommit(n, body string) httpCall {
	return httpCall{"POST", "/api/p/acme/tickets/" + n + "/report_pushed_commit", body}
}

func reportCIRuns(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/report_ci_runs", body}
}

// runJSON is a run of workflow on commit abc123 tracked against acme-3.
func runJSON(id int, workflow, status, conclusion string) string {
	b, _ := json.Marshal(ciRun(int64(id), workflow, "abc123", domain.CIRunStatus(status), domain.CIConclusion(conclusion)))
	return string(b)
}

// ciRun is a run of workflow on commit tracked against acme-3.
func ciRun(id int64, workflow, commit string, status domain.CIRunStatus, conclusion domain.CIConclusion) api.CIRunView {
	started := codecentraltest.Start
	return api.CIRunView{ID: id, Attempt: 1, Workflow: workflow, Name: workflow, Branch: "main", Commit: commit,
		Status: status, Conclusion: conclusion, Started: &started, URL: "https://github.com/acme/web/actions/runs/" + strconv.FormatInt(id, 10),
		Jobs: []api.CIJobView{{Name: "test", Conclusion: conclusion, URL: "https://github.com/acme/web/actions/runs/1/job/1"}}, Ticket: "acme-3"}
}

// trackedAccepted turns CI Tracking and Merge + Push on for acme, and
// makes acme-3 Accepted.
func trackedAccepted(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	ciTrackingOn(t, srv)
	pushOn(t, srv)
	acceptedTicket(t, srv)
}

// pushedAccepted is trackedAccepted with acme-3's pushed commit, abc123,
// reported.
func pushedAccepted(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	trackedAccepted(t, srv)
	pushCommit(t, srv, "abc123")
}

func pushOn(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	must(t, func() error {
		_, err := srv.Core.SetLandingMethod(api.WithActor(context.Background(), api.Human), "acme",
			api.SetLandingMethodInput{Method: domain.LandingMerge, Push: true, Remote: "origin"})
		return err
	})
}

// pushCommit reports commit as acme-3's pushed commit, as the Factory.
func pushCommit(t *testing.T, srv *codecentraltest.Server, commit string) {
	t.Helper()
	must(t, func() error {
		_, err := srv.Client.ReportPushedCommit(context.Background(), "acme", api.ReportPushedCommitInput{Ticket: "acme-3", Commit: commit})
		return err
	})
}

// reportRuns reports runs as the CI Watcher.
func reportRuns(t *testing.T, srv *codecentraltest.Server, runs ...api.CIRunView) {
	t.Helper()
	must(t, func() error {
		_, err := srv.Client.ReportCIRuns(context.Background(), "acme", api.ReportCIRunsInput{Runs: runs})
		return err
	})
}

// ticket3 is acme-3 as the Factory reads it.
func ticket3(t *testing.T, srv *codecentraltest.Server) api.TicketView {
	t.Helper()
	tk, err := srv.Client.GetTicket(context.Background(), "acme", "acme-3")
	if err != nil {
		t.Fatal(err)
	}
	return tk
}

func TestAPushedCommitIsRecordedWithWhenWhileCITrackingAndPushAreOn(t *testing.T) {
	srv := newServer(t, trackedAccepted)
	srv.Clock.Advance(time.Minute)
	pushCommit(t, srv, "abc123")
	tk := ticket3(t, srv)
	if tk.PushedCommit == nil || tk.PushedCommit.Commit != "abc123" || !tk.PushedCommit.Pushed.Equal(codecentraltest.Start.Add(time.Minute)) {
		t.Fatalf("pushed commit = %+v, want abc123 pushed a minute in", tk.PushedCommit)
	}
	if tk.CIResult != api.CIRunning || tk.CIRuns != nil {
		t.Fatalf("ci result %q with runs %+v, want running with none reported", tk.CIResult, tk.CIRuns)
	}
}

func TestAPushedCommitIsNotTrackedWithCITrackingOrPushOff(t *testing.T) {
	for name, setup := range map[string]func(*testing.T, *codecentraltest.Server){
		"ci tracking off": func(t *testing.T, srv *codecentraltest.Server) { pushOn(t, srv); acceptedTicket(t, srv) },
		"push off":        func(t *testing.T, srv *codecentraltest.Server) { ciTrackingOn(t, srv); acceptedTicket(t, srv) },
	} {
		t.Run(name, func(t *testing.T) {
			srv := newServer(t, setup)
			out := httpOK(t, srv, reportPushedCommit("3", `{"commit":"abc123"}`))
			acknowledgement("acme-3")(t, out)
			if tk := ticket3(t, srv); tk.PushedCommit != nil || tk.CIResult != "" {
				t.Fatalf("pushed commit %+v, ci result %q: want nothing tracked", tk.PushedCommit, tk.CIResult)
			}
		})
	}
}

func TestOnlyAnAcceptedOrInReviewTicketHasAPushedCommit(t *testing.T) {
	srv := newServer(t, ciTrackingOn, pushOn)
	httpRejected(t, srv, reportPushedCommit("3", `{"commit":"abc123"}`), api.ActionNotAllowed)

	claimedTicket(t, srv)
	must(t, func() error {
		_, err := srv.Core.Finish(context.Background(), "acme", api.FinishInput{Ticket: "3", ClaimID: "claim-1", Summary: "Done.", Branch: "acme-3", Commit: "abc123"})
		return err
	})
	pushCommit(t, srv, "abc123")
	if tk := ticket3(t, srv); tk.State != domain.InReview || tk.PushedCommit == nil {
		t.Fatalf("%s with pushed commit %+v, want In Review's hand merge tracked", tk.State, tk.PushedCommit)
	}
}

func TestTheCIResultOfEachMixOfRuns(t *testing.T) {
	run := func(id int64, workflow string, status domain.CIRunStatus, conclusion domain.CIConclusion) api.CIRunView {
		return ciRun(id, workflow, "abc123", status, conclusion)
	}
	for _, tc := range []struct {
		name string
		runs []api.CIRunView
		want api.CIResult
	}{
		{"none reported", nil, api.CIRunning},
		{"one queued", []api.CIRunView{run(1, "ci.yml", domain.CIQueued, ""), run(2, "lint.yml", domain.CICompleted, domain.CISuccess)}, api.CIRunning},
		{"one in progress beside a failure", []api.CIRunView{run(1, "ci.yml", domain.CIInProgress, ""), run(2, "lint.yml", domain.CICompleted, domain.CIFailure)}, api.CIRunning},
		{"all succeeded", []api.CIRunView{run(1, "ci.yml", domain.CICompleted, domain.CISuccess), run(2, "lint.yml", domain.CICompleted, domain.CISuccess)}, api.CIPassed},
		{"skipped and neutral pass", []api.CIRunView{run(1, "ci.yml", domain.CICompleted, domain.CISkipped), run(2, "lint.yml", domain.CICompleted, domain.CINeutral)}, api.CIPassed},
		{"a failure", []api.CIRunView{run(1, "ci.yml", domain.CICompleted, domain.CISuccess), run(2, "lint.yml", domain.CICompleted, domain.CIFailure)}, api.CIFailed},
		{"a lone cancelled run", []api.CIRunView{run(1, "ci.yml", domain.CICompleted, domain.CICancelled)}, api.CICancelled},
		{"a cancelled run beside a pass", []api.CIRunView{run(1, "ci.yml", domain.CICompleted, domain.CISuccess), run(2, "lint.yml", domain.CICompleted, domain.CICancelled)}, api.CICancelled},
		{"a failure beside a cancelled run", []api.CIRunView{run(1, "ci.yml", domain.CICompleted, domain.CICancelled), run(2, "lint.yml", domain.CICompleted, domain.CIFailure)}, api.CIFailed},
		{"a timed out run beside a cancelled run", []api.CIRunView{run(1, "ci.yml", domain.CICompleted, domain.CICancelled), run(2, "lint.yml", domain.CICompleted, domain.CITimedOut)}, api.CIFailed},
		{"one in progress beside a cancelled run", []api.CIRunView{run(1, "ci.yml", domain.CIInProgress, ""), run(2, "lint.yml", domain.CICompleted, domain.CICancelled)}, api.CIRunning},
		{"an ignored cancelled run", []api.CIRunView{run(1, "ci.yml", domain.CICompleted, domain.CISuccess), run(2, "nightly.yml", domain.CICompleted, domain.CICancelled)}, api.CIPassed},
		{"a timed out run", []api.CIRunView{run(1, "ci.yml", domain.CICompleted, domain.CITimedOut)}, api.CIFailed},
		{"an ignored failure", []api.CIRunView{run(1, "ci.yml", domain.CICompleted, domain.CISuccess), run(2, "nightly.yml", domain.CICompleted, domain.CIFailure)}, api.CIPassed},
		{"only an ignored run", []api.CIRunView{run(2, "nightly.yml", domain.CICompleted, domain.CISuccess)}, api.CIRunning},
		{"an ignored run unfinished", []api.CIRunView{run(1, "ci.yml", domain.CICompleted, domain.CISuccess), run(2, "nightly.yml", domain.CIInProgress, "")}, api.CIPassed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := newServer(t, pushedAccepted)
			if len(tc.runs) > 0 {
				reportRuns(t, srv, tc.runs...)
			}
			if got := ticket3(t, srv).CIResult; got != tc.want {
				t.Fatalf("ci result %q, want %q", got, tc.want)
			}
		})
	}
}

func TestACancelledRunReRunToSuccessIsPassed(t *testing.T) {
	srv := newServer(t, pushedAccepted)
	first := ciRun(1, "ci.yml", "abc123", domain.CICompleted, domain.CICancelled)
	reportRuns(t, srv, first)
	if got := ticket3(t, srv).CIResult; got != api.CICancelled {
		t.Fatalf("ci result %q, want cancelled", got)
	}
	again := ciRun(1, "ci.yml", "abc123", domain.CICompleted, domain.CISuccess)
	again.Attempt = 2
	reportRuns(t, srv, again)
	if got := ticket3(t, srv).CIResult; got != api.CIPassed {
		t.Fatalf("ci result %q, want passed once the re-run succeeded", got)
	}
}

func TestReportCIRunsUpsertsByRunIDAndAttempt(t *testing.T) {
	srv := newServer(t, pushedAccepted)
	reportRuns(t, srv, ciRun(1, "ci.yml", "abc123", domain.CIInProgress, ""))
	finished := ciRun(1, "ci.yml", "abc123", domain.CICompleted, domain.CIFailure)
	at := codecentraltest.Start.Add(5 * time.Minute)
	finished.Finished = &at
	reportRuns(t, srv, finished)
	tk := ticket3(t, srv)
	if len(tk.CIRuns) != 1 || tk.CIRuns[0].Status != domain.CICompleted || tk.CIRuns[0].Finished == nil || !tk.CIRuns[0].Finished.Equal(at) {
		t.Fatalf("runs = %+v, want the one run, now finished", tk.CIRuns)
	}
	if tk.CIResult != api.CIFailed {
		t.Fatalf("ci result %q, want failed", tk.CIResult)
	}

	// A re-run is the next attempt of the same run: it replaces the
	// failed attempt in the CI result, and both are kept.
	rerun := ciRun(1, "ci.yml", "abc123", domain.CICompleted, domain.CISuccess)
	rerun.Attempt = 2
	reportRuns(t, srv, rerun)
	tk = ticket3(t, srv)
	if len(tk.CIRuns) != 2 || tk.CIResult != api.CIPassed {
		t.Fatalf("runs %+v with ci result %q, want both attempts and passed", tk.CIRuns, tk.CIResult)
	}
}

func TestAReportedRunCarriesEverythingTheCIWatcherSaid(t *testing.T) {
	srv := newServer(t, pushedAccepted)
	want := ciRun(7, "nightly.yml", "abc123", domain.CICompleted, domain.CIFailure)
	want.Name, want.Branch = "Nightly build", "acme-3"
	at := codecentraltest.Start.Add(3 * time.Minute)
	want.Finished = &at
	want.Jobs = []api.CIJobView{{Name: "build", Conclusion: domain.CISuccess, URL: "https://github.com/acme/web/actions/runs/7/job/1"},
		{Name: "test", Conclusion: domain.CIFailure, URL: "https://github.com/acme/web/actions/runs/7/job/2"}}
	reportRuns(t, srv, want)
	want.Ignored = true
	got := ticket3(t, srv).CIRuns
	a, _ := json.Marshal(got)
	b, _ := json.Marshal([]api.CIRunView{want})
	if string(a) != string(b) {
		t.Fatalf("runs = %s\nwant %s", a, b)
	}
}

func TestANewerPushedCommitReplacesTheOldAndKeepsItsRunsAsHistory(t *testing.T) {
	srv := newServer(t, pushedAccepted)
	reportRuns(t, srv, ciRun(1, "ci.yml", "abc123", domain.CICompleted, domain.CIFailure))
	if got := ticket3(t, srv).CIResult; got != api.CIFailed {
		t.Fatalf("ci result %q, want failed", got)
	}
	pushCommit(t, srv, "def456")
	tk := ticket3(t, srv)
	if tk.PushedCommit.Commit != "def456" || tk.CIResult != api.CIRunning {
		t.Fatalf("pushed %+v with ci result %q, want def456 running", tk.PushedCommit, tk.CIResult)
	}
	reportRuns(t, srv, ciRun(2, "ci.yml", "def456", domain.CICompleted, domain.CISuccess))
	tk = ticket3(t, srv)
	if tk.CIResult != api.CIPassed || len(tk.CIRuns) != 2 || tk.CIRuns[0].Commit != "def456" || tk.CIRuns[1].Commit != "abc123" {
		t.Fatalf("ci result %q with runs %+v, want passed, def456's run first and abc123's kept", tk.CIResult, tk.CIRuns)
	}
}

func TestReportCIRunsRefusesWhatItCantTrack(t *testing.T) {
	srv := newServer(t, pushedAccepted)
	noTarget := ciRun(1, "ci.yml", "abc123", domain.CIQueued, "")
	noTarget.Ticket = ""
	unfinishedConcluded := ciRun(1, "ci.yml", "abc123", domain.CIInProgress, domain.CISuccess)
	path := ciRun(1, ".github/workflows/ci.yml", "abc123", domain.CIQueued, "")
	for name, run := range map[string]api.CIRunView{"nothing tracked": noTarget, "a conclusion while running": unfinishedConcluded, "a workflow path": path} {
		b, _ := json.Marshal(api.ReportCIRunsInput{Runs: []api.CIRunView{run}})
		if e := httpRejected(t, srv, reportCIRuns(string(b)), api.Invalid); e.Field == "" {
			t.Errorf("%s: refused with no field: %+v", name, e)
		}
	}
	missing := ciRun(1, "ci.yml", "abc123", domain.CIQueued, "")
	missing.Ticket = "acme-99"
	b, _ := json.Marshal(api.ReportCIRunsInput{Runs: []api.CIRunView{missing}})
	httpRejected(t, srv, reportCIRuns(string(b)), api.Invalid)
	if runs := ticket3(t, srv).CIRuns; runs != nil {
		t.Fatalf("runs = %+v, want nothing stored from a refused report", runs)
	}
}

func TestReportCIRunsNeedsCITrackingOn(t *testing.T) {
	srv := newServer(t, pushedAccepted)
	httpOK(t, srv, setCITracking(`{"tracking":false}`))
	b, _ := json.Marshal(api.ReportCIRunsInput{Runs: []api.CIRunView{ciRun(1, "ci.yml", "abc123", domain.CIQueued, "")}})
	httpRejected(t, srv, reportCIRuns(string(b)), api.ActionNotAllowed)
}

func TestReportedRunsSendTheirTicketAChangeNotice(t *testing.T) {
	srv := newServer(t, pushedAccepted)
	sub, stop := srv.Core.Events().Subscribe()
	defer stop()
	reportRuns(t, srv, ciRun(1, "ci.yml", "abc123", domain.CIQueued, ""))
	awaitNotice(t, sub, "ticket.changed on acme-3", func(e events.Event) bool { return e.Type == events.TicketChanged && e.ID == "acme-3" })
}

func TestGetCIWatchHasThePushedCommitsStillWaiting(t *testing.T) {
	srv := newServer(t, pushedAccepted)
	waiting := func() []api.CIWaitingCommit {
		t.Helper()
		w, err := srv.Client.GetCIWatch(context.Background(), "acme")
		if err != nil {
			t.Fatal(err)
		}
		return w.Waiting
	}
	if got := waiting(); len(got) != 1 || got[0].Ticket != "acme-3" || got[0].Commit != "abc123" || !got[0].Pushed.Equal(codecentraltest.Start) {
		t.Fatalf("waiting = %+v, want acme-3's abc123, newly pushed", got)
	}
	// Its runs all finished, but it is newly pushed: another workflow may
	// yet start.
	reportRuns(t, srv, ciRun(1, "ci.yml", "abc123", domain.CICompleted, domain.CISuccess))
	if got := waiting(); len(got) != 1 {
		t.Fatalf("waiting = %+v, want abc123 still watched while newly pushed", got)
	}
	srv.Clock.Advance(10 * time.Minute)
	if got := waiting(); len(got) != 0 {
		t.Fatalf("waiting = %+v, want nothing once passed and no longer new", got)
	}
	reportRuns(t, srv, ciRun(2, "lint.yml", "abc123", domain.CIInProgress, ""))
	if got := waiting(); len(got) != 1 {
		t.Fatalf("waiting = %+v, want abc123 watched again while a run is unfinished", got)
	}
	// A run that never finishes, or never starts, isn't watched for good.
	srv.Clock.Advance(24 * time.Hour)
	if got := waiting(); len(got) != 0 {
		t.Fatalf("waiting = %+v, want nothing a day after the push", got)
	}
}
