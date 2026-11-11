package pullrequestwatcher_test

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/ciwatcher/ciwatchertest"
	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/internal/fakegh"
	"github.com/tomoram/code-central/pullrequestwatcher"
	"github.com/tomoram/code-central/store"
)

func TestMain(m *testing.M) { ciwatchertest.Main(m) }

var (
	ctx   = context.Background()
	human = core.WithActor(ctx, core.Human)
)

const (
	repoPath = "repos/acme/web"
	listPath = repoPath + "/pulls?state=all&sort=created&direction=desc&per_page=100&head=acme%3Aacme-2&base=main"
	write    = `{"full_name":"acme/web","permissions":{"admin":false,"push":true,"pull":true}}`
	readOnly = `{"full_name":"acme/web","permissions":{"admin":false,"push":false,"pull":true}}`
)

// watching is a real Code Central with Pull Request on for acme, whose
// Push remote is on GitHub, with acme-2 accepted and pushed as its branch,
// and the Pull Request Watcher's loop running against it and the fake gh
// until the test ends or its stdin closes.
type watching struct {
	t     *testing.T
	h     *ciwatchertest.Harness
	srv   *codecentraltest.Server
	stdin *io.PipeWriter
	done  chan error
}

func newWatching(t *testing.T) *watching { return newWatchingOn(t, "git@github.com:acme/web.git") }

// newWatchingOn is newWatching with the Push remote at remote.
func newWatchingOn(t *testing.T, remote string) *watching {
	t.Helper()
	srv := codecentraltest.NewServer(t)
	c := srv.Core
	must(t, func() error { _, err := c.CreateProject(human, core.CreateProjectInput{Key: "acme"}); return err })
	must(t, func() error {
		_, err := c.ReportPushRemote(ctx, "acme", core.ReportPushRemoteInput{URL: remote, Base: "main"})
		return err
	})
	must(t, func() error {
		_, err := c.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingPullRequest})
		return err
	})
	must(t, func() error { _, err := c.CreateSpec(ctx, "acme", core.CreateSpecInput{Title: "Login"}); return err })
	// Long-lived, so that its Ticket gets a pull request of its own.
	must(t, func() error {
		_, err := c.AddLabels(ctx, "acme", core.LabelsInput{Target: "acme-1", Labels: []string{core.LongLivedLabel}})
		return err
	})
	must(t, func() error {
		_, err := c.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "Login form", State: store.Ready})
		return err
	})
	res, err := c.Claim(ctx, "acme", core.ClaimInput{Ticket: "2"})
	if err != nil {
		t.Fatal(err)
	}
	must(t, func() error {
		_, err := c.Finish(ctx, "acme", core.FinishInput{Ticket: "2", ClaimID: res.ClaimID, Summary: "Built the login form.", Branch: "acme-2", Commit: "abc123"})
		return err
	})
	must(t, func() error { _, err := c.Accept(human, "acme", core.TicketInput{Ticket: "2"}); return err })
	must(t, func() error {
		_, err := srv.Client.ReportPushedCommit(ctx, "acme", core.ReportPushedCommitInput{Ticket: "acme-2", Commit: "abc123", Branch: "acme-2"})
		return err
	})
	h := ciwatchertest.New(t)
	h.Answer(fakegh.Answer{Args: []string{"api", repoPath}, Stdout: write})
	return &watching{t: t, h: h, srv: srv}
}

func must(t *testing.T, f func() error) {
	t.Helper()
	if err := f(); err != nil {
		t.Fatal(err)
	}
}

// start runs the loop, gh found on path or nowhere.
func (w *watching) start(path ...string) {
	w.t.Helper()
	r, pw := io.Pipe()
	w.stdin, w.done = pw, make(chan error, 1)
	cctx, cancel := context.WithCancel(context.Background())
	w.t.Cleanup(func() {
		cancel()
		pw.Close()
	})
	go func() {
		w.done <- pullrequestwatcher.Watch(cctx, pullrequestwatcher.Config{
			Client: client.New(w.srv.URL), Project: "acme", Clock: w.srv.Clock, Stdin: r, Log: io.Discard,
			Env: gh.Env{Environ: w.h.Environ(path...), InstallPlaces: []string{}},
		})
	}()
}

// answerList scripts the pull requests GitHub lists from acme-2 into main.
func (w *watching) answerList(prs ...string) {
	w.h.Answer(fakegh.Answer{Args: []string{"api", listPath}, Stdout: "[" + strings.Join(prs, ",") + "]"})
}

// pr is a pull request as GitHub's REST API gives it.
func pr(n int, state, extra string) string {
	if extra != "" {
		extra = "," + extra
	}
	return fmt.Sprintf(`{"number":%d,"html_url":"https://github.com/acme/web/pull/%d","state":%q%s}`, n, n, state, extra)
}

// answerOne scripts GitHub's read of pull request n alone.
func (w *watching) answerOne(n int, body string) {
	w.h.Answer(fakegh.Answer{Args: []string{"api", fmt.Sprintf("%s/pulls/%d", repoPath, n)}, Stdout: body})
}

// recorded is the pull request the Pull Request Watcher reported for acme-2.
func (w *watching) recorded(state store.PullRequestState) *core.PullRequestView {
	w.t.Helper()
	var got *core.PullRequestView
	codecentraltest.Eventually(w.t, func() bool {
		watch, err := w.srv.Core.GetPullRequestWatch(ctx, "acme")
		if err != nil || len(watch.Work) != 1 {
			return false
		}
		got = watch.Work[0].PullRequest
		return got != nil && got.State == state
	}, "acme-2's pull request is %+v, want %s", &got, state)
	return got
}

// status waits for the Pull Request Watcher's status in acme's Project.
func (w *watching) status(state core.WatcherState) core.WatcherView {
	w.t.Helper()
	var got core.WatcherView
	codecentraltest.Eventually(w.t, func() bool {
		p, err := w.srv.Client.GetProject(ctx, "acme")
		if err != nil {
			w.t.Fatal(err)
		}
		got = p.Landing.Watcher
		return got.State == state
	}, "the Pull Request Watcher's status %+v, want %s", &got, state)
	return got
}

// calls is the gh calls whose args start with prefix.
func (w *watching) calls(prefix ...string) [][]string {
	var out [][]string
	for _, c := range w.h.Calls() {
		if len(c.Args) >= len(prefix) && slices.Equal(c.Args[:len(prefix)], prefix) {
			out = append(out, c.Args)
		}
	}
	return out
}

func (w *watching) opened() [][]string { return w.calls("api", repoPath+"/pulls", "--method", "POST") }
func (w *watching) lists() int         { return len(w.calls("api", listPath)) }

// nextPoll waits for the loop to wait, then advances the clock by d.
func (w *watching) nextPoll(d time.Duration) {
	w.t.Helper()
	w.srv.Clock.BlockUntil(1)
	w.srv.Clock.Advance(d)
}

// errorCount is how many times the Pull Request Watcher went into an
// error, each one Notification.
func (w *watching) errorCount() int {
	for _, a := range w.srv.Core.WatcherAttention(ctx) {
		if a.Kind == core.WatcherPullRequests && a.Project == "acme" {
			return a.Errors
		}
	}
	return 0
}

func TestItOpensAReadyPullRequestIntoTheBaseWithTheTicketsTitleAndSummaryAndReportsIt(t *testing.T) {
	w := newWatching(t)
	w.answerList()
	w.h.Answer(fakegh.Answer{Args: []string{"api", repoPath + "/pulls", "--method", "POST"}, Stdout: pr(7, "open", "")})
	w.start(w.h.Bin)
	got := w.recorded(store.PROpen)
	if got.Number != 7 || got.Link != "https://github.com/acme/web/pull/7" {
		t.Fatalf("recorded %+v, want pull request 7 and its link", got)
	}
	w.status(core.WatcherWatching)
	opened := w.opened()
	if len(opened) != 1 {
		t.Fatalf("opened %d pull requests, want one: %q", len(opened), opened)
	}
	args := strings.Join(opened[0], "\x00")
	for _, want := range []string{"title=Login form", "head=acme-2", "base=main", "Built the login form.", "acme-2"} {
		if !strings.Contains(args, want) {
			t.Errorf("open call %q lacks %q", opened[0], want)
		}
	}
	if strings.Contains(args, "draft") {
		t.Errorf("open call %q sets draft, want ready for review", opened[0])
	}
	// Once reported, it doesn't open another: GitHub now lists it.
	w.answerList(pr(7, "open", ""))
	w.nextPoll(pullrequestwatcher.Interval)
	codecentraltest.Eventually(t, func() bool { return w.lists() == 2 }, "no second poll")
	w.srv.Clock.BlockUntil(1)
	if n := len(w.opened()); n != 1 {
		t.Fatalf("opened %d pull requests after a second poll, want still one", n)
	}
}

func TestItAdoptsAnOpenPullRequestFromTheBranchInsteadOfOpeningOne(t *testing.T) {
	w := newWatching(t)
	w.answerList(pr(4, "open", ""))
	w.start(w.h.Bin)
	if got := w.recorded(store.PROpen); got.Number != 4 {
		t.Fatalf("recorded %+v, want the existing pull request 4", got)
	}
	if n := len(w.opened()); n != 0 {
		t.Fatalf("opened %d pull requests, want none", n)
	}
}

func TestItReportsEachStateAndTheMergeCommit(t *testing.T) {
	w := newWatching(t)
	w.answerList(pr(4, "open", ""))
	w.answerOne(4, pr(4, "open", `"mergeable":true,"mergeable_state":"clean"`))
	w.start(w.h.Bin)
	w.recorded(store.PROpen)

	// GitHub says it can't merge cleanly.
	w.answerOne(4, pr(4, "open", `"mergeable":false,"mergeable_state":"dirty"`))
	w.nextPoll(pullrequestwatcher.Interval)
	w.recorded(store.PRConflicting)

	// Closed without merging.
	w.answerList(pr(4, "closed", `"closed_at":"2099-01-01T00:00:00Z","merged_at":null`))
	w.nextPoll(pullrequestwatcher.Interval)
	w.recorded(store.PRClosed)

	// Merged, at the commit the host made, however it merged.
	w.answerList(pr(4, "closed", `"closed_at":"2099-01-01T00:00:00Z","merged_at":"2099-01-01T00:00:00Z","merge_commit_sha":"def4567"`))
	w.nextPoll(pullrequestwatcher.Interval)
	if got := w.recorded(store.PRMerged); got.MergeCommit != "def4567" {
		t.Fatalf("recorded %+v, want the merge commit def4567", got)
	}
	if n := len(w.opened()); n != 0 {
		t.Fatalf("opened %d pull requests, want none", n)
	}
}

func TestAClosedPullRequestIsPolledSoAReopenedOneIsSeen(t *testing.T) {
	w := newWatching(t)
	w.answerList(pr(4, "closed", `"closed_at":"2099-01-01T00:00:00Z","merged_at":null`))
	w.start(w.h.Bin)
	w.recorded(store.PRClosed)
	if n := len(w.opened()); n != 0 {
		t.Fatalf("opened %d pull requests while the closed one is newer than the push, want none", n)
	}
	w.answerList(pr(4, "open", ""))
	w.nextPoll(pullrequestwatcher.Interval)
	if got := w.recorded(store.PROpen); got.Number != 4 {
		t.Fatalf("recorded %+v, want pull request 4 open again", got)
	}
}

func TestANewPullRequestOpensAfterAClosedOneWhenTheLatestPushIsNewer(t *testing.T) {
	w := newWatching(t)
	w.answerList(pr(4, "closed", `"closed_at":"2000-01-01T00:00:00Z","merged_at":null`))
	w.h.Answer(fakegh.Answer{Args: []string{"api", repoPath + "/pulls", "--method", "POST"}, Stdout: pr(5, "open", "")})
	w.start(w.h.Bin)
	if got := w.recorded(store.PROpen); got.Number != 5 {
		t.Fatalf("recorded %+v, want the new pull request 5", got)
	}
	if n := len(w.opened()); n != 1 {
		t.Fatalf("opened %d pull requests, want one", n)
	}
}

func TestAMergedPullRequestIsNotReplacedByANewOneAfterANewerPush(t *testing.T) {
	w := newWatching(t)
	w.answerList(pr(4, "closed", `"closed_at":"2000-01-01T00:00:00Z","merged_at":"2000-01-01T00:00:00Z","merge_commit_sha":"def4567"`))
	w.start(w.h.Bin)
	w.recorded(store.PRMerged)
	if n := len(w.opened()); n != 0 {
		t.Fatalf("opened %d pull requests, want none beside a merged one", n)
	}
}

// It polls every 2 minutes, and every 30 seconds while pushed work has no
// pull request.
func TestItPollsEveryTwoMinutesAndEveryThirtySecondsWhileWorkLacksAPullRequest(t *testing.T) {
	w := newWatching(t)
	// Opening fails, so the work still has no pull request.
	w.answerList()
	w.h.Answer(fakegh.Answer{Args: []string{"api", repoPath + "/pulls", "--method", "POST"}, Exit: 1,
		Stdout: "HTTP/2.0 422 Unprocessable Entity\r\n\r\n{}", Stderr: "gh: Validation Failed (HTTP 422)\n"})
	w.start(w.h.Bin)
	codecentraltest.Eventually(t, func() bool { return w.lists() == 1 }, "no first poll")
	w.srv.Clock.BlockUntil(1)
	w.srv.Clock.Advance(pullrequestwatcher.WaitingInterval - time.Second)
	if w.lists() != 1 {
		t.Fatal("polled again before 30 seconds")
	}
	w.srv.Clock.Advance(time.Second)
	codecentraltest.Eventually(t, func() bool { return w.lists() == 2 }, "no poll after 30 seconds")

	// With the pull request open, the wait is 2 minutes.
	w.srv.Clock.BlockUntil(1) // the second poll is over
	w.h.Answer(fakegh.Answer{Args: []string{"api", repoPath + "/pulls", "--method", "POST"}, Stdout: pr(7, "open", "")})
	w.srv.Clock.Advance(pullrequestwatcher.WaitingInterval)
	w.recorded(store.PROpen)
	codecentraltest.Eventually(t, func() bool { return w.lists() == 3 }, "no third poll: %d lists", w.lists())
	w.srv.Clock.BlockUntil(1)
	w.srv.Clock.Advance(pullrequestwatcher.Interval - time.Second)
	if w.lists() != 3 {
		t.Fatalf("polled again before 2 minutes")
	}
	w.srv.Clock.Advance(time.Second)
	codecentraltest.Eventually(t, func() bool { return w.lists() == 4 }, "no poll after 2 minutes")
}

func TestARateLimitedWatcherMakesNoCallsUntilTheResetThenResumesAndIsNeverAnError(t *testing.T) {
	w := newWatching(t)
	reset := w.srv.Clock.Now().Add(15 * time.Minute)
	w.h.Answer(fakegh.Answer{Args: []string{"api", listPath}, Exit: 1,
		Stdout: "HTTP/2.0 403 Forbidden\r\nX-Ratelimit-Remaining: 0\r\n" + fmt.Sprintf("X-Ratelimit-Reset: %d\r\n", reset.Unix()) + "\r\n" + `{"message":"API rate limit exceeded"}`,
		Stderr: "gh: API rate limit exceeded (HTTP 403)\n"})
	w.start(w.h.Bin)
	var got core.WatcherView
	codecentraltest.Eventually(t, func() bool {
		p, err := w.srv.Client.GetProject(ctx, "acme")
		if err != nil {
			t.Fatal(err)
		}
		got = p.Landing.Watcher
		return got.RateLimitedUntil.Equal(reset)
	}, "status %+v, want Watching until %s", &got, reset)
	if got.State != core.WatcherWatching {
		t.Fatalf("status %+v, want Watching", got)
	}
	calls := len(w.h.Calls())
	w.nextPoll(15*time.Minute - time.Second)
	if n := len(w.h.Calls()); n != calls {
		t.Fatalf("%d gh calls before the reset, want none after %d", n, calls)
	}
	w.answerList(pr(4, "open", ""))
	w.srv.Clock.Advance(time.Second)
	w.recorded(store.PROpen)
	if n := w.errorCount(); n != 0 {
		t.Fatalf("went into an error %d times, want none", n)
	}
}

func TestEachErrorIsReportedWithItsFixOnceAndClearsWhenFixed(t *testing.T) {
	for _, c := range []struct {
		name       string
		cause, fix func(h *ciwatchertest.Harness)
		problem    core.WatcherProblem
		fixText    string
	}{
		{"logged out", func(h *ciwatchertest.Harness) { h.LoggedOut() }, func(h *ciwatchertest.Harness) { h.Answer(fakegh.Answer{Args: []string{"auth", "status"}}) },
			core.ProblemLoggedOut, "gh auth login"},
		{"unreadable", func(h *ciwatchertest.Harness) { h.Unreadable("acme/web") }, func(h *ciwatchertest.Harness) {
			h.Answer(fakegh.Answer{Args: []string{"api", repoPath}, Stdout: write})
		},
			core.ProblemRepositoryUnreadable, "owner/name"},
		{"no write access", func(h *ciwatchertest.Harness) {
			h.Answer(fakegh.Answer{Args: []string{"api", repoPath}, Stdout: readOnly})
		},
			func(h *ciwatchertest.Harness) {
				h.Answer(fakegh.Answer{Args: []string{"api", repoPath}, Stdout: write})
			}, core.ProblemNoWrite, "write access"},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := newWatching(t)
			c.cause(w.h)
			w.answerList(pr(4, "open", ""))
			w.start(w.h.Bin)
			got := w.status(core.WatcherError)
			if got.Problem != c.problem || !strings.Contains(got.Fix, c.fixText) {
				t.Fatalf("status %+v, want %s with the fix %q", got, c.problem, c.fixText)
			}
			w.nextPoll(pullrequestwatcher.Interval)
			w.nextPoll(pullrequestwatcher.Interval)
			w.status(core.WatcherError)
			if n := w.errorCount(); n != 1 {
				t.Fatalf("went into an error %d times, want once, for one Notification", n)
			}
			if n := len(w.opened()); n != 0 {
				t.Fatalf("opened %d pull requests while in error", n)
			}
			c.fix(w.h)
			w.nextPoll(pullrequestwatcher.Interval)
			w.status(core.WatcherWatching)
			w.recorded(store.PROpen)
		})
	}
}

func TestAMissingGHIsAnErrorWithHowToInstallIt(t *testing.T) {
	w := newWatching(t)
	w.start(t.TempDir())
	got := w.status(core.WatcherError)
	if got.Problem != core.ProblemGHMissing || !strings.Contains(got.Fix, "https://cli.github.com") {
		t.Fatalf("status %+v, want gh missing with how to install it", got)
	}
}

func TestItExitsWhenItsStdinCloses(t *testing.T) {
	w := newWatching(t)
	w.answerList(pr(4, "open", ""))
	w.start(w.h.Bin)
	w.recorded(store.PROpen)
	w.stdin.Close()
	select {
	case err := <-w.done:
		if err != nil {
			t.Fatalf("Watch returned %v, want nil", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the Pull Request Watcher kept running after its stdin closed")
	}
}

func TestItDoesNothingWhileTheLandingMethodIsNotPullRequest(t *testing.T) {
	w := newWatching(t)
	must(t, func() error {
		_, err := w.srv.Core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingBranch, Push: true, Remote: "origin"})
		return err
	})
	w.start(w.h.Bin)
	w.srv.Clock.BlockUntil(1)
	if n := len(w.h.Calls()); n != 0 {
		t.Fatalf("%d gh calls under Branch, want none", n)
	}
}
