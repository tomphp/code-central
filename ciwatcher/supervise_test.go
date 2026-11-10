package ciwatcher_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/ciwatcher"
	"github.com/tomoram/code-central/ciwatcher/ciwatchertest"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
)

// supervising is a real Code Central and the service's supervision of
// its CI Watchers, each run as program, the test binary as code-central
// or as a CI Watcher that crashes, with the fake gh on PATH.
type supervising struct {
	t   *testing.T
	h   *ciwatchertest.Harness
	srv *codecentraltest.Server
	// Supervising is the supervision, once started.
	*ciwatchertest.Supervising
}

func newSupervising(t *testing.T) *supervising {
	t.Helper()
	return &supervising{t: t, h: ciwatchertest.New(t), srv: codecentraltest.NewServer(t)}
}

var human = core.WithActor(context.Background(), core.Human)

// project creates key, with CI Tracking on or off.
func (s *supervising) project(key string, tracking bool) {
	s.t.Helper()
	if _, err := s.srv.Core.CreateProject(human, core.CreateProjectInput{Key: key}); err != nil {
		s.t.Fatal(err)
	}
	s.tracking(key, tracking)
}

// tracking turns key's CI Tracking on or off, as the human does.
func (s *supervising) tracking(key string, on bool) {
	s.t.Helper()
	if _, err := s.srv.Core.SetCITracking(human, key, core.SetCITrackingInput{Tracking: on, Repository: "acme/web"}); err != nil {
		s.t.Fatal(err)
	}
}

// start starts supervising, with program as the service's executable.
func (s *supervising) start(program string) {
	s.t.Helper()
	s.Supervising = s.h.Supervise(s.srv, program, ciwatcher.Kind)
}

// status waits for key's CI Watcher status to be state, and returns it.
func (s *supervising) status(key string, state core.WatcherState) core.WatcherView {
	s.t.Helper()
	var got core.WatcherView
	codecentraltest.Eventually(s.t, func() bool {
		p, err := s.srv.Client.GetProject(context.Background(), key)
		if err != nil {
			s.t.Fatal(err)
		}
		got = p.CI.Watcher
		return got.State == state
	}, "%s's CI Watcher status %+v, want %s", key, &got, state)
	return got
}

// crashRuns waits for the crashing CI Watcher to have started n times.
func (s *supervising) crashRuns(n int) {
	s.t.Helper()
	codecentraltest.Eventually(s.t, func() bool { return s.h.CrashRuns() >= n }, "the crashing CI Watcher didn't start %d times", n)
	if got := s.h.CrashRuns(); got != n {
		s.t.Fatalf("the crashing CI Watcher started %d times, want %d", got, n)
	}
}

// crashed waits for the service to wait out a crash's backoff, and
// checks the status it reports.
func (s *supervising) crashed(restarts string) {
	s.t.Helper()
	s.srv.Clock.BlockUntil(1)
	got := s.status("acme", core.WatcherError)
	if got.Problem != core.ProblemCrashed || !strings.Contains(got.Message, "crashed, restarted "+restarts) ||
		!strings.Contains(got.Message, "panic: the CI Watcher broke on purpose") || !strings.Contains(got.Fix, "service's log") {
		s.t.Fatalf("status %+v, want crashed, restarted %s, with its last line on stderr", got, restarts)
	}
}

// restartsAfter checks the service restarts the CI Watcher only once d
// has passed since the crash.
func (s *supervising) restartsAfter(d time.Duration, runs int) {
	s.t.Helper()
	s.srv.Clock.Advance(d - time.Millisecond)
	if s.srv.Clock.Waiters() != 1 {
		s.t.Fatalf("restarted before %v", d)
	}
	s.srv.Clock.Advance(time.Millisecond)
	s.crashRuns(runs)
}

func TestTheServiceStartsACIWatcherForEachProjectWithCITrackingOn(t *testing.T) {
	s := newSupervising(t)
	s.project("acme", true)
	s.project("beta", false)
	s.start(s.h.CodeCentral)
	s.status("acme", core.WatcherWatching)
	s.Logged("started the CI Watcher for acme", 1)

	// Turning CI Tracking on starts one.
	s.tracking("beta", true)
	s.status("beta", core.WatcherWatching)
	s.Logged("started the CI Watcher for beta", 1)
	if strings.Count(s.Log.String(), "started the CI Watcher for acme") != 1 {
		t.Fatalf("acme's CI Watcher started again:\n%s", s.Log)
	}
}

func TestTurningCITrackingOffStopsItsCIWatcher(t *testing.T) {
	s := newSupervising(t)
	s.project("acme", true)
	s.start(s.h.CodeCentral)
	s.status("acme", core.WatcherWatching)
	s.tracking("acme", false)
	s.Logged("stopped the CI Watcher for acme", 1)

	// And on again starts a new one.
	s.tracking("acme", true)
	s.Logged("started the CI Watcher for acme", 2)
	s.status("acme", core.WatcherWatching)
}

func TestArchivingAProjectStopsItsCIWatcher(t *testing.T) {
	s := newSupervising(t)
	s.project("acme", true)
	s.start(s.h.CodeCentral)
	s.status("acme", core.WatcherWatching)
	if _, err := s.srv.Core.ArchiveProject(human, "acme"); err != nil {
		t.Fatal(err)
	}
	s.Logged("stopped the CI Watcher for acme", 1)

	if _, err := s.srv.Core.UnarchiveProject(human, "acme"); err != nil {
		t.Fatal(err)
	}
	s.Logged("started the CI Watcher for acme", 2)
}

func TestStoppingTheServiceStopsEveryCIWatcher(t *testing.T) {
	s := newSupervising(t)
	s.project("acme", true)
	s.project("beta", true)
	s.start(s.h.CodeCentral)
	s.status("acme", core.WatcherWatching)
	s.status("beta", core.WatcherWatching)
	s.Stop()
	select {
	case <-s.Done:
	case <-time.After(10 * time.Second):
		t.Fatal("supervision didn't end")
	}
	for _, key := range []string{"acme", "beta"} {
		if !strings.Contains(s.Log.String(), "stopped the CI Watcher for "+key) {
			t.Fatalf("%s's CI Watcher wasn't stopped:\n%s", key, s.Log)
		}
	}
}

// Backoff starts at 1 second and doubles to at most 5 minutes; the
// crash shows in the status, with one Notification for the lot.
func TestACrashingCIWatcherIsRestartedWithBackoff(t *testing.T) {
	s := newSupervising(t)
	s.project("acme", true)
	s.start(s.h.Crashing)
	s.crashRuns(1)
	s.crashed("1 time")
	backoffs := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second,
		32 * time.Second, 64 * time.Second, 128 * time.Second, 256 * time.Second, 5 * time.Minute, 5 * time.Minute}
	for i, d := range backoffs {
		s.restartsAfter(d, i+2)
		s.crashed(fmt.Sprintf("%d times", i+2))
	}
	if got := s.srv.Core.WatcherAttention(context.Background()); len(got) != 1 || got[0].Errors != 1 {
		t.Fatalf("CI Watcher attention %+v, want one error, for one Notification", got)
	}
}

// After 10 minutes of running, a crash starts the backoff and the count
// again.
func TestTheBackoffResetsAfterTenMinutesOfRunning(t *testing.T) {
	s := newSupervising(t)
	s.project("acme", true)
	s.start(s.h.Crashing)
	s.crashRuns(1)
	s.crashed("1 time")
	s.restartsAfter(time.Second, 2)
	s.crashed("2 times")

	s.h.HoldCrashes()
	s.restartsAfter(2*time.Second, 3)
	s.srv.Clock.Advance(10 * time.Minute)
	s.h.ReleaseCrashes()
	s.crashed("1 time")
	s.restartsAfter(time.Second, 4)
	s.crashed("2 times")
}

func TestACIWatcherStoppedWhileWaitingToRestartIsNotRestarted(t *testing.T) {
	s := newSupervising(t)
	s.project("acme", true)
	s.start(s.h.Crashing)
	s.crashRuns(1)
	s.crashed("1 time")
	s.tracking("acme", false)
	s.Logged("stopped the CI Watcher for acme", 1)
	s.srv.Clock.Advance(time.Hour)
	if s.h.CrashRuns() != 1 {
		t.Fatal("a CI Watcher was restarted after CI Tracking was turned off")
	}
}
