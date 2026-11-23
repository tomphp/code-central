package factories

import (
	"context"
	"sync"
	"time"

	"github.com/tomoram/code-central/store"
)

// PingThrottle is the shortest gap between two counted pings from one
// session.
const PingThrottle = time.Minute

// pingExpiry is how old a ping row is before every service, and the Store,
// forget it. A Claim quiet for longer shows its last stored write as its
// last activity.
const pingExpiry = 7 * 24 * time.Hour

// pings holds each session's last counted ping: here, and on every
// service as of the last tick.
type pings struct {
	mu     sync.Mutex
	mine   map[string]time.Time // session ID -> last counted ping here: what the throttle goes by
	shared map[string]time.Time // session ID -> last ping any service counted, as the Store had it at the last tick
}

// Ping is POST /api/ping {session_id}: it counts activity for the session.
// Pings within PingThrottle of the session's last counted one here are
// ignored, as is an empty session ID. A counted ping is written through to
// the Store, so about one row a minute per session; a Store that can't be
// reached is logged and never fails the ping.
func (c *Factories) Ping(_ context.Context, sessionID string) {
	if sessionID == "" {
		return
	}
	now := c.f.Now()
	p := c.pings
	p.mu.Lock()
	if last, ok := p.mine[sessionID]; ok && now.Sub(last) < PingThrottle {
		p.mu.Unlock()
		return
	}
	p.mine[sessionID] = now
	p.mu.Unlock()
	svc := c.f.Service()
	if err := c.f.Store.Services().PutPing(store.Ping{Session: sessionID, At: now, Service: svc.ID, ServiceHost: svc.Host}); err != nil {
		c.f.Logf("cannot record a ping from session %s: %v", sessionID, err)
	}
}

// RefreshPings prunes the pings older than pingExpiry from the Store and
// reads the rest, so a ping any service counted is seen here. What it
// can't read is logged, and memory stands.
func (c *Factories) RefreshPings() {
	services := c.f.Store.Services()
	if err := services.PrunePings(c.f.Now().Add(-pingExpiry)); err != nil {
		c.f.Logf("cannot prune the pings: %v", err)
	}
	rows, err := services.Pings()
	if err != nil {
		c.f.Logf("cannot read the pings: %v", err)
		return
	}
	shared := make(map[string]time.Time, len(rows))
	for _, p := range rows {
		shared[p.Session] = p.At
	}
	c.pings.mu.Lock()
	defer c.pings.mu.Unlock()
	c.pings.shared = shared
}

// LastPing is the session's last counted ping, on this service or, as of
// the last RefreshPings, any other sharing the Store; zero when there is
// none.
func (c *Factories) LastPing(sessionID string) time.Time {
	c.pings.mu.Lock()
	defer c.pings.mu.Unlock()
	return laterOf(c.pings.mine[sessionID], c.pings.shared[sessionID])
}

func laterOf(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}
