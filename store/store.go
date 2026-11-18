// Package store is the persistence interface core writes through.
//
// Two implementations exist: Open (YAML files on disk, loaded whole into
// memory at startup) and NewMemory (an in-memory fake). Both pass the
// contract suite in store/storetest. Every read comes from memory; every
// Update writes at most one record, and memory changes only after that
// record is durable.
package store

import (
	"errors"
	"maps"
	"slices"
)

// Reader reads from memory. Records returned are copies.
type Reader interface {
	Project(key string) (Project, bool)
	Projects() []Project
	Spec(project string, n int) (Spec, bool)
	Specs(project string) []Spec
	Ticket(project string, n int) (Ticket, bool)
	// Tickets returns the project's Tickets in number order.
	Tickets(project string) []Ticket
	// NextNumber is the highest Spec or Ticket number in the project plus
	// one. It is derived, never stored, so numbers are never reused.
	NextNumber(project string) int
	// Options is always a whole value: the defaults until any are written.
	Options() Options
	// CIRuns is the project's CI Runs, in the order they were written.
	CIRuns(project string) []CIRun
}

// Writer is a Reader that can also write one record.
type Writer interface {
	Reader
	PutProject(Project) error
	PutSpec(project string, s Spec) error
	PutTicket(project string, t Ticket) error
	// PutImport is the one bulk write: every Spec and Ticket in im is new,
	// but for the existing ones it rewrites, and they all land together
	// or none does. It counts as the update's one write.
	PutImport(project string, im Import) error
	// PutOptions replaces the Options whole. It touches no Project.
	PutOptions(Options) error
	// PutCIRuns replaces the project's CI Runs whole.
	PutCIRuns(project string, runs []CIRun) error
}

// Import is a batch of new Specs and Tickets written all or nothing.
type Import struct {
	Specs   []Spec
	Tickets []Ticket
	// Rewritten are existing Tickets written along with the new records,
	// such as the Incoming Issue To Requirement Cancels.
	Rewritten []Ticket
	// RewrittenSpecs are existing Specs written along with them, such as
	// a Spec given the auto-accept Label with the Tickets it accepts.
	RewrittenSpecs []Spec
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

// Store is the persistence interface. View and Update share one
// store-wide read-write mutex, so a check and the write it guards are
// atomic.
type Store interface {
	View(fn func(Reader) error) error
	// Update runs fn under the write lock. fn may write at most one
	// record; a second write fails with ErrSecondWrite.
	Update(fn func(Writer) error) error
}

var (
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

func cloneSpec(s Spec) Spec {
	s.Labels = slices.Clone(s.Labels)
	s.Tags = maps.Clone(s.Tags)
	s.Edits = cloneEdits(s.Edits)
	s.Link.Written = slices.Clone(s.Link.Written)
	return s
}

func cloneTicket(t Ticket) Ticket {
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
	return t
}

func cloneEdits(es []Edit) []Edit {
	es = slices.Clone(es)
	for i := range es {
		es[i].Previous = maps.Clone(es[i].Previous)
	}
	return es
}

func cloneCIRun(r CIRun) CIRun {
	r.Jobs = slices.Clone(r.Jobs)
	return r
}

func cloneCIRuns(rs CIRuns) CIRuns {
	out := CIRuns{Runs: make([]CIRun, 0, len(rs.Runs))}
	for _, r := range rs.Runs {
		out.Runs = append(out.Runs, cloneCIRun(r))
	}
	return out
}

// landingOrDefault fills in what l leaves empty from DefaultLanding.
func landingOrDefault(l Landing) Landing {
	if l.Method == "" {
		// A Landing with no method was never set: Land per Spec is on.
		l.Method = LandingMerge
		l.LandPerSpec = true
	}
	if l.Remote == "" {
		l.Remote = DefaultLandingRemote
	}
	return l
}
