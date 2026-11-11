// Every Watcher's pace between polls, rate limits included.

package watchers

import (
	"time"
)

// MaxRateLimitInterval bounds a rate-limited Watcher's doubled interval,
// when GitHub gives no reset time.
const MaxRateLimitInterval = 10 * time.Minute

// Pace is how long a Watcher waits between polls. When GitHub rate-limits
// it, it makes no calls until the reset time GitHub gave, or with none,
// doubles its interval each poll up to MaxRateLimitInterval; once a poll
// isn't rate-limited, it goes back to its usual interval.
type Pace struct {
	// backoff is the doubled interval while rate-limited with no reset
	// time; zero once a poll isn't rate-limited.
	backoff time.Duration
}

// Next is how long to wait after a poll at now, whose usual interval is
// every, and that limit, if not nil, rate-limited; and until when GitHub
// rate-limits it, zero when it doesn't, for the Watcher to report with
// Watching.
func (p *Pace) Next(now time.Time, every time.Duration, limit *RateLimitError) (wait time.Duration, until time.Time) {
	if limit == nil {
		p.backoff = 0
		return every, time.Time{}
	}
	if until = limit.Until(now); !until.After(now) {
		p.backoff = min(max(p.backoff, every)*2, MaxRateLimitInterval)
		until = now.Add(p.backoff)
	}
	return until.Sub(now), until
}
