package core

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/store"
)

// A Spec's State is never stored: its StateSummary is derived as a count
// per State of its Tickets, or {Draft: 0} when it has no Tickets. A Spec
// is finished when it has Tickets and all of them are finished.
type StateSummary map[store.State]int

// SpecDetail is get_spec's result: the Spec, its State summary and its
// Tickets as find_tickets rows (finished ones too), in claim order.
type SpecDetail struct {
	SpecView
	StateSummary StateSummary `json:"state_summary"`
	Finished     bool         `json:"finished"`
	Tickets      []TicketRow  `json:"tickets"`
	// Requirement is set only when the Spec is a Requirement.
	Requirement *RequirementView `json:"requirement,omitempty"`
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

// GetSpec is get_spec(spec).
func (c *Core) GetSpec(_ context.Context, key string, ref Ref) (SpecDetail, error) {
	var v SpecDetail
	err := c.store.View(func(r store.Reader) error {
		if _, ok := r.Project(key); !ok {
			return unknownProject(key)
		}
		n, err := ref.number(key, "spec")
		if err != nil {
			return err
		}
		s, ok := r.Spec(key, n)
		if !ok {
			return notFound("there is no Spec %s", DisplayID(key, n))
		}
		tickets := specTickets(r, key, n)
		v = SpecDetail{SpecView: specView(key, s), Tickets: []TicketRow{}}
		v.StateSummary, v.Finished = summarise(tickets)
		for _, t := range tickets {
			v.Tickets = append(v.Tickets, ticketRow(r, key, t))
		}
		v.Requirement = c.requirementView(r, key, tickets)
		return nil
	})
	return v, err
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
