package api

import (
	"time"

	"github.com/tomoram/code-central/domain"
)

// The parts of a Spec a lighter get_spec can ask for. A Ticket has edits
// too, so PartEdits names its edits part as well.
const (
	PartBody    = "body"
	PartTickets = "tickets"
	PartEdits   = "edits"
	// PartOutline reads the outline alone: every read but the full one
	// returns it, so naming it asks for nothing else.
	PartOutline = "outline"
)

// A Spec's State is never stored: its StateSummary is derived as a count
// per State of its Tickets, or {Draft: 0} when it has no Tickets. A Spec
// is finished when it has Tickets and all of them are finished.
type StateSummary map[domain.State]int

// SpecDetail is get_spec's result: the Spec, its State summary and its
// Tickets as find_tickets rows (finished ones too), in claim order. Its
// first fields are SpecView's, in its order, so the full read is the
// SpecView it used to embed; a lighter read leaves out each part not asked
// for (nil, so it has no key) and adds the body's Outline.
type SpecDetail struct {
	ID      string            `json:"id"`
	Title   string            `json:"title"`
	Labels  []string          `json:"labels"`
	Tags    map[string]string `json:"tags"`
	Created time.Time         `json:"created"`
	Updated time.Time         `json:"updated"`
	Body    *string           `json:"body,omitempty"`
	Edits   []EditView        `json:"edits,omitzero"`
	// Link is set on a linked Spec (ticket 42).
	Link *LinkView `json:"link,omitempty"`
	// Outline is every heading of the whole body, on a lighter read only.
	Outline      []Heading    `json:"outline,omitzero"`
	StateSummary StateSummary `json:"state_summary"`
	Finished     bool         `json:"finished"`
	// Kind, Need and Claimed are as on SpecRow.
	Kind    SpecKind      `json:"kind"`
	Need    NeedsMeReason `json:"need,omitempty"`
	Claimed bool          `json:"claimed"`
	// Editable says the UI offers Edit on the Spec, and Retroable its
	// Retro button, as specEditable and specRetroable decide them. Every
	// read sets them.
	Editable  bool `json:"editable"`
	Retroable bool `json:"retroable"`
	// Ticketable says the UI offers New Ticket on the Spec, as
	// whyNotNewTicket decides it, the rule new_ticket refuses by. Every
	// read sets it.
	Ticketable bool `json:"ticketable"`
	// Repositories are the Spec's Changed Repositories, the union of its
	// Tickets', by Name in landing order; left out while the Project has
	// the Home Repository alone.
	Repositories []string    `json:"repositories,omitempty"`
	Tickets      []TicketRow `json:"tickets,omitzero"`
	// Requirement is set only when the Spec is a Requirement.
	Requirement *RequirementView `json:"requirement,omitempty"`
	// LandPerSpec is the Spec's Land per Spec status, left out unless it is
	// a Long-lived Spec or the Project isn't under Merge or Pull Request
	// with Land per Spec on.
	LandPerSpec *LandPerSpecView `json:"land_per_spec,omitempty"`
	// Retros are the Retros looking back on this Spec, oldest first; left
	// out when there are none.
	Retros []RetroView `json:"retros,omitempty"`
	// Map is set only when the Spec is a Map.
	Map *MapView `json:"map,omitempty"`
}

// GetSpecInput is get_spec's input. With neither Parts nor Sections, the
// read is the full Spec; with Sections and no Parts, it is only the body.
type GetSpecInput struct {
	Spec Ref `json:"spec" path:"n" desc:"The Spec." required:"true" cli:"<id>"`
	// Parts are any of body, tickets, edits and outline; all three when empty, or
	// only body when Sections are given.
	Parts []string `json:"parts,omitempty" query:"part" desc:"Only these parts, any of body, tickets, edits and outline; all three when left out, or only body with sections." cli:"--part body|tickets|edits"`
	// Sections are heading texts: the body returned is only those Sections.
	Sections []string `json:"sections,omitempty" query:"section" desc:"Only the body's Sections under these headings, matched case-insensitively at any level." cli:"--section <heading>"`
}

// SpecRow is one find_specs result.
type SpecRow struct {
	ID           string            `json:"id"`
	Title        string            `json:"title"`
	Labels       []string          `json:"labels"`
	Tags         map[string]string `json:"tags"`
	StateSummary StateSummary      `json:"state_summary"`
	Finished     bool              `json:"finished"`
	Updated      time.Time         `json:"updated"`
	Link         *LinkView         `json:"link,omitempty"`
	// Kind is the Spec's kind, for its badge.
	Kind SpecKind `json:"kind"`
	// Need is the reason of the first of its Tickets, in claim order,
	// that waits on the human, as Needs Me reads it; left out when none
	// does. Claimed says any of its Tickets holds a live Claim. A plain
	// Spec's headline says them. Computed at read time.
	Need    NeedsMeReason `json:"need,omitempty"`
	Claimed bool          `json:"claimed"`
	// Requirement is set only when the Spec is a Requirement.
	Requirement *RequirementView `json:"requirement,omitempty"`
	// Map is set only when the Spec is a Map.
	Map *MapView `json:"map,omitempty"`
}

// SpecKind is a Spec's kind (the glossary's Spec kind), which its badge
// shows.
type SpecKind string

// The Spec kinds, in the order a Spec is read as one, so a Spec that is a
// Requirement and Long-lived is a Requirement.
const (
	SpecKindRequirement SpecKind = "requirement"
	SpecKindMap         SpecKind = "map"
	SpecKindLongLived   SpecKind = "long_lived"
	SpecKindSpec        SpecKind = "spec"
)

// SpecKinds are the Spec kinds, in precedence order.
func SpecKinds() []SpecKind {
	return []SpecKind{SpecKindRequirement, SpecKindMap, SpecKindLongLived, SpecKindSpec}
}

// FindSpecsInput is find_specs' filters; every filter given must match.
type FindSpecsInput struct {
	Title  string   `json:"title,omitempty" desc:"Only Specs whose title contains this text." cli:"--title <t>"`
	Labels []string `json:"labels,omitempty" query:"label" desc:"Only Specs with all of these Labels." cli:"--label <l>"`
	// Tags must all be present with exactly these values, or with any
	// value for an empty one: tag=branch= finds every Spec with a branch
	// Tag. Over HTTP each is a tag=<key>=<value> parameter, which
	// DecodeQuery reads.
	Tags map[string]string `json:"tags,omitempty" query:"tag" desc:"Only Specs with all of these Tags, as {key: value}; an empty value matches any value." cli:"--tag <key>=<value>"`
	// Finished includes finished Specs, which are otherwise left out.
	Finished bool `json:"finished,omitempty" desc:"Include finished Specs."`
	// Requirements keeps only Specs that are Requirements.
	Requirements bool `json:"requirements,omitempty" desc:"Only Requirements: Specs with Stage Tickets."`
}
