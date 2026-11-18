package core

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
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

var specParts = []string{PartBody, PartTickets, PartEdits, PartOutline}

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

// specKind is s's kind, given its Requirement view: a Requirement, else a
// Map, else Long-lived, else a Spec.
func specKind(s domain.Spec, req *RequirementView) SpecKind {
	switch {
	case req != nil:
		return SpecKindRequirement
	case isMap(s.Labels):
		return SpecKindMap
	case slices.Contains(s.Labels, LongLivedLabel):
		return SpecKindLongLived
	}
	return SpecKindSpec
}

// specNeed is the reason of the first of tickets, in claim order, that
// waits on the human, read from e as Needs Me reads it; "" when none does.
func specNeed(e *evaluation, tickets []domain.Ticket) NeedsMeReason {
	for _, t := range tickets {
		if r, _ := e.reason(t); r != "" {
			return r
		}
	}
	return ""
}

// specClaimed reports whether any of tickets holds a live Claim.
func specClaimed(tickets []domain.Ticket) bool {
	return slices.ContainsFunc(tickets, func(t domain.Ticket) bool {
		_, ok := liveClaim(t)
		return ok
	})
}

// FindSpecsInput is find_specs' filters; every filter given must match.
type FindSpecsInput struct {
	Title  string   `json:"title,omitempty" desc:"Only Specs whose title contains this text." cli:"--title <t>"`
	Labels []string `json:"labels,omitempty" query:"label" desc:"Only Specs with all of these Labels." cli:"--label <l>"`
	// Tags must all be present with exactly these values. Over HTTP each
	// is a tag=<key>=<value> parameter, which DecodeQuery reads.
	Tags map[string]string `json:"tags,omitempty" query:"tag" desc:"Only Specs with all of these Tags, as {key: value}." cli:"--tag <key>=<value>"`
	// Finished includes finished Specs, which are otherwise left out.
	Finished bool `json:"finished,omitempty" desc:"Include finished Specs."`
	// Requirements keeps only Specs that are Requirements.
	Requirements bool `json:"requirements,omitempty" desc:"Only Requirements: Specs with Stage Tickets."`
}

// GetSpec is get_spec(spec, parts?, sections?).
func (c *Core) GetSpec(_ context.Context, key string, in GetSpecInput) (SpecDetail, error) {
	parts, err := in.parts()
	if err != nil {
		return SpecDetail{}, err
	}
	var v SpecDetail
	err = c.store.View(func(r store.Reader) error {
		p, ok := r.Project(key)
		if !ok {
			return unknownProject(key)
		}
		s, err := resolveSpec(r, key, "get_spec", "spec", in.Spec)
		if err != nil {
			return err
		}
		n := s.Number
		tickets := specTickets(r, key, n)
		sv := specView(key, s)
		v = SpecDetail{
			ID: sv.ID, Title: sv.Title, Labels: sv.Labels, Tags: sv.Tags,
			Created: sv.Created, Updated: sv.Updated, Link: sv.Link,
		}
		v.StateSummary, v.Finished = summarise(tickets)
		if severalRepositories(p) {
			v.Repositories = specRepositories(p, tickets)
		}
		e := c.evaluate(r, p)
		v.Requirement = requirementView(e, s, tickets)
		v.Kind, v.Need, v.Claimed = specKind(s, v.Requirement), specNeed(e, tickets), specClaimed(tickets)
		v.Editable, v.Retroable = specEditable(p, s, v.Finished), specRetroable(p, s, v.Finished)
		v.Ticketable = whyNotNewTicket(p, s, tickets) == ""
		v.LandPerSpec = landPerSpecView(r, key, s, tickets)
		v.Map = mapView(e, s, tickets)
		v.Retros = retrosOf(r, key, DisplayID(key, n))
		if in.lighter() {
			v.Outline = Outline(s.Body)
		}
		if parts[PartBody] {
			body := sv.Body
			if len(in.Sections) > 0 {
				var unmatched []string
				if body, unmatched = Sections(s.Body, in.Sections); unmatched != nil {
					return noSuchSections(unmatched, v.Outline)
				}
			}
			v.Body = &body
		}
		if parts[PartEdits] {
			v.Edits = sv.Edits
		}
		if parts[PartTickets] {
			v.Tickets = []TicketRow{}
			onMap := isMap(s.Labels)
			for _, t := range tickets {
				v.Tickets = append(v.Tickets, c.ticketRow(r, e, t, onMap))
			}
		}
		return nil
	})
	return v, err
}

// lighter reports whether the read asks for less than the full Spec, so
// returns the Outline.
func (in GetSpecInput) lighter() bool { return len(in.Parts) > 0 || len(in.Sections) > 0 }

// parts is the set of parts the read returns, checking them against
// Sections. Sections without Parts read only the body.
func (in GetSpecInput) parts() (map[string]bool, error) {
	asked := in.Parts
	switch {
	case len(asked) > 0:
	case len(in.Sections) > 0:
		asked = []string{PartBody}
	default:
		asked = specParts
	}
	out, err := partSet(asked, specParts)
	if err != nil {
		return nil, err
	}
	if len(in.Sections) > 0 && !out[PartBody] {
		return nil, invalid("sections", "sections cut the body, so parts must include body")
	}
	return out, nil
}

// noSuchSections rejects Section names that match no heading, listing the
// headings there are.
func noSuchSections(names []string, outline []Heading) *Error {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = strconv.Quote(n)
	}
	headings := make([]string, len(outline))
	for i, h := range outline {
		headings[i] = h.Text
	}
	if len(headings) == 0 {
		return invalid("sections", "no heading matches %s: the body has no headings", strings.Join(quoted, ", "))
	}
	return invalid("sections", "no heading matches %s; the headings are: %s", strings.Join(quoted, ", "), strings.Join(headings, "; "))
}

// FindSpecs is find_specs(title?, labels?, tags?, finished?). Finished
// Specs are left out unless finished is true. Rows are in creation order.
func (c *Core) FindSpecs(_ context.Context, key string, in FindSpecsInput) ([]SpecRow, error) {
	if err := checkTagFilter(in.Tags); err != nil {
		return nil, err
	}
	rows := []SpecRow{}
	err := c.store.View(func(r store.Reader) error {
		p, ok := r.Project(key)
		if !ok {
			return unknownProject(key)
		}
		// One evaluation for every Spec.
		e := c.evaluate(r, p)
		for _, s := range r.Specs(key) {
			if in.Title != "" && !strings.Contains(strings.ToLower(s.Title), strings.ToLower(in.Title)) {
				continue
			}
			if !hasAll(s.Labels, s.Tags, in.Labels, in.Tags) {
				continue
			}
			tickets := specTickets(r, key, s.Number)
			summary, done := summarise(tickets)
			if done && !in.Finished {
				continue
			}
			req := requirementView(e, s, tickets)
			if in.Requirements && req == nil {
				continue
			}
			rows = append(rows, SpecRow{
				ID: DisplayID(key, s.Number), Title: s.Title, Labels: orEmpty(s.Labels), Tags: orEmptyMap(s.Tags),
				StateSummary: summary, Finished: done, Updated: s.Updated, Link: linkView(s.Link),
				Kind: specKind(s, req), Need: specNeed(e, tickets), Claimed: specClaimed(tickets),
				Requirement: req, Map: mapView(e, s, tickets),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// specTickets are Spec n's Tickets in claim order.
func specTickets(r store.Reader, key string, n int) []domain.Ticket {
	tickets := slices.DeleteFunc(r.Tickets(key), func(t domain.Ticket) bool { return t.Spec != n })
	slices.SortStableFunc(tickets, claimOrder)
	return tickets
}

// summarise derives a Spec's State summary and whether it is finished.
func summarise(tickets []domain.Ticket) (StateSummary, bool) {
	if len(tickets) == 0 {
		return StateSummary{domain.Draft: 0}, false
	}
	out, done := StateSummary{}, true
	for _, t := range tickets {
		out[t.State]++
		done = done && finished(t.State)
	}
	return out, done
}

// checkTagFilter rejects a Tag filter key no Tag could have.
func checkTagFilter(tags map[string]string) error {
	for k := range tags {
		if !tagKey.MatchString(k) {
			return invalid("tags", "Tag key %q must be lowercase letters, digits, '.', '_' and '-'", k)
		}
	}
	return nil
}
