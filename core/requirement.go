package core

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/tomoram/code-central/store"
)

// Requirements (Spec 66, ADR 0007). A Requirement is a Spec
// with Stage Tickets: the human submits its text, and Code Central
// creates the Spec and its three Stages in one staged write.

// StagePrefix is the reserved Label prefix that marks a Stage Ticket.
const StagePrefix = "stage:"

// Stages are the three Stages in order, each named as its Label's suffix.
var Stages = []string{"grilling", "spec", "tickets"}

// StageLabel is the Label that marks a Ticket as the named Stage.
func StageLabel(stage string) string { return StagePrefix + stage }

// checkStageLabels rejects a reserved stage:* Label that names no Stage.
func checkStageLabels(field string, labels []string) error {
	for _, l := range labels {
		name, ok := strings.CutPrefix(l, StagePrefix)
		if ok && !slices.Contains(Stages, name) {
			return invalid(field, "%q uses the reserved %q prefix; the only Stage Labels are stage:grilling, stage:spec and stage:tickets", l, StagePrefix)
		}
	}
	return nil
}

// SubmitRequirementInput is submit_requirement's input: the Requirement's
// free text, whose first line becomes the Spec's title.
type SubmitRequirementInput struct {
	Text string `json:"text"`
	// Map is the finished Map the Requirement is written from, if any
	// (Write the Spec): its Decisions take the grilling Stage's place.
	Map Ref `json:"map,omitempty"`
	// SkipGrilling is Straight to Spec: no grilling Stage, so the spec
	// Stage works from the text alone. It can't be given with Map.
	SkipGrilling bool `json:"skip_grilling,omitempty"`
	// Full asks for the Spec and Stage Tickets in place of their
	// Acknowledgements.
	Full bool `json:"full,omitempty"`
}

// SubmitRequirementResult is the new Requirement: its Spec and its Stage
// Tickets (three, or two when made from a Map), in order, each
// acknowledged, or with full each the record.
type SubmitRequirementResult struct {
	Spec   Written[SpecView]     `json:"spec"`
	Stages []Written[TicketView] `json:"stages"`
}

// stageTickets are what submit_requirement writes for each Stage: its
// Ticket's title, before the Requirement's, and its description, given
// the Spec's ID. Each Stage's guide is stageguides/<name>.md.
var stageTickets = map[string]struct{ title, description string }{
	"grilling": {"Grilling", "Grill the human on the Requirement in Spec [%[1]s](%[1]s), until every decision, new term and open point is settled.\n"},
	"spec":     {"Spec", "Write the Spec for the Requirement in Spec [%[1]s](%[1]s), from its text and the grilling Ticket's Rounds and resolution.\n"},
	"tickets":  {"Tickets", "Slice the Spec [%[1]s](%[1]s) into implementation Tickets.\n"},
}

// straightSpecDescription is the spec Stage's description on a Requirement
// with no grilling Stage and no Map, given the Spec's ID.
const straightSpecDescription = "Write the Spec for the Requirement in Spec [%[1]s](%[1]s), from its text.\n"

// MapTag is the Tag on a Requirement made from a Map, whose value is the
// Map's ID.
const MapTag = "map"

// mapSpecDescription is the spec Stage's description on a Requirement made
// from a Map, given the Spec's ID and the Map's.
const mapSpecDescription = "Write the Spec for the Requirement in Spec [%[1]s](%[1]s), from its text and the Map [%[2]s](%[2]s): its Decisions so far and the resolution comments they link.\n"

// SubmitRequirement is submit_requirement(text, map?, skip_grilling?). It writes as human
// whoever calls, all its records or none. With map, the Requirement has
// only the spec and tickets Stages, and its map Tag names the Map.
func (c *Core) SubmitRequirement(_ context.Context, key string, in SubmitRequirementInput) (SubmitRequirementResult, error) {
	title := titleLine(in.Text)
	if title == "" {
		return SubmitRequirementResult{}, invalid("text", "a Requirement needs text; its first line becomes the Spec's title")
	}
	if in.SkipGrilling && in.Map != "" {
		return SubmitRequirementResult{}, invalid("skip_grilling", "skip_grilling can't be given with map: a Requirement written from a Map has no grilling Stage already")
	}
	var res SubmitRequirementResult
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		stages, mapID := Stages, ""
		if in.SkipGrilling {
			stages = Stages[1:]
		}
		if in.Map != "" {
			m, err := finishedMap(w, key, in.Map)
			if err != nil {
				return err
			}
			stages, mapID = Stages[1:], DisplayID(key, m.Number)
		}
		sp := store.Spec{Title: title, Body: in.Text}
		if mapID != "" {
			sp.Tags = map[string]string{MapTag: mapID}
		}
		im, specN, stageNs := c.requirementImport(w, key, sp, stages, mapID)
		if err := w.PutImport(key, im); err != nil {
			return err
		}
		var err error
		res, err = c.requirementResult(w, key, specN, stageNs, in.Full)
		return err
	})
	return res, err
}

// requirementImport is the staged write of Spec sp, given its title,
// body, Tags and Link, as a Requirement with these Stages, with the
// Spec's number and its Stage Tickets'. mapID is the Map it is made
// from, if any.
func (c *Core) requirementImport(r store.Reader, key string, sp store.Spec, stages []string, mapID string) (store.Import, int, []int) {
	now := c.now()
	specN := r.NextNumber(key)
	sp.Number, sp.Created, sp.Updated = specN, now, now
	im := store.Import{Specs: []store.Spec{sp}}
	specID := DisplayID(key, specN)
	var stageNs []int
	for i, stage := range stages {
		n := specN + 1 + i
		var blockedBy []int
		if i > 0 {
			blockedBy = []int{n - 1}
		}
		description := fmt.Sprintf(stageTickets[stage].description, specID)
		if stage == "spec" && mapID != "" {
			description = fmt.Sprintf(mapSpecDescription, specID, mapID)
		} else if stage == "spec" && stages[0] == "spec" {
			description = fmt.Sprintf(straightSpecDescription, specID)
		}
		im.Tickets = append(im.Tickets, store.Ticket{
			Number: n, Title: stageTickets[stage].title + ": " + sp.Title, State: store.Ready, Spec: specN,
			Labels: []string{StageLabel(stage)}, BlockedBy: blockedBy, Created: now, Updated: now,
			Description: description,
			Moves:       []store.Move{{To: store.Ready, Actor: string(Human), Time: now, Via: "submit_requirement"}},
		})
		stageNs = append(stageNs, n)
	}
	return im, specN, stageNs
}

// requirementResult acknowledges the Requirement requirementImport wrote,
// or with full gives its records.
func (c *Core) requirementResult(r store.Reader, key string, specN int, stageNs []int, full bool) (SubmitRequirementResult, error) {
	var res SubmitRequirementResult
	s, _ := r.Spec(key, specN)
	var err error
	if res.Spec, err = acknowledgeCreate(specView(key, store.Spec{}), specView(key, s), full); err != nil {
		return res, err
	}
	for _, n := range stageNs {
		t, _ := r.Ticket(key, n)
		stage, err := acknowledgeCreate(c.ticketView(r, key, store.Ticket{}), c.ticketView(r, key, t), full)
		if err != nil {
			return res, err
		}
		res.Stages = append(res.Stages, stage)
	}
	return res, nil
}

// finishedMap reads the Map ref names as submit_requirement's map, which
// must be labelled wayfinder:map and have no unfinished Tickets.
func finishedMap(r store.Reader, key string, ref Ref) (store.Spec, error) {
	m, err := resolveSpec(r, key, "submit_requirement", "map", ref)
	if err != nil {
		return store.Spec{}, err
	}
	id := DisplayID(key, m.Number)
	if !isMap(m.Labels) {
		return store.Spec{}, invalid("map", "%s isn't a Map: a Map is a Spec labelled %s", id, MapLabel)
	}
	var open []string
	for _, t := range specTickets(r, key, m.Number) {
		if !finished(t.State) {
			open = append(open, fmt.Sprintf("%s (%s)", DisplayID(key, t.Number), t.State))
		}
	}
	if len(open) > 0 {
		return store.Spec{}, &Error{Code: ActionNotAllowed, Message: fmt.Sprintf(
			"Map %s still has unfinished Tickets, so its Spec can't be written until every one is Done or Cancelled: %s",
			id, strings.Join(open, ", "))}
	}
	return m, nil
}

// titleLine is the text's first non-blank line, without any
// Markdown heading marks; empty when the text is blank.
func titleLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#"))
		if line != "" {
			return line
		}
	}
	return ""
}

// RequirementView is the requirement block on a Spec read, present only
// when the Spec is a Requirement: it has at least one Stage Ticket.
// Nothing in it is stored.
type RequirementView struct {
	// Map is the Map the Requirement was made from, from its map Tag, set
	// only when it has no grilling Stage.
	Map string `json:"map,omitempty"`
	// Stages are the Stage Tickets in Stage order.
	Stages []StageView `json:"stages"`
	// Build is how far the implementation Tickets have got.
	Build BuildProgress `json:"build"`
	// WaitingForFactory is whether the next Ticket can be claimed but
	// nothing has claimed it.
	WaitingForFactory bool `json:"waiting_for_factory"`
}

// StageView is one Stage Ticket: its Stage name, Ticket ID, State and
// Needs Me reason, if any.
type StageView struct {
	Name          string        `json:"name"`
	Ticket        string        `json:"ticket"`
	State         store.State   `json:"state"`
	NeedsMeReason NeedsMeReason `json:"needs_me_reason,omitempty"`
}

// BuildProgress is the implementation Tickets Done over their total,
// leaving out Cancelled ones.
type BuildProgress struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

// StageOf is the Stage a Ticket's Labels mark it as, or "" for an
// implementation Ticket.
func StageOf(labels []string) string {
	for _, l := range labels {
		if name, ok := strings.CutPrefix(l, StagePrefix); ok && slices.Contains(Stages, name) {
			return name
		}
	}
	return ""
}

// requirementView is the requirement block for Spec s with these Tickets
// (in claim order), or nil when none of them is a Stage Ticket.
func (c *Core) requirementView(r store.Reader, key string, s store.Spec, tickets []store.Ticket) *RequirementView {
	var stages, build []store.Ticket
	for _, t := range tickets {
		if StageOf(t.Labels) != "" {
			stages = append(stages, t)
		} else {
			build = append(build, t)
		}
	}
	if len(stages) == 0 {
		return nil
	}
	slices.SortStableFunc(stages, func(a, b store.Ticket) int {
		return slices.Index(Stages, StageOf(a.Labels)) - slices.Index(Stages, StageOf(b.Labels))
	})
	rule := c.blockRule(r, key)
	// claimable is a Ticket a Factory could claim now; one waiting on the
	// human doesn't count.
	claimable := func(t store.Ticket) bool {
		reason, _ := c.reasonOf(r, key, t)
		return t.State == store.Ready && !rule.blocked(t) && reason == ""
	}
	v := &RequirementView{Stages: []StageView{}}
	if StageOf(stages[0].Labels) != "grilling" {
		v.Map = s.Tags[MapTag]
	}
	current := -1
	for i, t := range stages {
		reason, _ := c.reasonOf(r, key, t)
		v.Stages = append(v.Stages, StageView{Name: StageOf(t.Labels), Ticket: DisplayID(key, t.Number), State: t.State, NeedsMeReason: reason})
		if current < 0 && t.State != store.Done && t.State != store.Cancelled {
			current = i
		}
	}
	anyClaimable := false
	for _, t := range build {
		if t.State == store.Cancelled {
			continue
		}
		v.Build.Total++
		if t.State == store.Done {
			v.Build.Done++
		}
		anyClaimable = anyClaimable || claimable(t)
	}
	if current >= 0 {
		v.WaitingForFactory = claimable(stages[current])
	} else {
		v.WaitingForFactory = anyClaimable && !slices.ContainsFunc(tickets, func(t store.Ticket) bool {
			_, held := liveClaim(t)
			return held
		})
	}
	return v
}
