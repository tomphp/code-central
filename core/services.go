package core

import (
	"context"
	"io"

	"github.com/tomoram/code-central/core/foundation"
	"github.com/tomoram/code-central/events"
)

// The service among the others running on its Store (ADR 0068): its
// Service Registration, renewed on a timer, and the Watcher Leases its
// supervisor holds, one per Project, so each Project's Watchers run on
// one service. On a Store one service holds alone, it is the only one
// registered, and holds every Lease.

// Service is this service as the Store's record of the services names
// it.
type Service = foundation.Service

// SetService names this service, before it registers or holds a Lease.
func (c *Core) SetService(s Service) {
	c.foundation.SetService(s)
	c.watchers.mu.Lock()
	defer c.watchers.mu.Unlock()
	c.watchers.self = s.ID
}

// Register writes this service's Service Registration, or renews it, as
// at the Store's format this build writes. A *store.FormatChangedError is
// the Store migrated past that by another service: this one must stop.
func (c *Core) Register(ctx context.Context) error { return c.foundation.Register(ctx) }

// Deregister deletes this service's Service Registration and lets go of
// its Watcher Leases, as it stops, so another service takes them at once.
func (c *Core) Deregister(ctx context.Context) error { return c.foundation.Deregister(ctx) }

// SetLog makes c log what it can't write to or read from the Store, and
// carries on without, to w. Without one, nothing is logged.
func (c *Core) SetLog(w io.Writer) { c.foundation.SetLog(w) }

// HoldWatcherLease renews the Project key's Watcher Lease for this
// service, or takes it when it is free, and reports whether this service
// holds it, so its supervisor runs key's Watchers. A change in who holds
// it publishes a notice. When the Store can't be reached the Lease is
// held, as last known, until it lapses, and the error is for the log.
func (c *Core) HoldWatcherLease(_ context.Context, key string) (bool, error) {
	now := c.now()
	self := c.foundation.Service().ID
	l, err := c.store.Services().HoldLease(key, self, now)
	if err != nil {
		l, ok := c.watchers.lease(key)
		return ok && l.Holder == self && !l.Lapsed(now), err
	}
	if c.watchers.hold(l, now) {
		c.events.Publish(events.Event{Type: events.ProjectChanged, Project: key})
	}
	return l.Holder == self, nil
}
