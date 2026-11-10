package ciwatcher_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
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
	log *syncBuffer
	// stop stops the service's supervision, and done closes once every
	// CI Watcher has stopped.
	stop func()
	done chan struct{}
}

func newSupervising(t *testing.T) *supervising {
	t.Helper()
	return &supervising{t: t, h: ciwatchertest.New(t), srv: codecentraltest.NewServer(t), log: &syncBuffer{}}
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
	ctx, cancel := context.WithCancel(context.Background())
	s.done = make(chan struct{})
	s.stop = cancel
	go func() {
		defer close(s.done)
		ciwatcher.Supervise(ctx, ciwatcher.Supervision{
			Projects: s.srv.Core, Program: program, Clock: s.srv.Clock, Log: s.log,
			Environ: append(s.h.Environ(s.h.Bin), "CODE_CENTRAL_URL="+s.srv.URL),
		})
	}()
	s.t.Cleanup(func() {
		cancel()
		<-s.done
	})
}

// status waits for key's CI Watcher status to be state, and returns it.
func (s *supervising) status(key string, state core.CIWatcherState) core.CIWatcherView {
	s.t.Helper()
	var got core.CIWatcherView
	s.eventually(func() bool {
		p, err := s.srv.Client.GetProject(context.Background(), key)
		if err != nil {
			s.t.Fatal(err)
		}
		got = p.CI.Watcher
		return got.State == state
	}, "%s's CI Watcher status %+v, want %s", key, &got, state)
	return got
}

// logged waits for the service's log to say line n times.
func (s *supervising) logged(line string, n int) {
	s.t.Helper()
	s.eventually(func() bool { return strings.Count(s.log.String(), line) >= n },
		"the service's log said %q fewer than %d times:\n%s", line, n, s.log)
}

// crashRuns waits for the crashing CI Watcher to have started n times.
func (s *supervising) crashRuns(n int) {
	s.t.Helper()
	s.eventually(func() bool { return s.h.CrashRuns() >= n }, "the crashing CI Watcher didn't start %d times", n)
	if got := s.h.CrashRuns(); got != n {
		s.t.Fatalf("the crashing CI Watcher started %d times, want %d", got, n)
	}
}

// crashed waits for the service to wait out a crash's backoff, and
// checks the status it reports.
func (s *supervising) crashed(restarts string) {
	s.t.Helper()
	s.srv.Clock.BlockUntil(1)
	got := s.status("acme", core.CIWatcherError)
	if got.Problem != core.CICrashed || !strings.Contains(got.Message, "crashed, restarted "+restarts) ||
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

func (s *supervising) eventually(ok func() bool, format string, args ...any) {
	s.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			s.t.Fatalf(format, args...)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestTheServiceStartsACIWatcherForEachProjectWithCITrackingOn(t *testing.T) {
	s := newSupervising(t)
	s.project("acme", true)
	s.project("beta", false)
	s.start(s.h.CodeCentral)
	s.status("acme", core.CIWatcherWatching)
	s.logged("started the CI Watcher for acme", 1)

	// Turning CI Tracking on starts one.
	s.tracking("beta", true)
	s.status("beta", core.CIWatcherWatching)
	s.logged("started the CI Watcher for beta", 1)
	if strings.Count(s.log.String(), "started the CI Watcher for acme") != 1 {
		t.Fatalf("acme's CI Watcher started again:\n%s", s.log)
	}
}

func TestTurningCITrackingOffStopsItsCIWatcher(t *testing.T) {
	s := newSupervising(t)
	s.project("acme", true)
	s.start(s.h.CodeCentral)
	s.status("acme", core.CIWatcherWatching)
	s.tracking("acme", false)
	s.logged("stopped the CI Watcher for acme", 1)

	// And on again starts a new one.
	s.tracking("acme", true)
	s.logged("started the CI Watcher for acme", 2)
	s.status("acme", core.CIWatcherWatching)
}

func TestArchivingAProjectStopsItsCIWatcher(t *testing.T) {
	s := newSupervising(t)
	s.project("acme", true)
	s.start(s.h.CodeCentral)
	s.status("acme", core.CIWatcherWatching)
	if _, err := s.srv.Core.ArchiveProject(human, "acme"); err != nil {
		t.Fatal(err)
	}
	s.logged("stopped the CI Watcher for acme", 1)

	if _, err := s.srv.Core.UnarchiveProject(human, "acme"); err != nil {
		t.Fatal(err)
	}
	s.logged("started the CI Watcher for acme", 2)
}

func TestStoppingTheServiceStopsEveryCIWatcher(t *testing.T) {
	s := newSupervising(t)
	s.project("acme", true)
	s.project("beta", true)
	s.start(s.h.CodeCentral)
	s.status("acme", core.CIWatcherWatching)
	s.status("beta", core.CIWatcherWatching)
	s.stop()
	select {
	case <-s.done:
	case <-time.After(10 * time.Second):
		t.Fatal("supervision didn't end")
	}
	for _, key := range []string{"acme", "beta"} {
		if !strings.Contains(s.log.String(), "stopped the CI Watcher for "+key) {
			t.Fatalf("%s's CI Watcher wasn't stopped:\n%s", key, s.log)
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
	if got := s.srv.Core.CIWatcherAttention(context.Background()); len(got) != 1 || got[0].Errors != 1 {
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
	s.logged("stopped the CI Watcher for acme", 1)
	s.srv.Clock.Advance(time.Hour)
	if s.h.CrashRuns() != 1 {
		t.Fatal("a CI Watcher was restarted after CI Tracking was turned off")
	}
}

// syncBuffer is the service's log, written from several goroutines.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}
