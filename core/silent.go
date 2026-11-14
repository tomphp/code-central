package core

import (
	"context"
	"sync"
	"time"

	"github.com/tomoram/code-central/store"
)

// Silent Claims. A Ticket In Progress with no activity for SilentAfter
// is a Silent Claim. Activity is any write to the Ticket (its updated
// time) or a ping from the Claim's session. Pings live in memory only, so
// after a restart last activity counts from service start.

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
const PingThrottle = time.Minute

// activity is the in-memory side of last activity: when the service
// started, each session's last counted ping, and the timed reason the
// background watch last saw on each Claim and Accepted Ticket.
type activity struct {
	started time.Time

	mu    sync.Mutex
	pings map[string]time.Time      // session ID -> last counted ping
	marks map[markKey]NeedsMeReason // Silent, Long, CI unavailable or Accepted but not landed
}

// markKey names what a timed reason, a Transcript or a latest event
// notice is on, across Projects: one Claim, or, with claim 0, a whole
// Ticket: an Accepted one, which has no live Claim, for a timed reason,
// and any Ticket for the latest event's throttle.
type markKey struct {
	project string
	ticket  int
	claim   int
}

func newActivity(started time.Time) *activity {
	return &activity{started: started, pings: map[string]time.Time{}, marks: map[markKey]NeedsMeReason{}}
}

// Ping is POST /api/ping {session_id}: it marks activity on every live
// Claim with that session ID, in any Project. Pings within PingThrottle
// of the session's last counted one are ignored, as is an empty or
// unknown session ID. It never fails and writes nothing to the store.
func (c *Core) Ping(_ context.Context, sessionID string) {
	if sessionID == "" {
		return
	}
	now := c.now()
	a := c.activity
	a.mu.Lock()
	defer a.mu.Unlock()
	if last, ok := a.pings[sessionID]; ok && now.Sub(last) < PingThrottle {
		return
	}
	a.pings[sessionID] = now
}

// claimActivity is a Claim's last activity as reads show it: the latest
// stored write its agent made (lastActivity, ticket 30), or, while the
// Claim is live, its session's last counted ping if that is later.
func (c *Core) claimActivity(t store.Ticket, cl store.Claim) time.Time {
	last := lastActivity(t, cl)
	if cl.Live() && cl.SessionID != "" {
		c.activity.mu.Lock()
		ping := c.activity.pings[cl.SessionID]
		c.activity.mu.Unlock()
		if ping.After(last) {
			last = ping
		}
	}
	return last
}

// quietSince is when the Ticket last saw any activity on its live Claim:
// the latest of service start (pings are not kept across a restart), any
// write to the Ticket by anyone, and the Claim's own activity.
func (c *Core) quietSince(t store.Ticket, cl store.Claim) time.Time {
	last := c.activity.started
	for _, at := range []time.Time{t.Updated, c.claimActivity(t, cl)} {
		if at.After(last) {
			last = at
		}
	}
	return last
}

// silentSince is the Ticket's last activity when it is a Silent Claim,
// else nil. Needs Answer and Answered are never Silent: the session is
// waiting on the human there, not working.
func (c *Core) silentSince(t store.Ticket) *time.Time {
	if t.State != store.InProgress {
		return nil
	}
	cl, ok := liveClaim(t)
	if !ok {
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
func (c *Core) longSince(t store.Ticket) *time.Time {
	if t.State != store.InProgress {
		return nil
	}
	if _, ok := liveClaim(t); !ok {
		return nil
	}
	moved := entered(t)
	if c.now().Sub(moved) <= LongAfter {
		return nil
	}
	return &moved
}

// notLandedSince is when t moved to Accepted, when it has been Accepted
// for more than NotLandedAfter in Project p under the Merge Landing
// Method, else nil. It needs no Claim or Factory: only the time since the
// accept counts. Under Branch, Accepted work is To merge at once instead.
func (c *Core) notLandedSince(p store.Project, t store.Ticket) *time.Time {
	if t.State != store.Accepted || p.Landing.Method != store.LandingMerge {
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
				changed(k.project, DisplayID(k.project, k.ticket))
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
	now := map[markKey]NeedsMeReason{}
	c.store.View(func(r store.Reader) error {
		for _, p := range r.Projects() {
			e := c.evaluate(r, p)
			for _, t := range e.tickets() {
				reason, _ := e.reason(t)
				if !reason.timed() {
					continue
				}
				k := markKey{p.Key, t.Number, 0}
				if cl, ok := liveClaim(t); ok && (reason == ReasonSilentClaim || reason == ReasonLongClaim) {
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
