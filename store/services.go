package store

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
)

// The services running on a Store (ADR 0068): each one's Service
// Registration, and which of them holds each Project's Watcher Lease.
// Only a Shared Store has more than one; on the others the one service is
// the only one registered and holds every Lease. Times are the caller's,
// so a test can run them on a clock of its own.

// How long a Service Registration and a Watcher Lease last: each is
// renewed every ServiceRenewal, and lives for ServiceLife from its last
// renewal, so a service missing two renewals in a row keeps it.
const (
	ServiceRenewal = 10 * time.Second
	ServiceLife    = 30 * time.Second
)

// Registration is a running service's Service Registration.
type Registration struct {
	// ID is the service's own, new each time it starts.
	ID string
	// Host is the machine it runs on, and Version its version.
	Host    string
	Version string
	// Format is the store format it reads and writes.
	Format int
	// Renewed is when it last renewed the Registration.
	Renewed time.Time
}

// live is whether r is still live at now.
func (r Registration) live(now time.Time) bool { return now.Before(r.Renewed.Add(ServiceLife)) }

// Lease is a Project's Watcher Lease: the service that holds it, and so
// runs the Project's Watchers, and when it lapses unless renewed.
type Lease struct {
	Project string
	// Holder is the holding service's ID, and Host the machine its
	// Registration names: empty when it has none.
	Holder string
	Host   string
	// Expires is when another service may take the Lease.
	Expires time.Time
}

// Lapsed is whether l has expired at now, so another service may take it.
func (l Lease) Lapsed(now time.Time) bool { return !now.Before(l.Expires) }

// byHost orders Registrations by host, then ID: the engines' collations
// would order hosts apart, so every Kind sorts them itself.
func byHost(a, b Registration) int {
	return cmp.Or(cmp.Compare(a.Host, b.Host), cmp.Compare(a.ID, b.ID))
}

// Services is a Store's record of the services running on it.
type Services interface {
	// Register writes r as its service's Registration, or renews it,
	// and checks the Store's format is still r.Format: one another
	// service has migrated is a *FormatChangedError, and the service
	// must stop writing.
	Register(r Registration) error
	// Deregister deletes the service id's Registration and lets go of
	// every Watcher Lease it holds, so another service can take them at
	// once.
	Deregister(id string) error
	// Registrations is every Registration live at now, in host, then ID,
	// order.
	Registrations(now time.Time) ([]Registration, error)
	// HoldLease gives the service id the project's Watcher Lease until
	// ServiceLife from now, renewing it, or taking it when no other
	// service holds it unexpired. It returns the Lease as it then is:
	// id's, or the other holder's.
	HoldLease(project, id string, now time.Time) (Lease, error)
}

// registered names each service in regs by its host and version.
func registered(regs []Registration) string {
	names := make([]string, 0, len(regs))
	for _, r := range regs {
		names = append(names, fmt.Sprintf("%s (%s)", r.Host, r.Version))
	}
	return strings.Join(names, ", ")
}

// FormatChangedError is a Store whose format another service changed
// under a running one, migrating it: the running service must stop
// writing it.
type FormatChangedError struct {
	// Where names the Store.
	Where string
	// Stored is the Store's format now, and Own the running service's.
	Stored, Own int
}

func (e *FormatChangedError) Error() string {
	return fmt.Sprintf("%s: another service migrated the store to format %d, and this code-central writes format %d; it stops writing it: upgrade it and start it again",
		e.Where, e.Stored, e.Own)
}

// memoryServices are the Services of a Store one service holds alone,
// kept in memory: the File Store's and the fake's.
type memoryServices struct {
	mu sync.Mutex
	// format is the Store's format, which no other service can change.
	format        int
	registrations map[string]Registration
	leases        map[string]Lease
}

func newMemoryServices(format int) *memoryServices {
	return &memoryServices{format: format, registrations: map[string]Registration{}, leases: map[string]Lease{}}
}

func (m *memoryServices) Register(r Registration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r.Format != m.format {
		return &FormatChangedError{Where: "the store", Stored: m.format, Own: r.Format}
	}
	m.registrations[r.ID] = r
	return nil
}

func (m *memoryServices) Deregister(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.registrations, id)
	for project, l := range m.leases {
		if l.Holder == id {
			delete(m.leases, project)
		}
	}
	return nil
}

func (m *memoryServices) Registrations(now time.Time) ([]Registration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Registration{}
	for _, r := range m.registrations {
		if r.live(now) {
			out = append(out, r)
		}
	}
	slices.SortFunc(out, byHost)
	return out, nil
}

func (m *memoryServices) HoldLease(project, id string, now time.Time) (Lease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.leases[project]
	if !ok || l.Holder == id || l.Lapsed(now) {
		l = Lease{Project: project, Holder: id, Expires: now.Add(ServiceLife)}
		m.leases[project] = l
	}
	l.Host = m.registrations[l.Holder].Host
	return l, nil
}
