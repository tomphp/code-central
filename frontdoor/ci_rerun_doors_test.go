package frontdoor_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/httpapi"
	"github.com/tomoram/code-central/store"
)

// Re-run failed jobs (code-central-265): the human's rerun_ci records a
// Re-run request on a Ticket whose CI failed, which puts it back to
// waiting; the CI Watcher reads it from get_ci_watch, and it clears once
// the CI Watcher reports the run's new attempt.
func init() {
	cases["rerun_ci"] = opCase{
		setup: failedCI,
		ok:    call{http: rerunCI("3")},
		check: ticketAcknowledgement("acme-3", "Accepted", "ci_result", "ci_item", "ci_failure_options", "needs_me_reason"),
		full:  field("ci_result", "running"),
		bad:   call{http: rerunCI("2")},
		code:  core.ActionNotAllowed,
	}
}

func rerunCI(n string) httpCall {
	return httpCall{"POST", "/api/p/acme/tickets/" + n + "/rerun_ci", `{}`}
}

// ciWatch is acme's get_ci_watch, as the CI Watcher reads it.
func ciWatch(t *testing.T, srv *codecentraltest.Server) core.CIWatch {
	t.Helper()
	w, err := srv.Client.GetCIWatch(context.Background(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestRerunCIPutsACIFailedTicketBackToWaiting(t *testing.T) {
	srv := newServer(t, failedCI)
	httpOK(t, srv, rerunCI("3"))
	if row, ok := needsMeRowOf(t, srv, "acme-3"); ok {
		t.Fatalf("row %+v, want acme-3 out of Needs Me while its jobs re-run", row)
	}
	tk := ticket3(t, srv)
	if tk.CIResult != core.CIRunning || tk.CIFailureOptions != nil || tk.LandAnyway {
		t.Fatalf("ci result %q, options %v, land anyway %v: want running with no options", tk.CIResult, tk.CIFailureOptions, tk.LandAnyway)
	}
	w := ciWatch(t, srv)
	if !slices.Equal(w.Reruns, []core.CIRerun{{Ticket: "acme-3", Commit: "abc123", ID: 1, Attempt: 1, Requested: codecentraltest.Start}}) {
		t.Fatalf("reruns = %+v, want ci.yml's failed run 1, attempt 1", w.Reruns)
	}
	if len(w.Waiting) != 1 || w.Waiting[0].Ticket != "acme-3" {
		t.Fatalf("waiting = %+v, want acme-3's pushed commit watched again", w.Waiting)
	}
	// Asking again while it waits changes nothing.
	out := httpOK(t, srv, rerunCI("3"))
	if changed, _ := out["changed"].([]any); len(changed) != 0 {
		t.Fatalf("changed %v, want nothing on a second Re-run", changed)
	}
}

func TestReportingTheRunsNewAttemptClearsTheRerunRequest(t *testing.T) {
	srv := newServer(t, failedCI)
	httpOK(t, srv, rerunCI("3"))
	again := ciRun(1, "ci.yml", "abc123", store.CIQueued, "")
	again.Attempt = 2
	reportRuns(t, srv, again)
	if w := ciWatch(t, srv); len(w.Reruns) != 0 {
		t.Fatalf("reruns = %+v, want none once attempt 2 is reported", w.Reruns)
	}
	if tk := ticket3(t, srv); tk.CIResult != core.CIRunning {
		t.Fatalf("ci result %q, want running while attempt 2 runs", tk.CIResult)
	}
	again.Status, again.Conclusion = store.CICompleted, store.CISuccess
	reportRuns(t, srv, again)
	if tk := ticket3(t, srv); tk.CIResult != core.CIPassed {
		t.Fatalf("ci result %q, want passed once attempt 2 passed", tk.CIResult)
	}
}

// Reporting the failed attempt again, as a restarted CI Watcher does,
// leaves the request waiting on the new one.
func TestReportingTheFailedAttemptAgainKeepsTheRerunRequest(t *testing.T) {
	srv := newServer(t, failedCI)
	httpOK(t, srv, rerunCI("3"))
	reportRuns(t, srv, ciRun(1, "ci.yml", "abc123", store.CICompleted, store.CIFailure))
	if w := ciWatch(t, srv); len(w.Reruns) != 1 {
		t.Fatalf("reruns = %+v, want run 1's still waiting", w.Reruns)
	}
	if tk := ticket3(t, srv); tk.CIResult != core.CIRunning {
		t.Fatalf("ci result %q, want running while the re-run waits", tk.CIResult)
	}
}

// A request GitHub never answers with a new attempt, say for a run too
// old to re-run, lapses after 10 minutes: the Ticket is CI failed again,
// and the human can ask again or choose another option.
func TestAnUnansweredRerunRequestLapsesBackToCIFailed(t *testing.T) {
	srv := newServer(t, failedCI)
	httpOK(t, srv, rerunCI("3"))
	srv.Clock.Advance(9 * time.Minute)
	if tk := ticket3(t, srv); tk.CIResult != core.CIRunning {
		t.Fatalf("ci result %q 9 minutes on, want running", tk.CIResult)
	}
	srv.Clock.Advance(time.Minute)
	if w := ciWatch(t, srv); len(w.Reruns) != 0 {
		t.Fatalf("reruns = %+v, want the request lapsed", w.Reruns)
	}
	tk := ticket3(t, srv)
	if tk.CIResult != core.CIFailed || tk.NeedsMeReason != core.ReasonCIFailed || !slices.Contains(tk.CIFailureOptions, "rerun_ci") {
		t.Fatalf("ci result %q, reason %q, options %v: want CI failed again, offering Re-run", tk.CIResult, tk.NeedsMeReason, tk.CIFailureOptions)
	}
	httpOK(t, srv, rerunCI("3"))
	if w := ciWatch(t, srv); len(w.Reruns) != 1 || !w.Reruns[0].Requested.Equal(srv.Clock.Now()) {
		t.Fatalf("reruns = %+v, want a new request", w.Reruns)
	}
}

// Every failed run of the pushed commit is re-run, each at its latest
// attempt; runs that passed or are ignored aren't.
func TestRerunCIAsksForEachFailedRun(t *testing.T) {
	srv := newServer(t, pushedAccepted)
	second := ciRun(5, "e2e.yml", "abc123", store.CICompleted, store.CIFailure)
	second.Attempt = 2
	reportRuns(t, srv, ciRun(1, "ci.yml", "abc123", store.CICompleted, store.CICancelled), second,
		ciRun(2, "lint.yml", "abc123", store.CICompleted, store.CISuccess),
		ciRun(3, "nightly.yml", "abc123", store.CICompleted, store.CIFailure))
	httpOK(t, srv, rerunCI("3"))
	at := codecentraltest.Start
	want := []core.CIRerun{{Ticket: "acme-3", Commit: "abc123", ID: 1, Attempt: 1, Requested: at}, {Ticket: "acme-3", Commit: "abc123", ID: 5, Attempt: 2, Requested: at}}
	if w := ciWatch(t, srv); !slices.Equal(w.Reruns, want) {
		t.Fatalf("reruns = %+v, want %+v", w.Reruns, want)
	}
}

// A Re-run request is offered under both Push Landing Methods.
func TestRerunCIIsOfferedUnderMergeAndBranch(t *testing.T) {
	srv := newServer(t, failedCI)
	if tk := ticket3(t, srv); !slices.Equal(tk.CIFailureOptions, []string{"rerun_ci", "land_anyway", "send_back", "new_fix_ticket"}) {
		t.Fatalf("options %v under Merge + Push, want Re-run failed jobs, Land anyway and the fixes", tk.CIFailureOptions)
	}
	httpOK(t, srv, setLandingMethod(`{"method":"branch","push":true,"remote":"origin"}`))
	if tk := ticket3(t, srv); !slices.Equal(tk.CIFailureOptions, []string{"rerun_ci", "send_back", "new_fix_ticket"}) {
		t.Fatalf("options %v under Branch + Push, want Re-run failed jobs and the fixes", tk.CIFailureOptions)
	}
	httpOK(t, srv, rerunCI("3"))
	if tk := ticket3(t, srv); tk.CIResult != core.CIRunning {
		t.Fatalf("ci result %q, want running", tk.CIResult)
	}
}

func TestRerunCINeedsAFailedCIResult(t *testing.T) {
	for name, setup := range map[string]func(*testing.T, *codecentraltest.Server){
		"never pushed": trackedAccepted,
		"running":      pushedAccepted,
		"passed": func(t *testing.T, srv *codecentraltest.Server) {
			pushedAccepted(t, srv)
			reportRuns(t, srv, ciRun(1, "ci.yml", "abc123", store.CICompleted, store.CISuccess))
		},
		"not Accepted": func(t *testing.T, srv *codecentraltest.Server) {
			ciTrackingOn(t, srv)
			pushOn(t, srv)
		},
	} {
		t.Run(name, func(t *testing.T) {
			srv := newServer(t, setup)
			httpRejected(t, srv, rerunCI("3"), core.ActionNotAllowed)
			if w := ciWatch(t, srv); len(w.Reruns) != 0 {
				t.Fatalf("reruns = %+v, want none on a refused call", w.Reruns)
			}
		})
	}
}

func TestRerunCIIsRejectedForAnAgent(t *testing.T) {
	srv := newServer(t, failedCI)
	status, b := httpDoAs(t, srv, core.Agent, rerunCI("3"))
	if e := errorJSON(t, b, core.ActionNotAllowed); status != httpapi.Status(core.ActionNotAllowed) {
		t.Fatalf("status %d, want %d for %s", status, httpapi.Status(core.ActionNotAllowed), e.Code)
	}
	if tk := ticket3(t, srv); tk.CIResult != core.CIFailed {
		t.Fatalf("ci result %q, want an agent's Re-run refused", tk.CIResult)
	}
}
