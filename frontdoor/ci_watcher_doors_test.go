package frontdoor_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/events"
)

// The CI Watcher's own operations (code-central-253): get_ci_watch, the
// read it polls, and report_ci_watcher_status. It writes as an ordinary
// agent client, over HTTP only.
func init() {
	cases["get_ci_watch"] = opCase{
		setup: ciTrackingOn,
		ok:    call{http: getCIWatch("acme")},
		check: func(t *testing.T, out map[string]any) {
			field("project", "acme")(t, out)
			field("repository", "acme/web")(t, out)
		},
		bad:  call{http: getCIWatch("nope")},
		code: core.UnknownProject,
	}
	cases["report_ci_watcher_status"] = opCase{
		setup: ciTrackingOn,
		ok:    call{http: reportCIWatcherStatus(`{"state":"watching"}`)},
		check: func(t *testing.T, out map[string]any) {
			acknowledgement("acme", "ci")(t, out)
		},
		full: func(t *testing.T, out map[string]any) {
			ciWatcher(t, out, "watching", "", "")
		},
		bad:  call{http: reportCIWatcherStatus(`{"state":"asleep"}`)},
		code: core.Invalid,
	}
}

func getCIWatch(key string) httpCall { return httpCall{"GET", "/api/p/" + key + "/ci-watch", ""} }

func reportCIWatcherStatus(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/report_ci_watcher_status", body}
}

// ciTrackingOn turns CI Tracking on for acme, watching acme/web.
func ciTrackingOn(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	httpOK(t, srv, setCITracking(`{"tracking":true,"repository":"acme/web","ignored_workflows":["nightly.yml"]}`))
}

// ciWatcher checks the CI Watcher's status in a Project's ci object.
func ciWatcher(t *testing.T, project map[string]any, state, message, fix string) {
	t.Helper()
	c, _ := project["ci"].(map[string]any)
	w, _ := c["watcher"].(map[string]any)
	if w == nil || w["state"] != state || str(w["message"]) != message || str(w["fix"]) != fix {
		t.Fatalf("ci.watcher = %v, want %s with message %q and fix %q", c["watcher"], state, message, fix)
	}
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func TestGetCIWatchHasTheCISettingsRulesAndGHPath(t *testing.T) {
	srv := newServer(t, ciTrackingOn)
	httpOK(t, srv, addCIRule(`{"workflow":"ci.yml","branch":"main","state":"Ready"}`))
	httpOK(t, srv, httpCall{"POST", "/api/set_options", `{"gh_path":` + strconv.Quote("/opt/tools/gh") + `}`})

	status, b := httpDoAs(t, srv, core.Agent, getCIWatch("acme"))
	if status != 200 {
		t.Fatalf("status %d: %s", status, b)
	}
	w, err := srv.Client.GetCIWatch(context.Background(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	if w.Project != "acme" || !w.Tracking || w.Provider != "github_actions" || w.Repository != "acme/web" || w.GHPath != "/opt/tools/gh" ||
		len(w.IgnoredWorkflows) != 1 || w.IgnoredWorkflows[0] != "nightly.yml" {
		t.Fatalf("ci watch = %+v, want acme/web on with nightly.yml ignored and the gh path Option", w)
	}
	if len(w.Rules) != 1 || w.Rules[0].Spec != "acme-4" || w.Rules[0].Workflow != "ci.yml" || w.Rules[0].Branch != "main" ||
		!w.Rules[0].Created.Equal(codecentraltest.Start) {
		t.Fatalf("rules = %+v, want ci.yml on main with when it was created", w.Rules)
	}
}

func TestGetCIWatchWithCITrackingOffSaysSo(t *testing.T) {
	srv := newServer(t)
	w, err := srv.Client.GetCIWatch(context.Background(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	if w.Tracking || w.Rules == nil || w.IgnoredWorkflows == nil {
		t.Fatalf("ci watch = %+v, want tracking off with empty lists", w)
	}
}

func TestEveryProjectReadCarriesTheCIWatchersStatus(t *testing.T) {
	srv := newServer(t, ciTrackingOn)
	ciWatcher(t, httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""}), "not_reported", "", "")

	status, b := httpDoAs(t, srv, core.Agent, reportCIWatcherStatus(`{"state":"error","problem":"logged_out","message":"gh isn't logged in","fix":"Run gh auth login."}`))
	if status != 200 {
		t.Fatalf("status %d: %s", status, b)
	}
	ciWatcher(t, httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""}), "error", "gh isn't logged in", "Run gh auth login.")
	rows := httpOK(t, srv, httpCall{"GET", "/api/projects", ""})["rows"].([]any)
	ciWatcher(t, rows[0].(map[string]any), "error", "gh isn't logged in", "Run gh auth login.")
	ciWatcher(t, rows[1].(map[string]any), "not_reported", "", "")

	srv.Clock.Advance(2 * time.Minute)
	out := httpOK(t, srv, reportCIWatcherStatus(`{"state":"watching","full":true}`))
	ciWatcher(t, out, "watching", "", "")
	since := out["ci"].(map[string]any)["watcher"].(map[string]any)["since"]
	if since != codecentraltest.Start.Add(2*time.Minute).Format(time.RFC3339) {
		t.Fatalf("since = %v, want when it went back to watching", since)
	}
}

func TestAnErrorNeedsItsMessage(t *testing.T) {
	srv := newServer(t, ciTrackingOn)
	e := httpRejected(t, srv, reportCIWatcherStatus(`{"state":"error"}`), core.Invalid)
	if e.Field != "message" {
		t.Fatalf("field %q, want message", e.Field)
	}
}

func TestTheCIWatchersStatusIsRefusedWhileCITrackingIsOff(t *testing.T) {
	srv := newServer(t)
	httpRejected(t, srv, reportCIWatcherStatus(`{"state":"watching"}`), core.ActionNotAllowed)
	httpRejected(t, srv, httpCall{"POST", "/api/p/nope/report_ci_watcher_status", `{"state":"watching"}`}, core.UnknownProject)
}

func TestTurningCITrackingOffForgetsTheCIWatchersStatus(t *testing.T) {
	srv := newServer(t, ciTrackingOn)
	httpOK(t, srv, reportCIWatcherStatus(`{"state":"watching"}`))
	ciWatcher(t, httpOK(t, srv, setCITracking(`{"tracking":false,"repository":"acme/web","full":true}`)), "not_reported", "", "")
}

func TestChangingTheRepositoryForgetsTheCIWatchersStatus(t *testing.T) {
	srv := newServer(t, ciTrackingOn)
	httpOK(t, srv, reportCIWatcherStatus(`{"state":"watching"}`))
	ciWatcher(t, httpOK(t, srv, setCITracking(`{"tracking":true,"repository":"acme/web","ignored_workflows":["deploy.yml"],"full":true}`)), "watching", "", "")
	ciWatcher(t, httpOK(t, srv, setCITracking(`{"tracking":true,"repository":"acme/site","full":true}`)), "not_reported", "", "")
}

func TestAnErrorsProblemIsACIProblem(t *testing.T) {
	srv := newServer(t, ciTrackingOn)
	e := httpRejected(t, srv, reportCIWatcherStatus(`{"state":"error","problem":"sad","message":"no"}`), core.Invalid)
	if e.Field != "problem" {
		t.Fatalf("field %q, want problem", e.Field)
	}
}

// The status lives in memory, not the store, so the notice is the
// CI Watcher's own: one per change, none for the same status again.
func TestTheCIWatchersStatusPublishesOnlyWhenItChanges(t *testing.T) {
	srv := newServer(t, ciTrackingOn)
	notices, stop := srv.Core.Events().Subscribe()
	defer stop()
	httpOK(t, srv, reportCIWatcherStatus(`{"state":"watching"}`))
	select {
	case got := <-notices:
		if want := (events.Event{Type: events.ProjectChanged, Project: "acme"}); got != want {
			t.Fatalf("notice %+v, want %+v", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a new status sent no notice")
	}
	srv.Clock.Advance(2 * time.Minute)
	if out := httpOK(t, srv, reportCIWatcherStatus(`{"state":"watching"}`)); len(out["changed"].([]any)) != 0 {
		t.Fatalf("the same status changed %v", out["changed"])
	}
	select {
	case got := <-notices:
		t.Fatalf("the same status sent %+v", got)
	case <-time.After(100 * time.Millisecond):
	}
}

// Only the service reports a crash: the CI Watcher can't name it.
func TestTheCIWatcherCantReportItsOwnCrash(t *testing.T) {
	srv := newServer(t, ciTrackingOn)
	e := httpRejected(t, srv, reportCIWatcherStatus(`{"state":"error","problem":"crashed","message":"crashed"}`), core.Invalid)
	if e.Field != "problem" {
		t.Fatalf("field %q, want problem", e.Field)
	}
}

// A crash streak stays shown, with one Notification, even when the
// restarted CI Watcher reports Watching before crashing again, until it
// has run CIWatcherSteadyAfter from its restart.
func TestACrashStaysShownUntilTheCIWatcherHasRunSteadily(t *testing.T) {
	srv := newServer(t, ciTrackingOn)
	ctx := context.Background()
	crash := func(restarts int) {
		srv.Core.ReportCIWatcherCrash(ctx, "acme", core.CIWatcherCrash{Restarts: restarts, LastLine: "panic: boom",
			RestartAt: srv.Clock.Now().Add(time.Second)})
	}
	crashed := "The CI Watcher crashed, restarted 2 times; the last line it wrote: panic: boom"
	fix := "Code Central restarts it by itself, waiting longer after each crash. Its output is in the service's log."

	crash(1)
	srv.Clock.Advance(time.Second)
	httpOK(t, srv, reportCIWatcherStatus(`{"state":"watching"}`))
	crash(2)
	srv.Clock.Advance(time.Second)
	httpOK(t, srv, reportCIWatcherStatus(`{"state":"watching"}`))
	ciWatcher(t, httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""}), "error", crashed, fix)
	if got := srv.Core.CIWatcherAttention(ctx); len(got) != 1 || got[0].Errors != 1 {
		t.Fatalf("CI Watcher attention %+v, want one error, for one Notification", got)
	}

	srv.Clock.Advance(core.CIWatcherSteadyAfter - time.Millisecond)
	httpOK(t, srv, reportCIWatcherStatus(`{"state":"watching"}`))
	ciWatcher(t, httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""}), "error", crashed, fix)
	srv.Clock.Advance(time.Millisecond)
	httpOK(t, srv, reportCIWatcherStatus(`{"state":"watching"}`))
	ciWatcher(t, httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""}), "watching", "", "")

	// A crash after that is a new streak, with its own Notification.
	crash(1)
	if got := srv.Core.CIWatcherAttention(ctx); got[0].Errors != 2 {
		t.Fatalf("CI Watcher attention %+v, want a second error", got)
	}
}

// Turning CI Tracking off ends a crash streak, and stops the service
// reporting one.
func TestTurningCITrackingOffEndsACrashStreak(t *testing.T) {
	srv := newServer(t, ciTrackingOn)
	ctx := context.Background()
	srv.Core.ReportCIWatcherCrash(ctx, "acme", core.CIWatcherCrash{Restarts: 1, RestartAt: srv.Clock.Now()})
	httpOK(t, srv, setCITracking(`{"tracking":false,"repository":"acme/web"}`))
	srv.Core.ReportCIWatcherCrash(ctx, "acme", core.CIWatcherCrash{Restarts: 2, RestartAt: srv.Clock.Now()})
	ciWatcher(t, httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""}), "not_reported", "", "")
	ciTrackingOn(t, srv)
	httpOK(t, srv, reportCIWatcherStatus(`{"state":"watching"}`))
	ciWatcher(t, httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""}), "watching", "", "")
}
