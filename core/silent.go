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

// PingThrottle is the shortest gap between two counted pings from one
// session.
const PingThrottle = time.Minute

// activity is the in-memory side of last activity: when the service
// started, each session's last counted ping, and which Claims the
// background watch last saw Silent.
type activity struct {
	started time.Time

	mu     sync.Mutex
	pings  map[string]time.Time // session ID -> last counted ping
	silent map[claimKey]bool
}

// claimKey names one Claim across Projects.
type claimKey struct {
	project string
	ticket  int
	claim   int
}

func newActivity(started time.Time) *activity {
	return &activity{started: started, pings: map[string]time.Time{}, silent: map[claimKey]bool{}}
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

// WatchSilent is the background ticker behind Silent Claims. On each tick
// it reads every Project and calls changed(project, ticket ID) once for
// each Claim that has turned Silent since the last tick, and once for each
// that has stopped being Silent without a write (a ping), so live views
// refetch. The time comes from the injected clock, not the tick; serve
// ticks every minute. It returns when ctx ends.
func (c *Core) WatchSilent(ctx context.Context, ticks <-chan time.Time, changed func(project, ticketID string)) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			for _, k := range c.silentCrossings() {
				changed(k.project, DisplayID(k.project, k.ticket))
			}
		}
	}
}

// silentCrossings returns the Claims whose Silent mark changed since the
// last call, and remembers the new marks.
func (c *Core) silentCrossings() []claimKey {
	now := map[claimKey]bool{}
	c.store.View(func(r store.Reader) error {
		for _, p := range r.Projects() {
			for _, t := range r.Tickets(p.Key) {
				if cl, ok := liveClaim(t); ok && c.silentSince(t) != nil {
					now[claimKey{p.Key, t.Number, cl.Number}] = true
				}
			}
		}
		return nil
	})
	a := c.activity
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []claimKey
	for k := range now {
		if !a.silent[k] {
			out = append(out, k)
		}
	}
	for k := range a.silent {
		if !now[k] {
			out = append(out, k)
		}
	}
	a.silent = now
	return out
}
