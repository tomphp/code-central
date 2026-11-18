// Package store is the persistence interface core writes through.
//
// Its Store Kinds (ADR 0067) are Open (YAML files on disk, loaded whole
// into memory at startup), OpenSQLite (one database file), OpenPostgres
// (a database several services share, ADR 0068) and NewMemory (an
// in-memory fake). All pass the contract suite in store/storetest. Every
// Update writes at most one record, and what a Reader sees changes only
// after that record is durable.
package store

import (
	"errors"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/usage"
	"iter"
	"maps"
	"slices"
)

// Reader reads from memory. Records returned are copies.
type Reader interface {
	Project(key string) (domain.Project, bool)
	Projects() []domain.Project
	Spec(project string, n int) (domain.Spec, bool)
	// Ticket reads Ticket n with the History parts named: the parts it
	// doesn't name come back empty, and its head's derived fields are
	// always there.
	Ticket(project string, n int, parts ...Part) (domain.Ticket, bool)
	// FindTickets returns the project's Tickets f picks, each with the
	// History parts f names, in f's Order.
	FindTickets(project string, f TicketFilter) []domain.Ticket
	// FindSpecs returns the project's Specs f picks, in number order.
	FindSpecs(project string, f SpecFilter) []domain.Spec
	// StateCounts is how many of the project's Tickets are in each State;
	// a State no Ticket is in is left out.
	StateCounts(project string) map[domain.State]int
	// ClaimBySession is the most recently started Claim with the session
	// ID, live or ended, in any Project; on a tie, the first in Project,
	// Ticket and Claim order. An empty session ID finds none.
	ClaimBySession(sessionID string) (SessionClaim, bool)
	// TicketCommits is every commit the project's Tickets pushed and
	// landed at, in number order, and each Ticket's pushed commits in the
	// order they were pushed, then its landed ones: the Home Repository's
	// from its Moves, then the other Repositories' in their order.
	TicketCommits(project string) []TicketCommit
	// Records reads the project's records whole: its Specs, then its
	// Tickets, each in number order and each Ticket with every History
	// part. It is for Export and Store Move, which copy a Project; every
	// other read names what it needs.
	Records(project string) iter.Seq[Record]
	// NextNumber is the highest Spec or Ticket number in the project plus
	// one. It is derived, never stored, so numbers are never reused.
	NextNumber(project string) int
	// Options is always a whole value: the defaults until any are written.
	Options() domain.Options
	// CIRuns is the project's CI Runs, in the order they were written.
	CIRuns(project string) []domain.CIRun
}

// Writer is a Reader that can also write one record.
type Writer interface {
	Reader
	PutProject(domain.Project) error
	PutSpec(project string, s domain.Spec) error
	// PutTicket writes t's head and the History parts named. The parts it
	// doesn't name stay as stored: none, for a new Ticket. The Store works
	// out the head's derived fields from the History it then holds.
	PutTicket(project string, t domain.Ticket, parts ...Part) error
	// PutImport is the one bulk write: every Spec and Ticket in im is new,
	// but for the existing ones it rewrites, and they all land together
	// or none does. It counts as the update's one write. Each Ticket is
	// written whole, every History part with it.
	PutImport(project string, im Import) error
	// PutOptions replaces the Options whole. It touches no Project.
	PutOptions(domain.Options) error
	// PutCIRuns replaces the project's CI Runs whole.
	PutCIRuns(project string, runs []domain.CIRun) error
	// Announce tells the other services sharing the Store of the update's
	// write, once it commits. A Store no other service shares ignores it.
	Announce(Change)
}

// Change is the notice of a write one service committed, as another
// service sharing the Store hears of it: the Project, the record as an
// event type and ID, and whether it wakes the Project's Factories.
type Change struct {
	Project string
	// Type is the record's event type, such as "ticket.changed".
	Type string
	// ID is the display ID of the Ticket or Spec, empty for the others.
	ID string
	// Wakes says the write is one the Factory's poll acts on.
	Wakes bool
}

// Import is a batch of new Specs and Tickets written all or nothing.
type Import struct {
	Specs   []domain.Spec
	Tickets []domain.Ticket
	// Rewritten are existing Tickets written along with the new records,
	// such as the Incoming Issue To Requirement Cancels.
	Rewritten []domain.Ticket
	// RewrittenSpecs are existing Specs written along with them, such as
	// a Spec given the auto-accept Label with the Tickets it accepts.
	RewrittenSpecs []domain.Spec
}

func (im Import) clone() Import {
	out := Import{}
	for _, s := range im.Specs {
		out.Specs = append(out.Specs, cloneSpec(s))
	}
	for _, t := range im.Tickets {
		out.Tickets = append(out.Tickets, cloneTicket(t))
	}
	for _, t := range im.Rewritten {
		out.Rewritten = append(out.Rewritten, cloneTicket(t))
	}
	for _, sp := range im.RewrittenSpecs {
		out.RewrittenSpecs = append(out.RewrittenSpecs, cloneSpec(sp))
	}
	return out
}

// Store is the persistence interface. A check and the write it guards
// are atomic: View and Update share one store-wide read-write mutex, or,
// on a Shared Store, each Update is one transaction the database orders
// against the other services' (ADR 0068).
type Store interface {
	View(fn func(Reader) error) error
	// Update runs fn under the write lock, or in a transaction of its
	// own. fn may write at most one
	// record; a second write fails with ErrSecondWrite. A Store that
	// retries on conflict runs fn again from the start, discarding the
	// earlier attempt's write, so fn must be safe to run more than once;
	// when its attempts run out, Update fails with ErrConflict.
	Update(fn func(Writer) error) error
	// Changes makes deliver hear of the Changes other services sharing the
	// Store commit. A Store no other service shares never calls it.
	Changes(deliver func(Change))
	// Usage is the Store's Usage (ADR 0066): what agents consume, kept
	// the way the Store Kind keeps it, and appended to, read and pruned on
	// its own, outside any View or Update.
	Usage() usage.Store
}

var (
	// ErrConflict means an Update kept conflicting with other services'
	// writes until its attempts ran out; trying again may succeed.
	ErrConflict = errors.New("store: another service changed this at the same moment")
	// ErrSecondWrite means an Update tried to write more than one record.
	ErrSecondWrite = errors.New("store: an update may write only one record")
	// ErrUnknownProject means a Spec or Ticket was written to a project
	// that does not exist.
	ErrUnknownProject = errors.New("store: unknown project")
	// ErrExists means an Import named a Spec or Ticket number already in use.
	ErrExists = errors.New("store: an import may only add new records")
	// ErrNotExists means an Import rewrote a Ticket or Spec that doesn't
	// exist.
	ErrNotExists = errors.New("store: an import may only rewrite existing Tickets and Specs")
	// ErrInvalidOption means an Option written was outside its set.
	ErrInvalidOption = errors.New("store: an Option's value is outside its set")
)

func cloneSpec(s domain.Spec) domain.Spec {
	s.Labels = slices.Clone(s.Labels)
	s.Tags = maps.Clone(s.Tags)
	s.Edits = cloneEdits(s.Edits)
	s.Link.Written = slices.Clone(s.Link.Written)
	return s
}

func cloneTicket(t domain.Ticket) domain.Ticket {
	t.Labels = slices.Clone(t.Labels)
	t.Tags = maps.Clone(t.Tags)
	t.BlockedBy = slices.Clone(t.BlockedBy)
	t.Replacements = slices.Clone(t.Replacements)
	t.PushedCommits = slices.Clone(t.PushedCommits)
	t.Repositories = slices.Clone(t.Repositories)
	for i, r := range t.Repositories {
		if r.Landed != nil {
			l := *r.Landed
			t.Repositories[i].Landed = &l
		}
	}
	if t.SpecMerge != nil {
		sm := *t.SpecMerge
		t.SpecMerge = &sm
	}
	t.PullRequests = slices.Clone(t.PullRequests)
	t.Claims = slices.Clone(t.Claims)
	t.Rounds = slices.Clone(t.Rounds)
	for i := range t.Rounds {
		t.Rounds[i].Questions = slices.Clone(t.Rounds[i].Questions)
	}
	t.FailureReports = slices.Clone(t.FailureReports)
	t.Comments = slices.Clone(t.Comments)
	t.Moves = slices.Clone(t.Moves)
	t.Edits = cloneEdits(t.Edits)
	t.Link.Written = slices.Clone(t.Link.Written)
	t.Head = cloneHead(t.Head)
	return t
}

func cloneEdits(es []domain.Edit) []domain.Edit {
	es = slices.Clone(es)
	for i := range es {
		es[i].Previous = maps.Clone(es[i].Previous)
	}
	return es
}

func cloneCIRun(r domain.CIRun) domain.CIRun {
	r.Jobs = slices.Clone(r.Jobs)
	return r
}

func cloneCIRuns(rs domain.CIRuns) domain.CIRuns {
	out := domain.CIRuns{Runs: make([]domain.CIRun, 0, len(rs.Runs))}
	for _, r := range rs.Runs {
		out.Runs = append(out.Runs, cloneCIRun(r))
	}
	return out
}

// landingOrDefault fills in what l leaves empty from DefaultLanding.
func landingOrDefault(l domain.Landing) domain.Landing {
	if l.Method == "" {
		// A Landing with no method was never set: Land per Spec is on.
		l.Method = domain.LandingMerge
		l.LandPerSpec = true
	}
	if l.Remote == "" {
		l.Remote = domain.DefaultLandingRemote
	}
	return l
}
