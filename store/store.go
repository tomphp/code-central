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
	"time"
)

// State is a Ticket's single position in the workflow.
type State string

// The nine States of the one fixed workflow.
const (
	Draft       State = "Draft"
	Ready       State = "Ready"
	InProgress  State = "In Progress"
	NeedsAnswer State = "Needs Answer"
	Answered    State = "Answered"
	Failed      State = "Failed"
	InReview    State = "In Review"
	Done        State = "Done"
	Cancelled   State = "Cancelled"
)

// States lists every State in workflow order.
var States = []State{Draft, Ready, InProgress, NeedsAnswer, Answered, Failed, InReview, Done, Cancelled}

// Valid reports whether s is one of the nine States.
func (s State) Valid() bool { return slices.Contains(States, s) }

// Project is one tracked repository.
type Project struct {
	Key         string
	DisplayName string
	Created     time.Time
	Mode        string
	Archived    bool
}

// Spec is a description of work, stored as Markdown with YAML frontmatter.
type Spec struct {
	Number  int
	Title   string
	Labels  []string
	Tags    map[string]string
	Link    string
	Created time.Time
	Updated time.Time
	Body    string
}

// Move records one State change on a Ticket.
type Move struct {
	From  State
	To    State
	Actor string
	Time  time.Time
	Claim int // Claim number, 0 when none
	Via   string
}

// Ticket is a unit of work within a Spec.
type Ticket struct {
	Number       int
	Title        string
	State        State
	Spec         int
	Labels       []string
	Tags         map[string]string
	BlockedBy    []int
	Replacements []int
	Link         string
	Created      time.Time
	Updated      time.Time
	Description  string
	Moves        []Move
}

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
}

// Writer is a Reader that can also write one record.
type Writer interface {
	Reader
	PutProject(Project) error
	PutSpec(project string, s Spec) error
	PutTicket(project string, t Ticket) error
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
)

func (s Spec) clone() Spec {
	s.Labels = slices.Clone(s.Labels)
	s.Tags = maps.Clone(s.Tags)
	return s
}

func (t Ticket) clone() Ticket {
	t.Labels = slices.Clone(t.Labels)
	t.Tags = maps.Clone(t.Tags)
	t.BlockedBy = slices.Clone(t.BlockedBy)
	t.Replacements = slices.Clone(t.Replacements)
	t.Moves = slices.Clone(t.Moves)
	return t
}
