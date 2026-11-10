// The CI Watcher backing off when GitHub rate-limits it
// (code-central-260): it waits until the reset time GitHub gives, or
// doubles its interval up to 10 minutes, reporting Watching with when it
// may call again; it is never an error, so no Notification is sent.

package ciwatcher_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/tomoram/code-central/ciwatcher"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/internal/fakegh"
	"github.com/tomoram/code-central/store"
	"github.com/tomoram/code-central/watchers"
)

// rateLimited scripts GitHub's rate-limit answer to calls starting with
// args, with headers such as X-Ratelimit-Reset, as gh api --include
// prints them.
func (w *watching) rateLimited(args []string, headers ...string) {
	w.t.Helper()
	head := "HTTP/2.0 403 Forbidden\r\nContent-Type: application/json; charset=utf-8\r\n"
	for _, h := range headers {
		head += h + "\r\n"
	}
	w.h.Answer(fakegh.Answer{Args: args, Exit: 1,
		Stdout: head + "\r\n" + `{"message":"API rate limit exceeded for user ID 1.","documentation_url":"https://docs.github.com/rest/overview/rate-limits-for-the-rest-api","status":"403"}`,
		Stderr: "gh: API rate limit exceeded for user ID 1. (HTTP 403)\n"})
}

// runsOf is the gh call that lists commit's runs.
func runsOf(commit string) []string {
	return []string{"api", "repos/acme/web/actions/runs?head_sha=" + commit + "&per_page=100"}
}

// rateLimitedUntil waits for the CI Watcher to report Watching,
// rate-limited until until.
func (w *watching) rateLimitedUntil(until time.Time) {
	w.t.Helper()
	var got core.WatcherView
	w.waitFor(fmt.Sprintf("rate-limited until %s", until), func() bool {
		p, err := w.srv.Client.GetProject(context.Background(), "acme")
		if err != nil {
			w.t.Fatal(err)
		}
		got = p.CI.Watcher
		return got.RateLimitedUntil.Equal(until)
	})
	if got.State != core.WatcherWatching || got.Message != "" {
		w.t.Fatalf("status %+v, want Watching, rate-limited", got)
	}
}

// noErrors checks the CI Watcher never went into an error, so notify
// sends no Notification.
func (w *watching) noErrors() {
	w.t.Helper()
	for _, a := range w.srv.Core.WatcherAttention(context.Background()) {
		if a.Errors != 0 {
			w.t.Fatalf("CI Watcher attention %+v, want no errors", a)
		}
	}
}

func TestARateLimitedCIWatcherMakesNoGitHubCallsUntilTheResetTimeThenResumes(t *testing.T) {
	w := newWatching(t)
	id := w.pushed("abc123")
	reset := w.srv.Clock.Now().Add(15 * time.Minute)
	w.rateLimited(runsOf("abc123"), "X-Ratelimit-Limit: 5000", "X-Ratelimit-Remaining: 0", fmt.Sprintf("X-Ratelimit-Reset: %d", reset.Unix()))
	w.start(w.h.Bin)
	w.rateLimitedUntil(reset)
	w.noErrors()

	calls := len(w.h.Calls())
	w.srv.Clock.BlockUntil(1)
	w.srv.Clock.Advance(15*time.Minute - time.Second)
	if n := w.srv.Clock.Waiters(); n != 1 {
		t.Fatalf("%d timers waiting a second before the reset, want the CI Watcher's still", n)
	}
	if got := len(w.h.Calls()); got != calls {
		t.Fatalf("%d gh calls before the reset, want none after the rate limit's %d", got, calls)
	}

	// At the reset it polls again as usual, and is no longer rate-limited.
	w.runsAnswer("abc123", listedRun{id: 11, attempt: 1, workflow: "ci.yml", status: "completed", conclude: "success", updated: "2026-01-05T09:05:00Z"})
	w.jobsAnswer(11, 1, [2]string{"test", "success"})
	w.srv.Clock.Advance(time.Second)
	w.ciResult(id, core.CIPassed)
	w.rateLimitedUntil(time.Time{})
	w.noErrors()
}

// GitHub's secondary rate limits may say how long to wait in
// Retry-After.
func TestARateLimitedCIWatcherWaitsAsLongAsRetryAfterSays(t *testing.T) {
	w := newWatching(t)
	w.pushed("abc123")
	w.rateLimited(runsOf("abc123"), "Retry-After: 90")
	start := w.srv.Clock.Now()
	w.start(w.h.Bin)
	w.rateLimitedUntil(start.Add(90 * time.Second))
}

// With no reset time, it doubles its interval each time, up to 10
// minutes, and goes back to its own once GitHub answers.
func TestWithoutAResetTimeTheCIWatchersIntervalDoublesUpTo10Minutes(t *testing.T) {
	w := newWatching(t)
	w.pushed("abc123")
	w.rateLimited(runsOf("abc123"))
	w.start(w.h.Bin)
	waits := []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, watchers.MaxRateLimitInterval, watchers.MaxRateLimitInterval}
	for i, wait := range waits {
		w.rateLimitedUntil(w.srv.Clock.Now().Add(wait))
		if i == len(waits)-1 {
			w.runsAnswer("abc123")
		}
		w.srv.Clock.BlockUntil(1)
		w.srv.Clock.Advance(wait)
	}
	w.rateLimitedUntil(time.Time{})
	w.waitingPoll()
	w.noErrors()
}

// Reading the repository, as each poll does first, may be rate-limited
// too: that is still Watching, not the repository unreadable.
func TestARateLimitOnReadingTheRepositoryIsntAnError(t *testing.T) {
	w := newWatching(t)
	reset := w.srv.Clock.Now().Add(5 * time.Minute)
	w.rateLimited([]string{"api", "repos/acme/web"}, "X-Ratelimit-Remaining: 0", fmt.Sprintf("X-Ratelimit-Reset: %d", reset.Unix()))
	w.start(w.h.Bin)
	w.rateLimitedUntil(reset)
	w.noErrors()
}

// gh auth status calls GitHub too: a rate limit there isn't a logout.
func TestARateLimitOnCheckingTheLoginIsntAnError(t *testing.T) {
	w := newWatching(t)
	w.h.Answer(fakegh.Answer{Args: []string{"auth", "status"}, Exit: 1,
		Stderr: "github.com\n  X Failed to log in to github.com account octocat (keyring)\n  - API rate limit exceeded for user ID 1.\n"})
	start := w.srv.Clock.Now()
	w.start(w.h.Bin)
	w.rateLimitedUntil(start.Add(2 * ciwatcher.Interval))
	w.noErrors()
}

// Listing a CI Rule's branch may be rate-limited too: the CI Watcher
// backs off as for a pushed commit's runs, and fires the rule once
// GitHub answers.
func TestARateLimitOnListingARulesBranchBacksOff(t *testing.T) {
	w := newWatching(t)
	spec := w.rule("ci.yml", "main", store.Ready)
	reset := w.srv.Clock.Now().Add(5 * time.Minute)
	branch := []string{"api", "repos/acme/web/actions/runs?branch=main&status=completed&per_page=100"}
	w.rateLimited(branch, "X-Ratelimit-Remaining: 0", fmt.Sprintf("X-Ratelimit-Reset: %d", reset.Unix()))
	w.start(w.h.Bin)
	w.rateLimitedUntil(reset)
	w.noErrors()

	w.branchAnswer("main", branchRun{id: 21, workflow: "ci.yml", branch: "main", commit: "fff111", conclude: "failure", finished: 5})
	w.jobsAnswer(21, 1, [2]string{"test", "failure"})
	w.srv.Clock.BlockUntil(1)
	w.srv.Clock.Advance(5 * time.Minute)
	w.rateLimitedUntil(time.Time{})
	w.waitFor("a fix Ticket", func() bool { return len(w.fixTickets(spec)) == 1 })
	w.noErrors()
}
