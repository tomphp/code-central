package issuewatcher_test

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
	"github.com/tomoram/code-central/issuewatcher"
	"github.com/tomoram/code-central/watchers"
)

// supervising is a real Code Central and the service's supervision of
// its Watchers of kinds, each run as program, the test binary as
// code-central or as a Watcher that crashes, with the fake gh on PATH
// able to triage acme/web.
type supervising struct {
	t   *testing.T
	h   *ciwatchertest.Harness
	srv *codecentraltest.Server
	log *codecentraltest.SyncBuffer
	// stop stops the service's supervision, and done closes once every
	// Watcher has stopped.
	stop func()
	done chan struct{}
}

func newSupervising(t *testing.T) *supervising {
	t.Helper()
	h := ciwatchertest.New(t)
	repository(h, true, triage)
	return &supervising{t: t, h: h, srv: codecentraltest.NewServer(t), log: &codecentraltest.SyncBuffer{}}
}

// project creates key, with GitHub Issues on or off.
func (s *supervising) project(key string, on bool) {
	s.t.Helper()
	if _, err := s.srv.Core.CreateProject(human, core.CreateProjectInput{Key: key}); err != nil {
		s.t.Fatal(err)
	}
	s.issues(key, on)
}

// issues turns key's GitHub Issues on or off, as the human does.
func (s *supervising) issues(key string, on bool) {
	s.t.Helper()
	if _, err := s.srv.Core.SetGitHubIssues(human, key, core.SetGitHubIssuesInput{On: on, Repository: "acme/web"}); err != nil {
		s.t.Fatal(err)
	}
}

// start supervises kinds, with program as the service's executable.
func (s *supervising) start(program string, kinds ...watchers.Kind) {
	s.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	s.done = make(chan struct{})
	s.stop = cancel
	go func() {
		defer close(s.done)
		watchers.Supervise(ctx, watchers.Supervision{
			Projects: s.srv.Core, Kinds: kinds, Program: program, Clock: s.srv.Clock, Log: s.log,
			Environ: append(s.h.Environ(s.h.Bin), "CODE_CENTRAL_URL="+s.srv.URL),
		})
	}()
	s.t.Cleanup(func() {
		cancel()
		<-s.done
	})
}

// status waits for key's Issue Watcher status to be state, and returns
// it.
func (s *supervising) status(key string, state core.WatcherState) core.WatcherView {
	s.t.Helper()
	var got core.WatcherView
	codecentraltest.Eventually(s.t, func() bool {
		p, err := s.srv.Client.GetProject(context.Background(), key)
		if err != nil {
			s.t.Fatal(err)
		}
		got = p.GitHubIssues.Watcher
		return got.State == state
	}, "%s's Issue Watcher status %+v, want %s", key, &got, state)
	return got
}

// logged waits for the service's log to say line n times.
func (s *supervising) logged(line string, n int) {
	s.t.Helper()
	codecentraltest.Eventually(s.t, func() bool { return strings.Count(s.log.String(), line) >= n },
		"the service's log said %q fewer than %d times:\n%s", line, n, s.log)
}

// Both kinds run side by side: the Issue Watcher for each Project with
// GitHub Issues on, whether or not CI Tracking is.
func TestTheServiceStartsAnIssueWatcherForEachProjectWithGitHubIssuesOn(t *testing.T) {
	s := newSupervising(t)
	s.project("acme", true)
	s.project("beta", false)
	s.start(s.h.CodeCentral, ciwatcher.Kind, issuewatcher.Kind)
	s.status("acme", core.WatcherWatching)
	s.logged("started the Issue Watcher for acme", 1)

	s.issues("beta", true)
	s.status("beta", core.WatcherWatching)
	s.logged("started the Issue Watcher for beta", 1)
	if strings.Contains(s.log.String(), "CI Watcher") {
		t.Fatalf("a CI Watcher started with CI Tracking off:\n%s", s.log)
	}
}

func TestTurningGitHubIssuesOffStopsItsIssueWatcher(t *testing.T) {
	s := newSupervising(t)
	s.project("acme", true)
	s.start(s.h.CodeCentral, ciwatcher.Kind, issuewatcher.Kind)
	s.status("acme", core.WatcherWatching)
	s.issues("acme", false)
	s.logged("stopped the Issue Watcher for acme", 1)

	// And on again starts a new one.
	s.issues("acme", true)
	s.logged("started the Issue Watcher for acme", 2)
	s.status("acme", core.WatcherWatching)
}

func TestArchivingAProjectStopsItsIssueWatcher(t *testing.T) {
	s := newSupervising(t)
	s.project("acme", true)
	s.start(s.h.CodeCentral, ciwatcher.Kind, issuewatcher.Kind)
	s.status("acme", core.WatcherWatching)
	if _, err := s.srv.Core.ArchiveProject(human, "acme"); err != nil {
		t.Fatal(err)
	}
	s.logged("stopped the Issue Watcher for acme", 1)

	if _, err := s.srv.Core.UnarchiveProject(human, "acme"); err != nil {
		t.Fatal(err)
	}
	s.logged("started the Issue Watcher for acme", 2)
}

func TestStoppingTheServiceStopsEveryIssueWatcher(t *testing.T) {
	s := newSupervising(t)
	s.project("acme", true)
	s.project("beta", true)
	s.start(s.h.CodeCentral, ciwatcher.Kind, issuewatcher.Kind)
	s.status("acme", core.WatcherWatching)
	s.status("beta", core.WatcherWatching)
	s.stop()
	select {
	case <-s.done:
	case <-time.After(10 * time.Second):
		t.Fatal("supervision didn't end")
	}
	for _, key := range []string{"acme", "beta"} {
		if !strings.Contains(s.log.String(), "stopped the Issue Watcher for "+key) {
			t.Fatalf("%s's Issue Watcher wasn't stopped:\n%s", key, s.log)
		}
	}
}

// A crashing Issue Watcher is restarted with backoff, shown as crashed,
// restarted N times, with the last line it wrote, and one Notification
// for the lot.
func TestACrashingIssueWatcherIsRestartedWithBackoff(t *testing.T) {
	s := newSupervising(t)
	s.project("acme", true)
	s.start(s.h.Crashing, issuewatcher.Kind)
	for i, d := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second} {
		codecentraltest.Eventually(t, func() bool { return s.h.CrashRuns() == i+1 }, "the crashing Issue Watcher didn't start %d times", i+1)
		s.srv.Clock.BlockUntil(1)
		got := s.status("acme", core.WatcherError)
		times := fmt.Sprintf("%d times", i+1)
		if i == 0 {
			times = "1 time"
		}
		if got.Problem != core.ProblemCrashed || !strings.Contains(got.Message, "The Issue Watcher crashed, restarted "+times) ||
			!strings.Contains(got.Message, "broke on purpose") || !strings.Contains(got.Fix, "service's log") {
			t.Fatalf("status %+v, want crashed, restarted %s, with its last line on stderr", got, times)
		}
		s.srv.Clock.Advance(d - time.Millisecond)
		if s.srv.Clock.Waiters() != 1 {
			t.Fatalf("restarted before %v", d)
		}
		s.srv.Clock.Advance(time.Millisecond)
	}
	for _, a := range s.srv.Core.WatcherAttention(context.Background()) {
		if a.Kind == core.WatcherIssues && a.Errors != 1 {
			t.Fatalf("Issue Watcher attention %+v, want one error, for one Notification", a)
		}
	}
}
