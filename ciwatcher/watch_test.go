package ciwatcher_test

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/ciwatcher"
	"github.com/tomoram/code-central/ciwatcher/ciwatchertest"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/internal/fakegh"
)

// watching is a real Code Central with acme watching acme/web, and the
// CI Watcher's loop running against it and the fake gh until the test
// ends or its stdin closes.
type watching struct {
	t   *testing.T
	h   *ciwatchertest.Harness
	srv *codecentraltest.Server
	// stdin is the CI Watcher's stdin; closing it stops the loop.
	stdin *io.PipeWriter
	done  chan error
}

func newWatching(t *testing.T) *watching {
	t.Helper()
	srv := codecentraltest.NewServer(t)
	human := core.WithActor(context.Background(), core.Human)
	if _, err := srv.Core.CreateProject(human, core.CreateProjectInput{Key: "acme"}); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.Core.SetCITracking(human, "acme", core.SetCITrackingInput{Tracking: true, Repository: "acme/web"}); err != nil {
		t.Fatal(err)
	}
	return &watching{t: t, h: ciwatchertest.New(t), srv: srv}
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
	go func() {
		w.done <- ciwatcher.Watch(ctx, ciwatcher.Config{
			Client: w.srv.Client, Project: "acme", Clock: w.srv.Clock, Stdin: r, Log: io.Discard,
			Env: ciwatcher.Env{Environ: w.h.Environ(path...), InstallPlaces: []string{}},
		})
	}()
}

// status waits for the CI Watcher's status in acme's Project read to be
// state, and returns it.
func (w *watching) status(state core.CIWatcherState) core.CIWatcherView {
	w.t.Helper()
	var p core.ProjectView
	w.waitFor("CI Watcher status "+string(state), func() bool {
		var err error
		if p, err = w.srv.Client.GetProject(context.Background(), "acme"); err != nil {
			w.t.Fatal(err)
		}
		return p.CI.Watcher.State == state
	})
	return p.CI.Watcher
}

// waitFor waits up to 10 seconds for cond, failing the test naming what
// it waited for.
func (w *watching) waitFor(what string, cond func() bool) {
	w.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			w.t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// nextPoll waits for the loop to wait out its interval, then ends it.
func (w *watching) nextPoll() {
	w.t.Helper()
	w.srv.Clock.BlockUntil(1)
	w.srv.Clock.Advance(ciwatcher.Interval)
}

func TestTheCIWatcherReportsWatchingWhenItCanReachGitHub(t *testing.T) {
	w := newWatching(t)
	w.start(w.h.Bin)
	if got := w.status(core.CIWatcherWatching); got.Message != "" || got.Fix != "" {
		t.Fatalf("status %+v, want watching with no message", got)
	}
}

func TestALoggedOutGHIsAnErrorUntilItLogsIn(t *testing.T) {
	w := newWatching(t)
	w.h.LoggedOut()
	w.start(w.h.Bin)
	got := w.status(core.CIWatcherError)
	if got.Problem != core.CILoggedOut || !strings.Contains(got.Message, "not logged into") || !strings.Contains(got.Fix, "gh auth login") {
		t.Fatalf("status %+v, want logged out with gh auth login", got)
	}

	// It retries each poll, still in error, and recovers by itself.
	w.nextPoll()
	w.nextPoll()
	w.status(core.CIWatcherError)
	w.h.Answer(fakegh.Answer{Args: []string{"auth", "status"}})
	w.nextPoll()
	w.status(core.CIWatcherWatching)
}

func TestAnUnreadableRepositoryIsAnErrorUntilItCanBeRead(t *testing.T) {
	w := newWatching(t)
	w.h.Unreadable("acme/web")
	w.start(w.h.Bin)
	got := w.status(core.CIWatcherError)
	if got.Problem != core.CIRepositoryUnreadable || !strings.Contains(got.Message, "acme/web") || !strings.Contains(got.Fix, "owner/name") {
		t.Fatalf("status %+v, want the repository unreadable, naming it", got)
	}
	w.h.Answer(fakegh.Answer{Args: []string{"api", "repos/acme/web"}, Stdout: "acme/web\n"})
	w.nextPoll()
	w.status(core.CIWatcherWatching)
}

// The gh path Option is read on every poll, so setting it fixes a
// missing gh without a restart.
func TestAMissingGHIsAnErrorUntilTheGHPathOptionFindsIt(t *testing.T) {
	w := newWatching(t)
	w.start(t.TempDir())
	got := w.status(core.CIWatcherError)
	if got.Problem != core.CIGHMissing || !strings.Contains(got.Fix, "https://cli.github.com") {
		t.Fatalf("status %+v, want gh missing with how to install it", got)
	}
	human := core.WithActor(context.Background(), core.Human)
	gh := w.h.Bin
	if _, err := w.srv.Core.SetOptions(human, core.SetOptionsInput{GHPath: &gh}); err != nil {
		t.Fatal(err)
	}
	w.nextPoll()
	w.status(core.CIWatcherWatching)
}

func TestTheCIWatcherExitsWhenItsStdinCloses(t *testing.T) {
	w := newWatching(t)
	w.start(w.h.Bin)
	w.status(core.CIWatcherWatching)
	w.stdin.Close()
	select {
	case err := <-w.done:
		if err != nil {
			t.Fatalf("Watch = %v, want nil on its stdin closing", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the CI Watcher kept running after its stdin closed")
	}
}

// The service runs the CI Watcher as its own executable's hidden
// subcommand, with a pipe for stdin.
func TestTheCIWatcherSubcommandWatchesAndExitsWhenItsStdinCloses(t *testing.T) {
	w := newWatching(t)
	cmd := exec.Command(w.h.CodeCentral, ciwatcher.WatchArgs("acme")...)
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
	w.status(core.CIWatcherWatching)
	stdin.Close()
	select {
	case <-done:
		if exit != nil {
			t.Fatalf("the CI Watcher exited with %v, want 0", exit)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the CI Watcher kept running after its stdin closed")
	}
}
