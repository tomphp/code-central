package pullrequestwatcher_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/internal/fakegh"
	"github.com/tomoram/code-central/pullrequestwatcher"
	"github.com/tomoram/code-central/watchers/watchertest"
)

// supervising is a real Code Central with acme under Pull Request, and the
// service's supervision of the Pull Request Watcher, run as program.
type supervising struct {
	t   *testing.T
	h   *watchertest.Harness
	srv *codecentraltest.Server
	*watchertest.Supervising
}

func newSupervising(t *testing.T) *supervising {
	t.Helper()
	h := watchertest.New(t)
	h.Answer(fakegh.Answer{Args: []string{"api", repoPath}, Stdout: write})
	srv := codecentraltest.NewServer(t)
	must(t, func() error {
		_, err := srv.Core.CreateProject(human, api.CreateProjectInput{Key: "acme"})
		return err
	})
	must(t, func() error {
		_, err := srv.Core.ReportPushRemote(ctx, "acme", api.ReportPushRemoteInput{URL: "git@github.com:acme/web.git", Base: "main"})
		return err
	})
	return &supervising{t: t, h: h, srv: srv}
}

func (s *supervising) method(m domain.LandingMethod) {
	s.t.Helper()
	must(s.t, func() error {
		_, err := s.srv.Core.SetLandingMethod(human, "acme", api.SetLandingMethodInput{Method: m, Remote: "origin"})
		return err
	})
}

func (s *supervising) status(state api.WatcherState) api.WatcherView {
	s.t.Helper()
	var got api.WatcherView
	codecentraltest.Eventually(s.t, func() bool {
		p, err := s.srv.Client.GetProject(ctx, "acme")
		if err != nil {
			s.t.Fatal(err)
		}
		got = p.Landing.Watcher
		return got.State == state
	}, "the Pull Request Watcher's status %+v, want %s", &got, state)
	return got
}

func TestTheServiceRunsAPullRequestWatcherOnlyUnderPullRequestAndStopsItOnSwitchingAway(t *testing.T) {
	s := newSupervising(t)
	s.method(domain.LandingPullRequest)
	s.Supervising = s.h.Supervise(s.srv, s.h.CodeCentral, pullrequestwatcher.Kind)
	s.Logged("started the Pull Request Watcher for acme", 1)
	s.status(api.WatcherWatching)
	s.method(domain.LandingBranch)
	s.Logged("stopped the Pull Request Watcher for acme", 1)
	if got := s.status(api.WatcherNotReported); got.State != api.WatcherNotReported {
		t.Fatalf("status %+v, want not reported once switched away", got)
	}
}

func TestArchivingAProjectStopsItsPullRequestWatcher(t *testing.T) {
	s := newSupervising(t)
	s.method(domain.LandingPullRequest)
	s.Supervising = s.h.Supervise(s.srv, s.h.CodeCentral, pullrequestwatcher.Kind)
	s.Logged("started the Pull Request Watcher for acme", 1)
	must(t, func() error { _, err := s.srv.Core.ArchiveProject(human, "acme"); return err })
	s.Logged("stopped the Pull Request Watcher for acme", 1)
}

func TestACrashingPullRequestWatcherIsRestartedWithBackoffAndNotifiesOnce(t *testing.T) {
	s := newSupervising(t)
	s.method(domain.LandingPullRequest)
	s.Supervising = s.h.Supervise(s.srv, s.h.Crashing, pullrequestwatcher.Kind)
	for i, d := range []time.Duration{time.Second, 2 * time.Second} {
		codecentraltest.Eventually(t, func() bool { return s.h.CrashRuns() == i+1 }, "the crashing Pull Request Watcher didn't start %d times", i+1)
		s.srv.Clock.BlockUntil(1)
		got := s.status(api.WatcherError)
		times := fmt.Sprintf("%d times", i+1)
		if i == 0 {
			times = "1 time"
		}
		if got.Problem != api.ProblemCrashed || !strings.Contains(got.Message, "The Pull Request Watcher crashed, restarted "+times) {
			t.Fatalf("status %+v, want crashed, restarted %s", got, times)
		}
		s.srv.Clock.Advance(d)
	}
	for _, a := range s.srv.Core.WatcherAttention(ctx) {
		if a.Kind == api.WatcherPullRequests && a.Errors != 1 {
			t.Fatalf("attention %+v, want one error, for one Notification", a)
		}
	}
}
