package core

import (
	"context"
	"sync"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core/factories"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// Silent Claims. A Ticket In Progress with no activity for SilentAfter
// is a Silent Claim. Activity is any write to the Ticket (its updated
// time) or a ping from the Claim's session. Pings are written through to
// the Store's Services side, so every service sharing the Store sees them
// (once its Factory Status tick has read them), and read from memory.

// SilentAfter is how long an In Progress Ticket may be quiet before its
// Claim is Silent.
const SilentAfter = 30 * time.Minute

// LongAfter is how long a Claim may go without a move (claim or
// pick_up_answers) before it is Long, if it isn't Silent.
const LongAfter = 60 * time.Minute

// NotLandedAfter is how long a Ticket may stay Accepted before it is
// Accepted but not landed (ticket 72).
const NotLandedAfter = 10 * time.Minute

// PingThrottle is the shortest gap between two counted pings from one
// session.
const PingThrottle = factories.PingThrottle

// activity is the in-memory side of last activity: when the service
// started, and the timed reason the background watch last saw on each
// Claim and Accepted Ticket.
type activity struct {
	started time.Time

	mu    sync.Mutex
	marks map[markKey]api.NeedsMeReason // Silent, Long, CI unavailable or Accepted but not landed
}

// markKey names what a timed reason is on: one Claim, or, with claim 0, an
// Accepted Ticket, which has no live Claim.
type markKey struct {
	project string
	ticket  int
	claim   int
}

func newActivity(started time.Time) *activity {
	return &activity{started: started, marks: map[markKey]api.NeedsMeReason{}}
}

// claimActivity is a Claim's last activity as reads show it: the latest
// stored write its agent made (lastActivity, ticket 30), or, while the
// Claim is live, its session's last counted ping if that is later.
func (c *Core) claimActivity(t domain.Ticket, cl domain.Claim) time.Time {
	last := lastActivity(t, cl)
	if ping := c.lastPing(cl); ping.After(last) {
		last = ping
	}
	return last
}

// lastPing is a live Claim's session's last counted ping, on this service
// or, as of the last tick, any other sharing the Store; zero when there is
// none or the Claim has ended.
func (c *Core) lastPing(cl domain.Claim) time.Time {
	if !cl.Live() || cl.SessionID == "" {
		return time.Time{}
	}
	return c.factories.LastPing(cl.SessionID)
}

// quietSince is when the Ticket last saw any activity on its live Claim:
// the latest of service start, any write to the Ticket by anyone, and the
// Claim's own activity. Every write that stores the Claim's activity also
// sets Updated, so it reads only the Ticket's head and the Claim's pings,
// as the evaluation does.
//
// Service start is a floor on every service, even on a Shared Store where
// pings outlive a restart: a service started in the last 30 minutes can
// lag the others in flagging a Claim as Silent, but never flags one
// falsely. This is the one exception to "Silent on every service or on
// none".
func (c *Core) quietSince(t domain.Ticket, cl domain.Claim) time.Time {
	last := c.activity.started
	for _, at := range []time.Time{t.Updated, cl.Started, c.lastPing(cl), latestResume(cl)} {
		if at.After(last) {
			last = at
		}
	}
	return last
}

// silentSince is the Ticket's last activity when it is a Silent Claim,
// else nil. Needs Answer and Answered are never Silent: the session is
// waiting on the human there, not working.
func (c *Core) silentSince(t domain.Ticket) *time.Time {
	if t.State != domain.InProgress {
		return nil
	}
	cl, ok := liveClaim(t)
	if !ok || cl.Stopped() {
		return nil
	}
	last := c.quietSince(t, *cl)
	if c.now().Sub(last) < SilentAfter {
		return nil
	}
	return &last
}

// longSince is the Claim's last move when it has run long, else nil: In
// Progress with a live Claim, and more than LongAfter since it last moved
// into In Progress. Whether that makes it a Long Claim is the
// evaluation's call: Silent takes precedence.
func (c *Core) longSince(t domain.Ticket) *time.Time {
	if t.State != domain.InProgress {
		return nil
	}
	cl, ok := liveClaim(t)
	if !ok || cl.Stopped() {
		return nil
	}
	moved := entered(t)
	if resumed := latestResume(*cl); resumed.After(moved) {
		moved = resumed
	}
	if c.now().Sub(moved) <= LongAfter {
		return nil
	}
	return &moved
}

// notLandedSince is when t moved to Accepted, when it has been Accepted
// for more than NotLandedAfter in Project p under the Merge Landing
// Method in any of its Changed Repositories, else nil. It needs no Claim
// or Factory: only the time since the accept counts. Under Branch,
// Accepted work is To merge at once instead.
func (c *Core) notLandedSince(p domain.Project, t domain.Ticket) *time.Time {
	if t.State != domain.Accepted || !ticketLandingPolicies(p, t).Any(domain.LandingPolicy.FlagsNotLanded) {
		return nil
	}
	accepted := entered(t)
	if c.now().Sub(accepted) <= NotLandedAfter {
		return nil
	}
	return &accepted
}

// WatchTimedReasons is the background ticker behind the timed reasons:
// Silent and Long Claims, CI unavailable, and Accepted but not landed. On each tick it
// reads every Project and calls changed(project, ticket ID) once for each
// Claim (or Accepted Ticket) whose timed reason (or none) has changed
// since the last tick without a write (time passing, or a ping), so live
// views refetch. The time comes from the injected clock, not the tick; serve
// ticks every minute. It returns when ctx ends.
func (c *Core) WatchTimedReasons(ctx context.Context, ticks <-chan time.Time, changed func(project, ticketID string)) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			for _, k := range c.timedReasonChanges() {
				changed(k.project, domain.DisplayID(k.project, k.ticket))
			}
		}
	}
}

// timedReasonChanges returns the Claims and Accepted Tickets whose timed
// reason changed since the last call, and remembers the new ones. A
// Ticket's timed reason is its reason in its Project's evaluation when
// that is a timed one, so a Ticket waiting for an earlier reason, such
// as a CI item's, has none.
func (c *Core) timedReasonChanges() []markKey {
	now := map[markKey]api.NeedsMeReason{}
	c.store.View(func(r store.Reader) error {
		for _, p := range r.Projects() {
			e := c.evaluate(r, p)
			for _, t := range e.tickets() {
				reason, _ := e.reason(t)
				if !isTimed(reason) {
					continue
				}
				k := markKey{p.Key, t.Number, 0}
				if cl, ok := liveClaim(t); ok && (reason == api.ReasonSilentClaim || reason == api.ReasonLongClaim) {
					k.claim = cl.Number
				}
				now[k] = reason
			}
		}
		return nil
	})
	a := c.activity
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []markKey
	for k, m := range now {
		if a.marks[k] != m {
			out = append(out, k)
		}
	}
	for k := range a.marks {
		if _, ok := now[k]; !ok {
			out = append(out, k)
		}
	}
	a.marks = now
	return out
}
