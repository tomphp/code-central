package core

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tomoram/code-central/store"
)

// A Spec's State is never stored: its StateSummary is derived as a count
// per State of its Tickets, or {Draft: 0} when it has no Tickets. A Spec
// is finished when it has Tickets and all of them are finished.
type StateSummary map[store.State]int

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
	Tickets      []TicketRow  `json:"tickets,omitzero"`
	// Requirement is set only when the Spec is a Requirement.
	Requirement *RequirementView `json:"requirement,omitempty"`
}

// GetSpecInput is get_spec's input. With neither Parts nor Sections, the
// read is the full Spec.
type GetSpecInput struct {
	Spec Ref `json:"spec"`
	// Parts are any of body, tickets and edits; all three when empty.
	Parts []string `json:"parts,omitempty"`
	// Sections are heading texts: the body returned is only those Sections.
	Sections []string `json:"sections,omitempty"`
}

// The parts of a Spec a lighter get_spec can ask for. A Ticket has edits
// too, so PartEdits names its edits part as well.
const (
	PartBody    = "body"
	PartTickets = "tickets"
	PartEdits   = "edits"
)

var specParts = []string{PartBody, PartTickets, PartEdits}

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
	// Requirement is set only when the Spec is a Requirement.
	Requirement *RequirementView `json:"requirement,omitempty"`
}

// FindSpecsInput is find_specs' filters; every filter given must match.
type FindSpecsInput struct {
	Title  string            `json:"title,omitempty"`
	Labels []string          `json:"labels,omitempty"`
	Tags   map[string]string `json:"tags,omitempty"`
	// Finished includes finished Specs, which are otherwise left out.
	Finished bool `json:"finished,omitempty"`
	// Requirements keeps only Specs that are Requirements.
	Requirements bool `json:"requirements,omitempty"`
}

// GetSpec is get_spec(spec, parts?, sections?).
func (c *Core) GetSpec(_ context.Context, key string, in GetSpecInput) (SpecDetail, error) {
	parts, err := in.parts()
	if err != nil {
		return SpecDetail{}, err
	}
	var v SpecDetail
	err = c.store.View(func(r store.Reader) error {
		if _, ok := r.Project(key); !ok {
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
		v.Requirement = c.requirementView(r, key, tickets)
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
			for _, t := range tickets {
				v.Tickets = append(v.Tickets, c.ticketRow(r, key, t))
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
// Sections.
func (in GetSpecInput) parts() (map[string]bool, error) {
	asked := in.Parts
	if len(asked) == 0 {
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
		if _, ok := r.Project(key); !ok {
			return unknownProject(key)
		}
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
			req := c.requirementView(r, key, tickets)
			if in.Requirements && req == nil {
				continue
			}
			rows = append(rows, SpecRow{
				ID: DisplayID(key, s.Number), Title: s.Title, Labels: orEmpty(s.Labels), Tags: orEmptyMap(s.Tags),
				StateSummary: summary, Finished: done, Updated: s.Updated, Link: linkView(s.Link),
				Requirement: req,
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
func specTickets(r store.Reader, key string, n int) []store.Ticket {
	tickets := slices.DeleteFunc(r.Tickets(key), func(t store.Ticket) bool { return t.Spec != n })
	slices.SortStableFunc(tickets, claimOrder)
	return tickets
}

// summarise derives a Spec's State summary and whether it is finished.
func summarise(tickets []store.Ticket) (StateSummary, bool) {
	if len(tickets) == 0 {
		return StateSummary{store.Draft: 0}, false
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
