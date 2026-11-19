package store

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"
)

// The services running on a Store (ADR 0068): each one's Service
// Registration, which of them holds each Project's Watcher Lease, and the
// runtime state they share (ADR 0072). Only a Shared Store has more than
// one; on the others the one service is the only one registered and holds
// every Lease. Times are the caller's, so a test can run them on a clock
// of its own.

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

	// PutHeartbeat writes h, replacing the Heartbeat of the same
	// Factory: the newest wins, whichever service it reached.
	PutHeartbeat(h Heartbeat) error
	// Heartbeats is every Heartbeat, in Project, Factory, then Factory
	// host order.
	Heartbeats() ([]Heartbeat, error)
	// PruneHeartbeats deletes the Heartbeats expired at now.
	PruneHeartbeats(now time.Time) error
	// PutFactoryReports writes r, replacing its Project's.
	PutFactoryReports(r FactoryReports) error
	// FactoryReports is every Project's Factory reports, in Project
	// order.
	FactoryReports() ([]FactoryReports, error)
	// PutPing writes p, replacing its session's.
	PutPing(p Ping) error
	// Pings is every session's last counted ping, in session order.
	Pings() ([]Ping, error)
	// PrunePings deletes the pings from before before.
	PrunePings(before time.Time) error
	// PutTranscriptHolder writes h, replacing its Claim's: the newest
	// wins.
	PutTranscriptHolder(h TranscriptHolder) error
	// TranscriptHolder is the holder of project's ticket's Claim claim's
	// Transcript, and whether there is one.
	TranscriptHolder(project string, ticket, claim int) (TranscriptHolder, bool, error)
	// PruneTranscriptHolders deletes the holders written before before.
	PruneTranscriptHolders(before time.Time) error
}

// The runtime state every service sharing a Store sees (Spec
// code-central-1107): each Factory's latest Heartbeat, each Project's
// Factory reports, each session's last counted ping and which service
// holds each Claim's Transcript. Core keeps them in memory too, writes
// them through here and reads them back on its tick. A Store one service
// holds alone keeps them in memory only, as it keeps Registrations.

// Heartbeat is a Factory's latest Heartbeat as the Store keeps it: one
// per Project, Factory name and Factory host.
type Heartbeat struct {
	Project, Factory, FactoryHost string
	// Doc is the Heartbeat itself as its writer encodes it: its state,
	// note, interval, Chat support, Plan Limits and when it arrived. The
	// Store keeps it whole and reads nothing in it.
	Doc json.RawMessage
	// Service and ServiceHost are the Factory's Reporting Service's ID
	// and host: the service the Heartbeat reached.
	Service, ServiceHost string
	// Expires is when PruneHeartbeats deletes it.
	Expires time.Time
}

// FactoryReports are a Project's Base and clone reports, by Repository
// Name, as its Factories last reported them.
type FactoryReports struct {
	Project string
	// Doc is the reports as their writer encodes them; the Store keeps
	// them whole and reads nothing in them.
	Doc json.RawMessage
}

// Ping is a session's last counted ping.
type Ping struct {
	Session string
	At      time.Time
	// Service and ServiceHost are the ID and host of the service it
	// reached.
	Service, ServiceHost string
}

// TranscriptHolder names the service holding a Claim's Transcript: the
// one its Session Events reach.
type TranscriptHolder struct {
	Project       string
	Ticket, Claim int
	Service       string
	ServiceHost   string
	// At is when the service started holding it.
	At time.Time
}

// heartbeatKey is a Heartbeat's key, and holderKey a Transcript
// holder's.
type (
	heartbeatKey struct{ project, factory, host string }
	holderKey    struct {
		project       string
		ticket, claim int
	}
)

// byHeartbeat orders Heartbeats by Project, Factory, then Factory host.
func byHeartbeat(a, b Heartbeat) int {
	return cmp.Or(cmp.Compare(a.Project, b.Project), cmp.Compare(a.Factory, b.Factory), cmp.Compare(a.FactoryHost, b.FactoryHost))
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
	heartbeats    map[heartbeatKey]Heartbeat
	reports       map[string]FactoryReports
	pings         map[string]Ping
	holders       map[holderKey]TranscriptHolder
}

func newMemoryServices(format int) *memoryServices {
	return &memoryServices{format: format, registrations: map[string]Registration{}, leases: map[string]Lease{},
		heartbeats: map[heartbeatKey]Heartbeat{}, reports: map[string]FactoryReports{},
		pings: map[string]Ping{}, holders: map[holderKey]TranscriptHolder{}}
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

// The Docs are copied in and out, so no caller's slice aliases another's.

func (m *memoryServices) PutHeartbeat(h Heartbeat) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	h.Doc = slices.Clone(h.Doc)
	m.heartbeats[heartbeatKey{h.Project, h.Factory, h.FactoryHost}] = h
	return nil
}

func (m *memoryServices) Heartbeats() ([]Heartbeat, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Heartbeat, 0, len(m.heartbeats))
	for _, h := range m.heartbeats {
		h.Doc = slices.Clone(h.Doc)
		out = append(out, h)
	}
	slices.SortFunc(out, byHeartbeat)
	return out, nil
}

func (m *memoryServices) PruneHeartbeats(now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	maps.DeleteFunc(m.heartbeats, func(_ heartbeatKey, h Heartbeat) bool { return !now.Before(h.Expires) })
	return nil
}

func (m *memoryServices) PutFactoryReports(r FactoryReports) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r.Doc = slices.Clone(r.Doc)
	m.reports[r.Project] = r
	return nil
}

func (m *memoryServices) FactoryReports() ([]FactoryReports, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]FactoryReports, 0, len(m.reports))
	for _, project := range slices.Sorted(maps.Keys(m.reports)) {
		r := m.reports[project]
		r.Doc = slices.Clone(r.Doc)
		out = append(out, r)
	}
	return out, nil
}

func (m *memoryServices) PutPing(p Ping) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pings[p.Session] = p
	return nil
}

func (m *memoryServices) Pings() ([]Ping, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Ping, 0, len(m.pings))
	for _, session := range slices.Sorted(maps.Keys(m.pings)) {
		out = append(out, m.pings[session])
	}
	return out, nil
}

func (m *memoryServices) PrunePings(before time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	maps.DeleteFunc(m.pings, func(_ string, p Ping) bool { return p.At.Before(before) })
	return nil
}

func (m *memoryServices) PutTranscriptHolder(h TranscriptHolder) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.holders[holderKey{h.Project, h.Ticket, h.Claim}] = h
	return nil
}

func (m *memoryServices) TranscriptHolder(project string, ticket, claim int) (TranscriptHolder, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	h, ok := m.holders[holderKey{project, ticket, claim}]
	return h, ok, nil
}

func (m *memoryServices) PruneTranscriptHolders(before time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	maps.DeleteFunc(m.holders, func(_ holderKey, h TranscriptHolder) bool { return h.At.Before(before) })
	return nil
}
