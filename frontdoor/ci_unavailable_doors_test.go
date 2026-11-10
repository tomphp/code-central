package frontdoor_test

import (
	"context"
	"testing"
	"time"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// CI unavailable (code-central-262): a Ticket the CI gate has held for
// 10 minutes or more since its push shows in Needs Me while the CI
// Watcher isn't Watching or no CI Runs have been reported for its pushed
// commit, worked out on read through the test clock.

// wantCIUnavailable checks acme-3 is CI unavailable for cause since its
// push at codecentraltest.Start.
func wantCIUnavailable(t *testing.T, srv *codecentraltest.Server, cause core.CIUnavailableCause) core.NeedsMeRow {
	t.Helper()
	row, ok := needsMeRowOf(t, srv, "acme-3")
	if !ok || row.Reason != core.ReasonCIUnavailable {
		t.Fatalf("acme-3 in Needs Me %v as %+v, want ci_unavailable", ok, row)
	}
	if row.CIUnavailable == nil || row.CIUnavailable.Cause != cause || row.CIUnavailable.Commit != "abc123" {
		t.Fatalf("ci_unavailable = %+v, want %s at abc123", row.CIUnavailable, cause)
	}
	if !row.Since.Equal(codecentraltest.Start) {
		t.Fatalf("since %v, want the push at %v", row.Since, codecentraltest.Start)
	}
	return row
}

// wantNotInNeedsMe checks acme-3 isn't in Needs Me at all.
func wantNotInNeedsMe(t *testing.T, srv *codecentraltest.Server, when string) {
	t.Helper()
	if row, ok := needsMeRowOf(t, srv, "acme-3"); ok {
		t.Fatalf("%s, acme-3 is in Needs Me as %+v, want it not there", when, row)
	}
}

func watcherReports(t *testing.T, srv *codecentraltest.Server, in core.ReportCIWatcherStatusInput) {
	t.Helper()
	must(t, func() error {
		_, err := srv.Client.ReportCIWatcherStatus(context.Background(), "acme", in)
		return err
	})
}

var (
	watching   = core.ReportCIWatcherStatusInput{State: core.CIWatcherWatching}
	loggedOut  = core.ReportCIWatcherStatusInput{State: core.CIWatcherError, Problem: core.CILoggedOut, Message: "gh is logged out", Fix: "Run gh auth login."}
	runningRun = ciRun(1, "ci.yml", "abc123", store.CIInProgress, "")
)

func TestAGatedTicketWithNoCIRunsIsCIUnavailableAfterTenMinutes(t *testing.T) {
	srv := newServer(t, pushedAccepted)
	watcherReports(t, srv, watching)

	srv.Clock.Advance(core.CIUnavailableAfter - time.Millisecond)
	wantNotInNeedsMe(t, srv, "just under 10 minutes after its push")

	srv.Clock.Advance(time.Millisecond)
	row := wantCIUnavailable(t, srv, core.CINoRuns)
	if row.CIUnavailable.Message != "" {
		t.Fatalf("with the CI Watcher Watching, message %q, want none", row.CIUnavailable.Message)
	}

	reportRuns(t, srv, runningRun)
	wantNotInNeedsMe(t, srv, "with a run going and the CI Watcher Watching")
	// Waiting on CI that is running isn't Accepted but not landed either.
	srv.Clock.Advance(time.Hour)
	wantNotInNeedsMe(t, srv, "an hour on with the run still going")
}

func TestAGatedTicketIsCIUnavailableWhileTheCIWatcherIsInError(t *testing.T) {
	srv := newServer(t, pushedAccepted)
	reportRuns(t, srv, runningRun)
	watcherReports(t, srv, loggedOut)

	srv.Clock.Advance(core.CIUnavailableAfter - time.Millisecond)
	wantNotInNeedsMe(t, srv, "just under 10 minutes after its push")

	srv.Clock.Advance(time.Millisecond)
	row := wantCIUnavailable(t, srv, core.CIWatcherNotWatching)
	if row.CIUnavailable.Message != "gh is logged out" {
		t.Fatalf("message %q, want the CI Watcher's error", row.CIUnavailable.Message)
	}

	watcherReports(t, srv, watching)
	wantNotInNeedsMe(t, srv, "with the CI Watcher Watching again")
}

func TestACIWatcherThatHasntReportedIsNotWatching(t *testing.T) {
	srv := newServer(t, pushedAccepted)
	reportRuns(t, srv, runningRun)
	srv.Clock.Advance(core.CIUnavailableAfter)
	wantCIUnavailable(t, srv, core.CIWatcherNotWatching)
}

func TestTheCIWatcherNotWatchingIsSaidBeforeNoCIRuns(t *testing.T) {
	srv := newServer(t, pushedAccepted)
	watcherReports(t, srv, loggedOut)
	srv.Clock.Advance(core.CIUnavailableAfter)
	wantCIUnavailable(t, srv, core.CIWatcherNotWatching)
}

func TestOnlyIgnoredRunsAreNoCIRuns(t *testing.T) {
	srv := newServer(t, pushedAccepted)
	watcherReports(t, srv, watching)
	reportRuns(t, srv, ciRun(1, "nightly.yml", "abc123", store.CICompleted, store.CISuccess))
	srv.Clock.Advance(core.CIUnavailableAfter)
	wantCIUnavailable(t, srv, core.CINoRuns)
}

func TestRunsForAnOlderPushedCommitAreNoCIRuns(t *testing.T) {
	srv := newServer(t, pushedAccepted)
	watcherReports(t, srv, watching)
	reportRuns(t, srv, ciRun(1, "ci.yml", "abc123", store.CIInProgress, ""))
	srv.Clock.Advance(core.CIUnavailableAfter)
	pushCommit(t, srv, "def456")
	wantNotInNeedsMe(t, srv, "just after a newer push")

	srv.Clock.Advance(core.CIUnavailableAfter)
	row, _ := needsMeRowOf(t, srv, "acme-3")
	if row.Reason != core.ReasonCIUnavailable || row.CIUnavailable.Cause != core.CINoRuns || row.CIUnavailable.Commit != "def456" {
		t.Fatalf("10 minutes after the newer push, row %+v, want ci_unavailable, no CI Runs at def456", row)
	}
}

func TestCIUnavailableClearsWhenTheGateLifts(t *testing.T) {
	for _, tc := range []struct {
		name string
		lift func(t *testing.T, srv *codecentraltest.Server)
	}{
		{"CI Tracking off", func(t *testing.T, srv *codecentraltest.Server) {
			httpOK(t, srv, setCITracking(`{"tracking":false,"repository":"acme/web"}`))
		}},
		{"Push off", func(t *testing.T, srv *codecentraltest.Server) {
			must(t, func() error {
				_, err := srv.Core.SetLandingMethod(core.WithActor(context.Background(), core.Human), "acme",
					core.SetLandingMethodInput{Method: store.LandingMerge})
				return err
			})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := newServer(t, pushedAccepted)
			srv.Clock.Advance(core.CIUnavailableAfter)
			wantCIUnavailable(t, srv, core.CIWatcherNotWatching)

			tc.lift(t, srv)
			row, ok := needsMeRowOf(t, srv, "acme-3")
			if ok && row.Reason == core.ReasonCIUnavailable {
				t.Fatalf("with the gate lifted, acme-3 is still CI unavailable: %+v", row)
			}
		})
	}
}

func TestAGatedTicketWhoseCIPassedIsNotCIUnavailable(t *testing.T) {
	srv := newServer(t, pushedAccepted)
	reportRuns(t, srv, ciRun(1, "ci.yml", "abc123", store.CICompleted, store.CISuccess))
	srv.Clock.Advance(core.NotLandedAfter + time.Minute)
	// The Factory lands a passed Ticket; if it hasn't, it isn't landing.
	row, _ := needsMeRowOf(t, srv, "acme-3")
	if row.Reason != core.ReasonAcceptedNotLanded {
		t.Fatalf("passed and still Accepted after 11 minutes, row %+v, want accepted_not_landed", row)
	}
}

func TestTheWatchNoticesATicketTurningCIUnavailable(t *testing.T) {
	srv := newServer(t, pushedAccepted)
	watcherReports(t, srv, watching)
	ticks := make(chan time.Time)
	changed := make(chan string, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Core.WatchTimedReasons(ctx, ticks, func(_, id string) { changed <- id })

	ticks <- time.Time{}
	srv.Clock.Advance(core.CIUnavailableAfter)
	ticks <- time.Time{}
	ticks <- time.Time{} // the second tick waits for the first's notice
	select {
	case id := <-changed:
		if id != "acme-3" {
			t.Fatalf("changed %s, want acme-3", id)
		}
	default:
		t.Fatal("no change notice for acme-3 turning CI unavailable")
	}
}
