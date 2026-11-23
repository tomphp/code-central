package foundation

import (
	"fmt"
	"io"
	"time"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/store"
)

// Foundation is what the Core and every area it builds on share: the
// Store the writes go through, the clock, the Claim ID source, the bus
// the change notices go out on, and this service's identity and log.
// Build one with New and set Store before the first operation.
type Foundation struct {
	Store  store.Store
	Clock  clock.Clock
	IDs    ids.Source
	Events *events.Bus
	// service is this service as the Store's record of the services
	// running on it names it.
	service Service
	// log is where this service logs what it can't write to or read from
	// the Store, and carries on without; nil logs nothing.
	log io.Writer
}

// New returns a Foundation telling time with clk, issuing Claim IDs from
// idSource and sending change notices on bus.
func New(clk clock.Clock, idSource ids.Source, bus *events.Bus) *Foundation {
	return &Foundation{Clock: clk, IDs: idSource, Events: bus}
}

// Now is the current time as stored: UTC, whole seconds.
func (f *Foundation) Now() time.Time { return f.Clock.Now().UTC().Truncate(time.Second) }

// SetLog makes the service log what it can't write to or read from the
// Store, and carries on without, to w. Without one, nothing is logged.
func (f *Foundation) SetLog(w io.Writer) { f.log = w }

// Logf logs a line to the service's log, if it has one.
func (f *Foundation) Logf(format string, a ...any) {
	if f.log != nil {
		fmt.Fprintf(f.log, "code-central serve: "+format+"\n", a...)
	}
}

// ClaimKey names what a timed reason, a Transcript or a latest event
// notice is on, across Projects: one Claim, or, with Claim 0, a whole
// Ticket.
type ClaimKey struct {
	Project string
	Ticket  int
	Claim   int
}

// NewestClaim is the most recently started Claim with the session ID, in
// any Project, and when it started.
func (f *Foundation) NewestClaim(sessionID string) (ClaimKey, time.Time, bool) {
	var sc store.SessionClaim
	found := false
	f.Store.View(func(r store.Reader) error {
		sc, found = r.ClaimBySession(sessionID)
		return nil
	})
	return ClaimKey{sc.Project, sc.Ticket, sc.Claim.Number}, sc.Claim.Started, found
}

// PublishTicketChanged sends a ticket.changed notice for ticket in project.
func (f *Foundation) PublishTicketChanged(project string, ticket int) {
	f.Events.Publish(events.Event{Type: events.TicketChanged, Project: project, ID: domain.DisplayID(project, ticket)})
}

// WaitTimeout is how long one long-poll call holds before returning, on
// the injected clock.
const WaitTimeout = 60 * time.Second
