package foundation

import (
	"context"

	"github.com/tomoram/code-central/store"
)

// The service among the others running on its Store (ADR 0068): its
// Service Registration, renewed on a timer.

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
func (f *Foundation) SetService(s Service) { f.service = s }

// Service is this service as named by SetService.
func (f *Foundation) Service() Service { return f.service }

// Register writes this service's Service Registration, or renews it, as
// at the Store's format this build writes. A *store.FormatChangedError is
// the Store migrated past that by another service: this one must stop.
func (f *Foundation) Register(context.Context) error {
	return f.Store.Services().Register(store.Registration{ID: f.service.ID, Host: f.service.Host,
		Version: f.service.Version, Format: store.Format, Renewed: f.Now()})
}

// Deregister deletes this service's Service Registration, as it stops, so
// another service takes its Watcher Leases at once.
func (f *Foundation) Deregister(context.Context) error {
	return f.Store.Services().Deregister(f.service.ID)
}
