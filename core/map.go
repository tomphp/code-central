package core

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// Maps (Spec code-central-213, ADR 0057). A Map is a Spec labelled
// wayfinder:map: the human submits its text, and Code Central creates the
// Spec and its Charting Ticket in one staged write. Its Map block is
// derived on the read side; nothing in it is stored.

// MapSections are the Map body's level-2 headings, in order. They are
// fixed, because the UI and the guided read parse them as Sections.
func MapSections() []string {
	return []string{"Destination", "Notes", "Decisions so far", "Not yet specified", "Out of scope"}
}

// chartingDescription is the Charting Ticket's description, given the
// Map's ID. Its guide is stageguides/map-chart.md.
const chartingDescription = "Chart the Map [%[1]s](%[1]s): settle its destination with the human, fill in its sections and create its first decision Tickets.\n"

// SubmitMap is submit_map(text). It writes as human whoever calls, both
// records or neither.
func (c *Core) SubmitMap(_ context.Context, key string, in api.SubmitMapInput) (api.SubmitMapResult, error) {
	title := titleLine(in.Text)
	if title == "" {
		return api.SubmitMapResult{}, invalid("text", "a Map needs text; its first line becomes the Spec's title")
	}
	var specN int
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		now := c.now()
		specN = w.NextNumber(key)
		return w.PutImport(key, store.Import{
			Specs: []domain.Spec{{Number: specN, Title: title, Labels: []string{domain.MapLabel}, Created: now, Updated: now, Body: mapBody(in.Text)}},
			Tickets: []domain.Ticket{{
				Number: specN + 1, Title: "Charting: " + title, State: submitState(in.Draft), Spec: specN,
				Labels: []string{domain.WayfinderPrefix + "chart"}, Created: now, Updated: now,
				Description: fmt.Sprintf(chartingDescription, DisplayID(key, specN)),
				Moves:       submitMoves(in.Draft, now, "submit_map"),
			}},
		})
	})
	if err != nil {
		return api.SubmitMapResult{}, err
	}
	var res api.SubmitMapResult
	err = c.store.View(func(r store.Reader) error {
		s, _ := r.Spec(key, specN)
		var err error
		if res.Spec, err = acknowledgeCreate(specView(key, domain.Spec{}), specView(key, s), in.Full); err != nil {
			return err
		}
		t, _ := r.Ticket(key, specN+1, whole...)
		res.Charting, err = acknowledgeCreate(c.ticketView(r, key, domain.Ticket{}), c.ticketView(r, key, t), in.Full)
		return err
	})
	return res, err
}

// mapBody is a new Map's body: the human's text, then the Map's
// sections, empty for Charting to fill.
func mapBody(text string) string {
	var b strings.Builder
	b.WriteString(strings.TrimRight(text, "\n"))
	b.WriteString("\n")
	for _, s := range MapSections() {
		b.WriteString("\n## " + s + "\n")
	}
	return b.String()
}

// isMap reports whether a Spec's Labels mark it as a Map.
func isMap(labels []string) bool { return slices.Contains(labels, domain.MapLabel) }

// mapView is the Map block for Spec s with these Tickets, or nil when s
// isn't a Map, reading their reasons from their Project's evaluation e.
func mapView(e *evaluation, s domain.Spec, tickets []domain.Ticket) *api.MapView {
	if !isMap(s.Labels) {
		return nil
	}
	key := e.p.Key
	id := DisplayID(key, s.Number)
	v := &api.MapView{Guided: s.Tags[domain.GuidedTag] == domain.GuidedOn, CanWriteSpec: true, Requirements: []string{}}
	for _, sp := range e.r.FindSpecs(key, store.SpecFilter{Tag: &store.TagFilter{Key: MapTag, Values: []string{id}}}) {
		v.Requirements = append(v.Requirements, DisplayID(key, sp.Number))
	}
	for _, t := range tickets {
		if reason, _ := e.reason(t); reason != "" {
			v.NeedsYou++
		}
		v.CanWriteSpec = v.CanWriteSpec && finished(t.State)
		if !slices.Contains(api.WayfinderTypes(), NodeTypeOf(t.Labels)) || t.State == domain.Cancelled {
			continue
		}
		v.Total++
		if t.State == domain.Done {
			v.Decided++
		}
	}
	return v
}

// NodeTypeOf is a Map Ticket's type, from its reserved Label: one of the
// WayfinderTypes, prototype, or ticket for a Ticket with neither.
func NodeTypeOf(labels []string) api.NodeType {
	for _, l := range labels {
		guide, _ := api.TypeGuide(l)
		if guide == domain.PrototypeLabel {
			return api.NodePrototype
		}
		if name, ok := strings.CutPrefix(guide, api.MapGuidePrefix); ok {
			return api.NodeType(name)
		}
	}
	return api.NodeTicket
}
