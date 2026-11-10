package factory

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
)

// outageErrors are the API errors that mean the trouble is the service's
// or the account's limits, not the Ticket's: usage and rate limits,
// overload, server and network errors. A turn cut off by one is resumed
// later, never failed.
//
// Source: docs/research/agent-sdk-host.md (ticket 50, on branch
// research/agent-sdk-host), section 5, names the stream-json
// `system/api_retry` event's `error` values `rate_limit`,
// `authentication_failed` and `billing_error` (the usage limit), from the
// headless docs. The other codes here (overload, server, network and
// timeout, and the `_error` spellings) are unverified: neither that note
// nor docs/research/claude-code-session-host.md lists them, nor the
// result's `terminal_reason` values.
var outageErrors = map[string]bool{
	"rate_limit":       true,
	"rate_limit_error": true,
	"billing_error":    true,
	"overloaded":       true,
	"overloaded_error": true,
	"server_error":     true,
	"api_error":        true,
	"network_error":    true,
	"connection_error": true,
	"timeout":          true,
}

// authFailed is the API error of an expired or missing login, which only
// the human can fix.
const authFailed = "authentication_failed"

// apiError is the API error the turn ended on, from its messages or its
// result's terminal reason.
func (t turnEnd) apiError() string {
	if t.APIError != "" {
		return t.APIError
	}
	if t.Result != nil && (outageErrors[t.Result.TerminalReason] || t.Result.TerminalReason == authFailed) {
		return t.Result.TerminalReason
	}
	return ""
}

// maxBackoff caps the wait between resumes of cut-off turns.
const maxBackoff = 30 * time.Minute

// pause is the Factory's state during an outage: no claiming, resuming
// Answered Claims or landing; only the cut-off turns are resumed, once
// the backoff has passed. It lives in memory only.
type pause struct {
	reason  string    // what the Factory is waiting on
	since   time.Time // when it began; a turn started after it ending well ends it
	backoff time.Duration
	until   time.Time // when cut-off turns may next be resumed
	cutOff  []claim   // oldest first
}

func (f *factory) firstBackoff() time.Duration {
	if f.e.Backoff > 0 {
		return f.e.Backoff
	}
	return time.Minute
}

// cutOff records that an outage cut off c's turn: the Factory pauses, or
// stays paused with its backoff doubled.
func (f *factory) cutOff(c claim, apiErr string) {
	f.pauseMu.Lock()
	defer f.pauseMu.Unlock()
	now := f.e.Now()
	p := f.pause
	if p == nil {
		p = &pause{since: now}
		f.pause = p
	}
	if p.backoff == 0 {
		p.backoff = f.firstBackoff()
	} else {
		p.backoff = min(2*p.backoff, maxBackoff)
	}
	p.until = now.Add(p.backoff)
	p.reason = fmt.Sprintf("%s (it cut off %s's turn)", apiErr, c.Ticket)
	if !slices.ContainsFunc(p.cutOff, func(h claim) bool { return h.ClaimID == c.ClaimID }) {
		p.cutOff = append(p.cutOff, c)
	}
	f.printf("paused: waiting on %s: no claiming, resuming or landing; resuming %d cut-off turn(s) in %s", p.reason, len(p.cutOff), p.backoff)
}

// turnWentWell ends the pause when a turn started during it ends with no
// outage: the service is back. Turns still cut off are resumed at the
// next poll.
func (f *factory) turnWentWell(t *turn) {
	f.pauseMu.Lock()
	defer f.pauseMu.Unlock()
	p := f.pause
	if p == nil || t.started.Before(p.since) {
		return
	}
	if len(p.cutOff) == 0 {
		f.pause = nil
		f.printf("the outage is over: carrying on")
		return
	}
	p.backoff, p.until = 0, f.e.Now()
}

// paused reports whether the Factory is paused for an outage.
func (f *factory) paused() bool {
	f.pauseMu.Lock()
	defer f.pauseMu.Unlock()
	return f.pause != nil
}

// duePausedResumes says what the paused Factory is waiting on and takes
// the cut-off Claims due a resume, up to free slots.
func (f *factory) duePausedResumes(free int) []claim {
	f.pauseMu.Lock()
	defer f.pauseMu.Unlock()
	p := f.pause
	if f.e.Now().Before(p.until) || free <= 0 {
		var names []string
		for _, c := range p.cutOff {
			names = append(names, c.Ticket)
		}
		f.printf("paused: waiting on %s; cut off: %s; next resume at %s", p.reason, strings.Join(names, ", "), p.until.Format(time.TimeOnly))
		return nil
	}
	n := min(free, len(p.cutOff))
	due := p.cutOff[:n:n]
	p.cutOff = p.cutOff[n:]
	return due
}

// requeueCutOff puts back a cut-off Claim that couldn't be resumed yet.
func (f *factory) requeueCutOff(c claim) {
	f.pauseMu.Lock()
	defer f.pauseMu.Unlock()
	if f.pause != nil {
		f.pause.cutOff = append([]claim{c}, f.pause.cutOff...)
	}
}

// dropCutOff forgets a cut-off Claim that has ended.
func (f *factory) dropCutOff(claimID string) {
	f.pauseMu.Lock()
	defer f.pauseMu.Unlock()
	if f.pause != nil {
		f.pause.cutOff = slices.DeleteFunc(f.pause.cutOff, func(c claim) bool { return c.ClaimID == claimID })
	}
}

// resumeCutOff resumes c's session, which an outage or a stop cut off,
// unless another process holds it or its Model Choice can't be read. It
// reports whether it resumed.
func (f *factory) resumeCutOff(ctx context.Context, c claim) bool {
	if !f.live(c.ClaimID) {
		return false
	}
	if pid, ok := f.sessionHolder(c.SessionID); ok {
		f.claimf(c, "%s: session %s is held by pid %d: waiting a poll before resuming", c.Ticket, c.SessionID, pid)
		return false
	}
	tm, ok := f.resumeModel(ctx, c)
	if !ok {
		return false
	}
	f.claimf(c, "%s resumed %s after being cut off", c.Ticket, f.about(c))
	f.startTurn(ctx, c, f.worktree(c.Ticket), tm, append(f.resumeArgs(c, tm), cutOffPrompt(c)))
	return true
}

func cutOffPrompt(c claim) string {
	return fmt.Sprintf(`Your last turn was cut off; carry on.

Ticket: %s
Claim: %s
`, c.Ticket, c.ClaimID)
}
