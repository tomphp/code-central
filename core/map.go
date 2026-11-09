package core

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/tomoram/code-central/store"
)

// Maps (Spec code-central-213, ADR 0016). A Map is a Spec labelled
// wayfinder:map: the human submits its text, and Code Central creates the
// Spec and its Charting Ticket in one staged write. Its Map block is
// derived on the read side; nothing in it is stored.

// MapSections are the Map body's level-2 headings, in order. They are
// fixed, because the UI and the guided read parse them as Sections.
var MapSections = []string{"Destination", "Notes", "Decisions so far", "Not yet specified", "Out of scope"}

// SubmitMapInput is submit_map's input: the Map's free text, whose first
// line becomes the Spec's title.
type SubmitMapInput struct {
	Text string `json:"text"`
	// Full asks for the Spec and Charting Ticket in place of their
	// Acknowledgements.
	Full bool `json:"full,omitempty"`
}

// SubmitMapResult is the new Map: its Spec and its Charting Ticket, each
// acknowledged, or with full each the record.
type SubmitMapResult struct {
	Spec     Written[SpecView]   `json:"spec"`
	Charting Written[TicketView] `json:"charting"`
}

// chartingDescription is the Charting Ticket's description, given the
// Map's ID. Its guide is stageguides/map-chart.md.
const chartingDescription = "Chart the Map [%[1]s](%[1]s): settle its destination with the human, fill in its sections and create its first decision Tickets.\n"

// SubmitMap is submit_map(text). It writes as human whoever calls, both
// records or neither.
func (c *Core) SubmitMap(_ context.Context, key string, in SubmitMapInput) (SubmitMapResult, error) {
	title := titleLine(in.Text)
	if title == "" {
		return SubmitMapResult{}, invalid("text", "a Map needs text; its first line becomes the Spec's title")
	}
	var specN int
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		now := c.now()
		specN = w.NextNumber(key)
		return w.PutImport(key, store.Import{
			Specs: []store.Spec{{Number: specN, Title: title, Labels: []string{MapLabel}, Created: now, Updated: now, Body: mapBody(in.Text)}},
			Tickets: []store.Ticket{{
				Number: specN + 1, Title: "Charting: " + title, State: store.Ready, Spec: specN,
				Labels: []string{WayfinderPrefix + "chart"}, Created: now, Updated: now,
				Description: fmt.Sprintf(chartingDescription, DisplayID(key, specN)),
				Moves:       []store.Move{{To: store.Ready, Actor: string(Human), Time: now, Via: "submit_map"}},
			}},
		})
	})
	if err != nil {
		return SubmitMapResult{}, err
	}
	var res SubmitMapResult
	err = c.store.View(func(r store.Reader) error {
		s, _ := r.Spec(key, specN)
		var err error
		if res.Spec, err = acknowledgeCreate(specView(key, store.Spec{}), specView(key, s), in.Full); err != nil {
			return err
		}
		t, _ := r.Ticket(key, specN+1)
		res.Charting, err = acknowledgeCreate(c.ticketView(r, key, store.Ticket{}), c.ticketView(r, key, t), in.Full)
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
	for _, s := range MapSections {
		b.WriteString("\n## " + s + "\n")
	}
	return b.String()
}

// MapView is the Map block on a Spec read, present only when the Spec is
// labelled wayfinder:map. Nothing in it is stored.
type MapView struct {
	// Decided is the Map's Wayfinder Tickets that are Done.
	Decided int `json:"decided"`
	// Total is its Wayfinder Tickets that aren't Cancelled.
	Total int `json:"total"`
	// NeedsYou is its Tickets with a Needs Me reason, Prototypes included.
	NeedsYou int `json:"needs_you"`
	// Guided is whether its guided Tag is on: the Factory then starts its
	// next decision whenever it is idle.
	Guided bool `json:"guided"`
	// CanWriteSpec is whether it has no unfinished Tickets.
	CanWriteSpec bool `json:"can_write_spec"`
}

// GuidedTag is the Tag that marks a Map as guided when its value is
// GuidedOn; a missing Tag or any other value means off.
const (
	GuidedTag = "guided"
	GuidedOn  = "on"
)

// isMap reports whether a Spec's Labels mark it as a Map.
func isMap(labels []string) bool { return slices.Contains(labels, MapLabel) }

// mapView is the Map block for Spec s with these Tickets, or nil when s
// isn't a Map.
func (c *Core) mapView(r store.Reader, key string, s store.Spec, tickets []store.Ticket) *MapView {
	if !isMap(s.Labels) {
		return nil
	}
	v := &MapView{Guided: s.Tags[GuidedTag] == GuidedOn, CanWriteSpec: true}
	for _, t := range tickets {
		if reason, _ := c.reasonOf(r, key, t); reason != "" {
			v.NeedsYou++
		}
		v.CanWriteSpec = v.CanWriteSpec && finished(t.State)
		if !slices.Contains(WayfinderTypes, NodeType(t.Labels)) || t.State == store.Cancelled {
			continue
		}
		v.Total++
		if t.State == store.Done {
			v.Decided++
		}
	}
	return v
}

// NodeType is a Map Ticket's type, from its reserved Label: one of the
// WayfinderTypes, prototype, or ticket for a Ticket with neither.
func NodeType(labels []string) string {
	for _, l := range labels {
		guide, _ := typeGuide(l)
		if guide == PrototypeLabel {
			return guide
		}
		if name, ok := strings.CutPrefix(guide, mapGuidePrefix); ok {
			return name
		}
	}
	return "ticket"
}
