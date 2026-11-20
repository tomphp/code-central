package core

import (
	"context"

	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/store"
)

// The service among the others running on its Store (ADR 0068): its
// Service Registration, renewed on a timer, and the Watcher Leases its
// supervisor holds, one per Project, so each Project's Watchers run on
// one service. On a Store one service holds alone, it is the only one
// registered, and holds every Lease.

// Service is this service as the Store's record of the services names
// it.
type Service struct {
	// ID is its own, new each time it starts.
	ID string
	// Host is the machine it runs on, and Version its version.
	Host    string
	Version string
}

// SetService names this service, before it registers or holds a Lease.
func (c *Core) SetService(s Service) {
	c.service = s
	c.heartbeats.mu.Lock()
	c.heartbeats.self = s.ID
	c.heartbeats.mu.Unlock()
	c.watchers.mu.Lock()
	defer c.watchers.mu.Unlock()
	c.watchers.self = s.ID
}

// Register writes this service's Service Registration, or renews it, as
// at the Store's format this build writes. A *store.FormatChangedError is
// the Store migrated past that by another service: this one must stop.
func (c *Core) Register(context.Context) error {
	return c.store.Services().Register(store.Registration{ID: c.service.ID, Host: c.service.Host,
		Version: c.service.Version, Format: store.Format, Renewed: c.now()})
}

// Deregister deletes this service's Service Registration and lets go of
// its Watcher Leases, as it stops, so another service takes them at once.
func (c *Core) Deregister(context.Context) error {
	return c.store.Services().Deregister(c.service.ID)
}

// HoldWatcherLease renews the Project key's Watcher Lease for this
// service, or takes it when it is free, and reports whether this service
// holds it, so its supervisor runs key's Watchers. A change in who holds
// it publishes a notice. When the Store can't be reached the Lease is
// held, as last known, until it lapses, and the error is for the log.
func (c *Core) HoldWatcherLease(_ context.Context, key string) (bool, error) {
	now := c.now()
	l, err := c.store.Services().HoldLease(key, c.service.ID, now)
	if err != nil {
		l, ok := c.watchers.lease(key)
		return ok && l.Holder == c.service.ID && !l.Lapsed(now), err
	}
	if c.watchers.hold(l, now) {
		c.events.Publish(events.Event{Type: events.ProjectChanged, Project: key})
	}
	return l.Holder == c.service.ID, nil
}
