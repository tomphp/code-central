package issuewatcher_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/ciwatcher/ciwatchertest"
	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/internal/fakegh"
	"github.com/tomoram/code-central/issuewatcher"
	"github.com/tomoram/code-central/watchers"
)

var human = core.WithActor(context.Background(), core.Human)

// watching is a real Code Central with GitHub Issues on for acme, from
// acme/web, which gh can triage, and the Issue Watcher's loop running
// against it and the fake gh, as the integration Actor, until the test
// ends or its stdin closes.
type watching struct {
	t   *testing.T
	h   *ciwatchertest.Harness
	srv *codecentraltest.Server
	// stdin is the Issue Watcher's stdin; closing it stops the loop.
	stdin *io.PipeWriter
	done  chan error
}

func newWatching(t *testing.T) *watching {
	t.Helper()
	srv := codecentraltest.NewServer(t)
	if _, err := srv.Core.CreateProject(human, core.CreateProjectInput{Key: "acme"}); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.Core.SetGitHubIssues(human, "acme", core.SetGitHubIssuesInput{On: true, Repository: "acme/web"}); err != nil {
		t.Fatal(err)
	}
	h := ciwatchertest.New(t)
	repository(h, true, triage)
	return &watching{t: t, h: h, srv: srv}
}

// start runs the loop, gh found on path or nowhere.
func (w *watching) start(path ...string) {
	w.t.Helper()
	r, pw := io.Pipe()
	w.stdin, w.done = pw, make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	w.t.Cleanup(func() {
		cancel()
		pw.Close()
	})
	c := client.New(w.srv.URL)
	c.Actor = core.Integration
	go func() {
		w.done <- issuewatcher.Watch(ctx, issuewatcher.Config{
			Client: c, Project: "acme", Clock: w.srv.Clock, Stdin: r, Log: io.Discard,
			Env: gh.Env{Environ: w.h.Environ(path...), InstallPlaces: []string{}},
		})
	}()
}

// status waits for the Issue Watcher's status in acme's Project read to
// be state, and returns it.
func (w *watching) status(state core.WatcherState) core.WatcherView {
	w.t.Helper()
	var got core.WatcherView
	codecentraltest.Eventually(w.t, func() bool {
		got = settings(w.t, w.srv).Watcher
		return got.State == state
	}, "the Issue Watcher's status %+v, want %s", &got, state)
	return got
}

// nextPoll waits for the loop to wait out its interval, then ends it.
func (w *watching) nextPoll() {
	w.t.Helper()
	w.srv.Clock.BlockUntil(1)
	w.srv.Clock.Advance(issuewatcher.Interval)
}

// errorCount is how many times the Issue Watcher went into an error,
// each one Notification.
func (w *watching) errorCount() int {
	for _, a := range w.srv.Core.WatcherAttention(context.Background()) {
		if a.Kind == core.WatcherIssues && a.Project == "acme" {
			return a.Errors
		}
	}
	return 0
}

func TestTheIssueWatcherReportsWatchingWhenItCanTriageTheRepositorysIssues(t *testing.T) {
	w := newWatching(t)
	w.start(w.h.Bin)
	if got := w.status(core.WatcherWatching); got.Message != "" || got.Fix != "" || !got.RateLimitedUntil.IsZero() {
		t.Fatalf("status %+v, want watching with no message", got)
	}
	// It polls every 2 minutes, reading the repository each time.
	reads := func() int {
		n := 0
		for _, c := range w.h.Calls() {
			if len(c.Args) > 1 && c.Args[0] == "api" && c.Args[1] == "repos/acme/web" {
				n++
			}
		}
		return n
	}
	w.srv.Clock.BlockUntil(1)
	w.srv.Clock.Advance(issuewatcher.Interval - time.Second)
	if got := reads(); got != 1 {
		t.Fatalf("%d reads of acme/web before 2 minutes, want 1", got)
	}
	w.srv.Clock.Advance(time.Second)
	codecentraltest.Eventually(t, func() bool { return reads() == 2 }, "the Issue Watcher didn't poll again after 2 minutes")
}

// Each error has its own fix, and the Issue Watcher retries each poll,
// still in error, so it sends one Notification, and goes back to
// Watching by itself once the cause is fixed.
func TestEachErrorIsReportedWithItsFixUntilItIsFixed(t *testing.T) {
	for _, c := range []struct {
		name        string
		cause, fix  func(h *ciwatchertest.Harness)
		problem     core.WatcherProblem
		message, to string
	}{
		{"logged out", func(h *ciwatchertest.Harness) { h.LoggedOut() }, func(h *ciwatchertest.Harness) { h.Answer(fakegh.Answer{Args: []string{"auth", "status"}}) },
			core.ProblemLoggedOut, "not logged into", "gh auth login"},
		{"unreadable", func(h *ciwatchertest.Harness) { h.Unreadable("acme/web") }, func(h *ciwatchertest.Harness) { repository(h, true, triage) },
			core.ProblemRepositoryUnreadable, "can't read the repository acme/web", "owner/name"},
		{"issues disabled", func(h *ciwatchertest.Harness) { repository(h, false, write) }, func(h *ciwatchertest.Harness) { repository(h, true, write) },
			core.ProblemIssuesDisabled, "acme/web has its issues turned off", "Features"},
		{"below triage", func(h *ciwatchertest.Harness) { repository(h, true, read) }, func(h *ciwatchertest.Harness) { repository(h, true, triage) },
			core.ProblemNoTriage, "can't triage the issues of acme/web", "triage permission"},
		{"unreadable answer", func(h *ciwatchertest.Harness) {
			h.Answer(fakegh.Answer{Args: []string{"api", "repos/acme/web"}, Stdout: "not json"})
		}, func(h *ciwatchertest.Harness) { repository(h, true, triage) }, core.ProblemCheckFailed, "acme/web", "gh works"},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := newWatching(t)
			c.cause(w.h)
			w.start(w.h.Bin)
			got := w.status(core.WatcherError)
			if got.Problem != c.problem || !strings.Contains(got.Message, c.message) || !strings.Contains(got.Fix, c.to) {
				t.Fatalf("status %+v, want %s, %q, with the fix %q", got, c.problem, c.message, c.to)
			}
			w.nextPoll()
			w.nextPoll()
			w.status(core.WatcherError)
			if n := w.errorCount(); n != 1 {
				t.Fatalf("went into an error %d times, want once, for one Notification", n)
			}
			c.fix(w.h)
			w.nextPoll()
			w.status(core.WatcherWatching)
		})
	}
}

// The gh path Option is read on every poll, so setting it fixes a
// missing gh without a restart.
func TestAMissingGHIsAnErrorUntilTheGHPathOptionFindsIt(t *testing.T) {
	w := newWatching(t)
	w.start(t.TempDir())
	got := w.status(core.WatcherError)
	if got.Problem != core.ProblemGHMissing || !strings.Contains(got.Fix, "https://cli.github.com") {
		t.Fatalf("status %+v, want gh missing with how to install it", got)
	}
	ghPath := w.h.Bin
	if _, err := w.srv.Core.SetOptions(human, core.SetOptionsInput{GHPath: &ghPath}); err != nil {
		t.Fatal(err)
	}
	w.nextPoll()
	w.status(core.WatcherWatching)
}

// rateLimited scripts GitHub's rate-limit answer to reading acme/web,
// with headers such as X-Ratelimit-Reset, as gh api --include prints
// them.
func (w *watching) rateLimited(headers ...string) {
	w.t.Helper()
	head := "HTTP/2.0 403 Forbidden\r\nContent-Type: application/json; charset=utf-8\r\n"
	for _, h := range headers {
		head += h + "\r\n"
	}
	w.h.Answer(fakegh.Answer{Args: []string{"api", "repos/acme/web"}, Exit: 1,
		Stdout: head + "\r\n" + `{"message":"API rate limit exceeded for user ID 1.","status":"403"}`,
		Stderr: "gh: API rate limit exceeded for user ID 1. (HTTP 403)\n"})
}

// rateLimitedUntil waits for the Issue Watcher to report Watching,
// rate-limited until until.
func (w *watching) rateLimitedUntil(until time.Time) {
	w.t.Helper()
	var got core.WatcherView
	codecentraltest.Eventually(w.t, func() bool {
		got = settings(w.t, w.srv).Watcher
		return got.State == core.WatcherWatching && got.RateLimitedUntil.Equal(until)
	}, "the Issue Watcher's status %+v, want Watching, rate-limited until %s", &got, until)
}

// Rate-limited, the Issue Watcher is still Watching, with when it calls
// GitHub again, makes no calls until GitHub's reset time, and sends no
// Notification.
func TestARateLimitedIssueWatcherMakesNoGitHubCallsUntilTheResetTime(t *testing.T) {
	w := newWatching(t)
	reset := w.srv.Clock.Now().Add(15 * time.Minute)
	w.rateLimited("X-Ratelimit-Remaining: 0", fmt.Sprintf("X-Ratelimit-Reset: %d", reset.Unix()))
	w.start(w.h.Bin)
	w.rateLimitedUntil(reset)

	calls := len(w.h.Calls())
	w.srv.Clock.BlockUntil(1)
	w.srv.Clock.Advance(15*time.Minute - time.Second)
	if n := w.srv.Clock.Waiters(); n != 1 {
		t.Fatalf("%d timers waiting a second before the reset, want the Issue Watcher's still", n)
	}
	if got := len(w.h.Calls()); got != calls {
		t.Fatalf("%d gh calls before the reset, want none after the rate limit's %d", got, calls)
	}

	// At the reset it polls again, and is no longer rate-limited.
	repository(w.h, true, triage)
	w.srv.Clock.Advance(time.Second)
	w.rateLimitedUntil(time.Time{})
	if n := w.errorCount(); n != 0 {
		t.Fatalf("went into an error %d times, want none: a rate limit sends no Notification", n)
	}
}

// With no reset time, it doubles its interval each time, up to 10
// minutes, and goes back to its own once GitHub answers.
func TestWithoutAResetTimeTheIssueWatchersIntervalDoublesUpTo10Minutes(t *testing.T) {
	w := newWatching(t)
	w.rateLimited()
	w.start(w.h.Bin)
	waits := []time.Duration{4 * time.Minute, 8 * time.Minute, watchers.MaxRateLimitInterval, watchers.MaxRateLimitInterval}
	for i, wait := range waits {
		w.rateLimitedUntil(w.srv.Clock.Now().Add(wait))
		if i == len(waits)-1 {
			repository(w.h, true, triage)
		}
		w.srv.Clock.BlockUntil(1)
		w.srv.Clock.Advance(wait)
	}
	w.rateLimitedUntil(time.Time{})
	w.nextPoll()
	w.status(core.WatcherWatching)
	if n := w.errorCount(); n != 0 {
		t.Fatalf("went into an error %d times, want none: a rate limit sends no Notification", n)
	}
}

// Turned off, GitHub Issues has no Issue Watcher status to report: the
// loop reports nothing until it is stopped.
func TestTheIssueWatcherReportsNothingWithGitHubIssuesOff(t *testing.T) {
	w := newWatching(t)
	if _, err := w.srv.Core.SetGitHubIssues(human, "acme", core.SetGitHubIssuesInput{On: false, Repository: "acme/web"}); err != nil {
		t.Fatal(err)
	}
	w.start(w.h.Bin)
	w.srv.Clock.BlockUntil(1)
	if calls := w.h.Calls(); len(calls) != 0 {
		t.Fatalf("gh calls %v with GitHub Issues off, want none", calls)
	}
	if got := settings(t, w.srv).Watcher; got.State != core.WatcherNotReported {
		t.Fatalf("status %+v, want not reported", got)
	}
}

func TestTheIssueWatcherExitsWhenItsStdinCloses(t *testing.T) {
	w := newWatching(t)
	w.start(w.h.Bin)
	w.status(core.WatcherWatching)
	w.stdin.Close()
	select {
	case err := <-w.done:
		if err != nil {
			t.Fatalf("Watch = %v, want nil on its stdin closing", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the Issue Watcher kept running after its stdin closed")
	}
}

// The service runs the Issue Watcher as its own executable's hidden
// subcommand, with a pipe for stdin; it writes as the integration Actor,
// whose status report alone is accepted.
func TestTheIssueWatcherSubcommandWatchesAndExitsWhenItsStdinCloses(t *testing.T) {
	w := newWatching(t)
	cmd := exec.Command(w.h.CodeCentral, issuewatcher.WatchArgs("acme")...)
	cmd.Env = append(w.h.Environ(w.h.Bin), "CODE_CENTRAL_URL="+w.srv.URL)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var exit error
	done := make(chan struct{})
	go func() {
		exit = cmd.Wait()
		close(done)
	}()
	t.Cleanup(func() {
		cmd.Process.Kill()
		<-done
	})
	w.status(core.WatcherWatching)
	stdin.Close()
	select {
	case <-done:
		if exit != nil {
			t.Fatalf("the Issue Watcher exited with %v, want 0", exit)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the Issue Watcher kept running after its stdin closed")
	}
}
