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

// The ten States of the one fixed workflow.
const (
	Draft       State = "Draft"
	Ready       State = "Ready"
	InProgress  State = "In Progress"
	NeedsAnswer State = "Needs Answer"
	Answered    State = "Answered"
	Failed      State = "Failed"
	InReview    State = "In Review"
	Accepted    State = "Accepted"
	Done        State = "Done"
	Cancelled   State = "Cancelled"
)

// States lists every State in workflow order.
var States = []State{Draft, Ready, InProgress, NeedsAnswer, Answered, Failed, InReview, Accepted, Done, Cancelled}

// Valid reports whether s is one of the ten States.
func (s State) Valid() bool { return slices.Contains(States, s) }

// Mode is a Project's mode: where init puts its files, committed to the
// repository or kept local to one checkout (`init --local`).
type Mode string

// The two modes.
const (
	ModeCommitted Mode = "committed"
	ModeLocal     Mode = "local"
)

// Valid reports whether m is one of the two modes.
func (m Mode) Valid() bool { return m == ModeCommitted || m == ModeLocal }

// Project is one tracked repository.
type Project struct {
	Key         string
	DisplayName string
	Created     time.Time
	Mode        Mode
	// Archived is when the human archived the Project; zero while it isn't.
	Archived time.Time
}

// Spec is a description of work, stored as Markdown with YAML frontmatter.
type Spec struct {
	Number  int
	Title   string
	Labels  []string
	Tags    map[string]string
	Link    Link
	Created time.Time
	Updated time.Time
	Body    string
	Edits   []Edit
}

// Comment is one numbered comment on a Ticket. Required notes on actions
// (a finish summary, a reject comment...) are stored as comments too.
type Comment struct {
	Number int
	Actor  string
	Time   time.Time
	Claim  int // Claim number the note was made under, 0 when none
	Body   string
}

// Edit records one edit that changed a Spec's or Ticket's text. Previous
// maps each changed field (title, body, description) to its old value.
type Edit struct {
	Number   int
	Actor    string
	Time     time.Time
	Previous map[string]string
}

// Move records one State change on a Ticket.
type Move struct {
	From  State
	To    State
	Actor string
	Time  time.Time
	Claim int // Claim number, 0 when none
	Via   string
	// Commit is the commit that put the work into the base, on a land.
	Commit string
}

// Claim is one agent's hold on a Ticket, from the claim until a move out
// of In Progress, Needs Answer or Answered ends it for good.
type Claim struct {
	Number    int
	ID        string // the Claim ID; never returned by a read
	SessionID string
	// WorkingDir and ResumeCommand are free-form, never parsed: how the
	// human gets back into the Claim's session.
	WorkingDir    string
	ResumeCommand string
	Started       time.Time
	Ended         time.Time // zero while the Claim is live
	EndedBy       string    // the action that ended it
}

// FailureReport is one numbered, permanent report of why an agent with
// no way forward stopped. Posting one ends the Claim it was made on.
type FailureReport struct {
	Number          int
	Claim           int
	SessionID       string // the Claim's session ID when the report was posted
	WorkingDir      string // the Claim's working directory then
	ResumeCommand   string // the Claim's Resume Command then
	Actor           string
	Time            time.Time
	WhatBroke       string
	Why             string
	WhatToFix       string
	WhereWorkStands string
}

// Live reports whether the Claim has not ended.
func (c Claim) Live() bool { return c.Ended.IsZero() }

// QuestionStatus is where one Question stands.
type QuestionStatus string

// The three Question statuses.
const (
	QuestionOpen      QuestionStatus = "open"
	QuestionAnswered  QuestionStatus = "answered"
	QuestionDismissed QuestionStatus = "dismissed"
)

// Question is one query in a Round. Questions are numbered across the
// Ticket, so a number names one Question on its Ticket.
type Question struct {
	Number      int
	Title       string
	Body        string
	Recommended string
	Status      QuestionStatus
	Answer      string
	Closed      time.Time // when last answered or dismissed; zero while open
}

// Round is the Questions an agent posted together, in one write.
type Round struct {
	Number    int
	Claim     int // the Claim number it was posted under
	SessionID string
	Posted    time.Time
	Questions []Question
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
	Link         Link
	Created      time.Time
	Updated      time.Time
	Description  string
	// Branch and Commit are what the last finish naming them gave: the
	// work's branch and its head commit. Free-form, never parsed.
	Branch string
	Commit string
	Claims []Claim
	Rounds []Round
	// FailureReports are numbered and never edited.
	FailureReports []FailureReport
	Comments       []Comment
	Moves          []Move
	Edits          []Edit
}

// FocusClear is what Focus Mode shows when nothing needs the human.
type FocusClear string

// The two things Focus Mode can show when nothing needs the human.
const (
	FocusAllClear          FocusClear = "all_clear"
	FocusActivityDashboard FocusClear = "activity_dashboard"
)

// FocusClears lists every FocusClear, the default first.
var FocusClears = []FocusClear{FocusAllClear, FocusActivityDashboard}

// Valid reports whether f is one of the FocusClears.
func (f FocusClear) Valid() bool { return slices.Contains(FocusClears, f) }

// Options are the human's global preferences, set from the UI's Options
// view. They are the store's one record that belongs to no Project.
type Options struct {
	FocusClear FocusClear
}

// DefaultOptions are the Options of a store that has never had any
// written.
func DefaultOptions() Options { return Options{FocusClear: FocusAllClear} }

// focusClearNames is the FocusClears as strings, for messages and the
// schema.
func focusClearNames() []string {
	var out []string
	for _, f := range FocusClears {
		out = append(out, string(f))
	}
	return out
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
	// Options is always a whole value: the defaults until any are written.
	Options() Options
}

// Writer is a Reader that can also write one record.
type Writer interface {
	Reader
	PutProject(Project) error
	PutSpec(project string, s Spec) error
	PutTicket(project string, t Ticket) error
	// PutImport is the one bulk write: every Spec and Ticket in im is new,
	// and they all land together or none does. It counts as the update's
	// one write.
	PutImport(project string, im Import) error
	// PutOptions replaces the Options whole. It touches no Project.
	PutOptions(Options) error
}

// Import is a batch of new Specs and Tickets written all or nothing.
type Import struct {
	Specs   []Spec
	Tickets []Ticket
}

func (im Import) clone() Import {
	out := Import{}
	for _, s := range im.Specs {
		out.Specs = append(out.Specs, s.clone())
	}
	for _, t := range im.Tickets {
		out.Tickets = append(out.Tickets, t.clone())
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
	// ErrInvalidOption means an Option written was outside its set.
	ErrInvalidOption = errors.New("store: an Option's value is outside its set")
)

func (s Spec) clone() Spec {
	s.Labels = slices.Clone(s.Labels)
	s.Tags = maps.Clone(s.Tags)
	s.Edits = cloneEdits(s.Edits)
	return s
}

func (t Ticket) clone() Ticket {
	t.Labels = slices.Clone(t.Labels)
	t.Tags = maps.Clone(t.Tags)
	t.BlockedBy = slices.Clone(t.BlockedBy)
	t.Replacements = slices.Clone(t.Replacements)
	t.Claims = slices.Clone(t.Claims)
	t.Rounds = slices.Clone(t.Rounds)
	for i := range t.Rounds {
		t.Rounds[i].Questions = slices.Clone(t.Rounds[i].Questions)
	}
	t.FailureReports = slices.Clone(t.FailureReports)
	t.Comments = slices.Clone(t.Comments)
	t.Moves = slices.Clone(t.Moves)
	t.Edits = cloneEdits(t.Edits)
	return t
}

func cloneEdits(es []Edit) []Edit {
	es = slices.Clone(es)
	for i := range es {
		es[i].Previous = maps.Clone(es[i].Previous)
	}
	return es
}
