package core

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

var specParts = []string{api.PartBody, api.PartTickets, api.PartEdits, api.PartOutline}

// specKind is s's kind, given its Requirement view: a Requirement, else a
// Map, else Long-lived, else a Spec.
func specKind(s domain.Spec, req *api.RequirementView) api.SpecKind {
	switch {
	case req != nil:
		return api.SpecKindRequirement
	case isMap(s.Labels):
		return api.SpecKindMap
	case slices.Contains(s.Labels, domain.LongLivedLabel):
		return api.SpecKindLongLived
	}
	return api.SpecKindSpec
}

// specNeed is the reason of the first of tickets, in claim order, that
// waits on the human, read from e as Needs Me reads it; "" when none does.
func specNeed(e *evaluation, tickets []domain.Ticket) api.NeedsMeReason {
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

// GetSpec is get_spec(spec, parts?, sections?).
func (c *Core) GetSpec(_ context.Context, key string, in api.GetSpecInput) (api.SpecDetail, error) {
	parts, err := specReadParts(in)
	if err != nil {
		return api.SpecDetail{}, err
	}
	var v api.SpecDetail
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
		tickets := specTickets(r, key, n, store.Moves)
		sv := specView(key, s)
		v = api.SpecDetail{
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
		if specReadLighter(in) {
			v.Outline = Outline(s.Body)
		}
		if parts[api.PartBody] {
			body := sv.Body
			if len(in.Sections) > 0 {
				var unmatched []string
				if body, unmatched = Sections(s.Body, in.Sections); unmatched != nil {
					return noSuchSections(unmatched, v.Outline)
				}
			}
			v.Body = &body
		}
		if parts[api.PartEdits] {
			v.Edits = sv.Edits
		}
		if parts[api.PartTickets] {
			v.Tickets = []api.TicketRow{}
			onMap, withBlockers := isMap(s.Labels), v.Kind != api.SpecKindLongLived
			for _, t := range tickets {
				v.Tickets = append(v.Tickets, c.ticketRow(r, e, t, onMap, withBlockers))
			}
		}
		return nil
	})
	return v, err
}

// specReadLighter reports whether the read asks for less than the full Spec, so
// returns the Outline.
func specReadLighter(in api.GetSpecInput) bool { return len(in.Parts) > 0 || len(in.Sections) > 0 }

// parts is the set of parts the read returns, checking them against
// Sections. Sections without Parts read only the body.
func specReadParts(in api.GetSpecInput) (map[string]bool, error) {
	asked := in.Parts
	switch {
	case len(asked) > 0:
	case len(in.Sections) > 0:
		asked = []string{api.PartBody}
	default:
		asked = specParts
	}
	out, err := partSet(asked, specParts)
	if err != nil {
		return nil, err
	}
	if len(in.Sections) > 0 && !out[api.PartBody] {
		return nil, invalid("sections", "sections cut the body, so parts must include body")
	}
	return out, nil
}

// noSuchSections rejects Section names that match no heading, listing the
// headings there are.
func noSuchSections(names []string, outline []api.Heading) *api.Error {
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
func (c *Core) FindSpecs(_ context.Context, key string, in api.FindSpecsInput) ([]api.SpecRow, error) {
	if err := checkTagFilter(in.Tags); err != nil {
		return nil, err
	}
	rows := []api.SpecRow{}
	err := c.store.View(func(r store.Reader) error {
		p, ok := r.Project(key)
		if !ok {
			return unknownProject(key)
		}
		// One evaluation for every Spec.
		e := c.evaluate(r, p)
		for _, s := range r.FindSpecs(key, store.SpecFilter{}) {
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
			rows = append(rows, api.SpecRow{
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

// specTickets are Spec n's Tickets in claim order, with the History parts
// named.
func specTickets(r store.Reader, key string, n int, parts ...store.Part) []domain.Ticket {
	return r.FindTickets(key, store.TicketFilter{Spec: []int{n}, Parts: parts, Order: store.ByClaim})
}

// summarise derives a Spec's State summary and whether it is finished.
func summarise(tickets []domain.Ticket) (api.StateSummary, bool) {
	if len(tickets) == 0 {
		return api.StateSummary{domain.Draft: 0}, false
	}
	out, done := api.StateSummary{}, true
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
