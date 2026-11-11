// The error a CI system's rate limit is, for every Watcher's pace.

package watchers

import (
	"strings"
	"time"

	"github.com/tomoram/code-central/core"
)

// RateLimitError is a CI system refusing a call for its rate limit.
type RateLimitError struct {
	// System names the CI system, as its messages do: "GitHub".
	System string
	// Reset is when the system said the limit ends, and RetryAfter how
	// long it said to wait; both are zero when it said neither.
	Reset      time.Time
	RetryAfter time.Duration
	// Message is what the system's tool said.
	Message string
}

func (e *RateLimitError) Error() string {
	if e.System == "GitHub" {
		return e.System + " is rate-limiting gh: " + e.Message
	}
	return e.System + " is throttling Code Central: " + e.Message
}

// Until is when a Watcher may call the system again, from now; zero when
// it didn't say.
func (e *RateLimitError) Until(now time.Time) time.Time {
	if e.RetryAfter > 0 {
		return now.Add(e.RetryAfter)
	}
	return e.Reset
}

// CheckResult is a check failing because the system rate-limited it, at
// now: there is no knowing yet whether the Watcher can watch, so it is the
// check failed problem, with when to try again.
func (e *RateLimitError) CheckResult(now time.Time) core.WatcherCheckResult {
	r := core.WatcherCheckResult{Problem: core.ProblemCheckFailed, Reason: e.Error(), Fix: "Try again in a few minutes."}
	if until := e.Until(now); !until.IsZero() {
		r.Fix = "Try again after " + until.Local().Format(time.Kitchen) + ", when " + e.System + "'s rate limit ends."
	}
	return r
}

// FirstLine is s's first line with text on it, trimmed.
func FirstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}
