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

// Requirements (Spec 66, ADR 0007). A Requirement is a Spec
// with Stage Tickets: the human submits its text, and Code Central
// creates the Spec and its three Stages in one staged write.

// StageLabel is the Label that marks a Ticket as the named Stage.
func StageLabel(stage domain.StageName) string { return domain.StagePrefix + string(stage) }

// checkStageLabels rejects a reserved stage:* Label that names no Stage.
func checkStageLabels(field string, labels []string) error {
	for _, l := range labels {
		name, ok := strings.CutPrefix(l, domain.StagePrefix)
		if ok && !slices.Contains(domain.Stages(), domain.StageName(name)) {
			return invalid(field, "%q uses the reserved %q prefix; the only Stage Labels are stage:grilling, stage:spec and stage:tickets", l, domain.StagePrefix)
		}
	}
	return nil
}

// stageTickets are what submit_requirement writes for each Stage: its
// Ticket's title, before the Requirement's, and its description, given
// the Spec's ID. Each Stage's guide is stageguides/<name>.md.
var stageTickets = map[domain.StageName]struct{ title, description string }{
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
func (c *Core) SubmitRequirement(_ context.Context, key string, in api.SubmitRequirementInput) (api.SubmitRequirementResult, error) {
	title := titleLine(in.Text)
	if title == "" {
		return api.SubmitRequirementResult{}, invalid("text", "a Requirement needs text; its first line becomes the Spec's title")
	}
	if in.SkipGrilling && in.Map != "" {
		return api.SubmitRequirementResult{}, invalid("skip_grilling", "skip_grilling can't be given with map: a Requirement written from a Map has no grilling Stage already")
	}
	if in.Draft && in.Map != "" {
		return api.SubmitRequirementResult{}, invalid("draft", "Write the Spec, a Requirement made from a Map, is saved as Ready only")
	}
	var res api.SubmitRequirementResult
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		stages, mapID := domain.Stages(), ""
		if in.SkipGrilling {
			stages = domain.Stages()[1:]
		}
		if in.Map != "" {
			m, err := finishedMap(w, key, in.Map)
			if err != nil {
				return err
			}
			stages, mapID = domain.Stages()[1:], DisplayID(key, m.Number)
		}
		sp := domain.Spec{Title: title, Body: in.Text}
		if mapID != "" {
			sp.Tags = map[string]string{MapTag: mapID}
		}
		im, specN, stageNs := c.requirementImport(w, key, sp, stages, mapID, in.Draft)
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
func (c *Core) requirementImport(r store.Reader, key string, sp domain.Spec, stages []domain.StageName, mapID string, draft bool) (store.Import, int, []int) {
	now := c.now()
	specN := r.NextNumber(key)
	sp.Number, sp.Created, sp.Updated = specN, now, now
	im := store.Import{Specs: []domain.Spec{sp}}
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
		im.Tickets = append(im.Tickets, domain.Ticket{
			Number: n, Title: stageTickets[stage].title + ": " + sp.Title, State: submitState(draft), Spec: specN,
			Labels: []string{StageLabel(stage)}, BlockedBy: blockedBy, Created: now, Updated: now,
			Description: description,
			Moves:       submitMoves(draft, now, "submit_requirement"),
		})
		stageNs = append(stageNs, n)
	}
	return im, specN, stageNs
}

// requirementResult acknowledges the Requirement requirementImport wrote,
// or with full gives its records.
func (c *Core) requirementResult(r store.Reader, key string, specN int, stageNs []int, full bool) (api.SubmitRequirementResult, error) {
	var res api.SubmitRequirementResult
	s, _ := r.Spec(key, specN)
	var err error
	if res.Spec, err = acknowledgeCreate(specView(key, domain.Spec{}), specView(key, s), full); err != nil {
		return res, err
	}
	for _, n := range stageNs {
		t, _ := r.Ticket(key, n, whole...)
		stage, err := acknowledgeCreate(c.ticketView(r, key, domain.Ticket{}), c.ticketView(r, key, t), full)
		if err != nil {
			return res, err
		}
		res.Stages = append(res.Stages, stage)
	}
	return res, nil
}

// finishedMap reads the Map ref names as submit_requirement's map, which
// must be labelled wayfinder:map and have no unfinished Tickets.
func finishedMap(r store.Reader, key string, ref api.Ref) (domain.Spec, error) {
	m, err := resolveSpec(r, key, "submit_requirement", "map", ref)
	if err != nil {
		return domain.Spec{}, err
	}
	id := DisplayID(key, m.Number)
	if !isMap(m.Labels) {
		return domain.Spec{}, invalid("map", "%s isn't a Map: a Map is a Spec labelled %s", id, MapLabel)
	}
	var open []string
	for _, t := range specTickets(r, key, m.Number) {
		if !finished(t.State) {
			open = append(open, fmt.Sprintf("%s (%s)", DisplayID(key, t.Number), t.State))
		}
	}
	if len(open) > 0 {
		return domain.Spec{}, &api.Error{Code: api.ActionNotAllowed, Message: fmt.Sprintf(
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

// StageOf is the Stage a Ticket's Labels mark it as, or "" for an
// implementation Ticket.
func StageOf(labels []string) domain.StageName {
	for _, l := range labels {
		if name, ok := strings.CutPrefix(l, domain.StagePrefix); ok && slices.Contains(domain.Stages(), domain.StageName(name)) {
			return domain.StageName(name)
		}
	}
	return ""
}

// requirementView is the requirement block for Spec s with these Tickets
// (in claim order), or nil when none of them is a Stage Ticket, reading
// their reasons from their Project's evaluation e.
func requirementView(e *evaluation, s domain.Spec, tickets []domain.Ticket) *api.RequirementView {
	var stages, build []domain.Ticket
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
	slices.SortStableFunc(stages, func(a, b domain.Ticket) int {
		return slices.Index(domain.Stages(), StageOf(a.Labels)) - slices.Index(domain.Stages(), StageOf(b.Labels))
	})
	// claimable is a Ticket a Factory could claim now; one waiting on the
	// human doesn't count.
	claimable := func(t domain.Ticket) bool {
		reason, _ := e.reason(t)
		return t.State == domain.Ready && !e.rule.blocked(t) && reason == ""
	}
	v := &api.RequirementView{Stages: []api.StageView{}}
	if StageOf(stages[0].Labels) != domain.StageGrilling {
		v.Map = s.Tags[MapTag]
	}
	firstOpen := -1
	for i, t := range stages {
		reason, _ := e.reason(t)
		v.Stages = append(v.Stages, api.StageView{Name: StageOf(t.Labels), Ticket: DisplayID(e.p.Key, t.Number), State: t.State, NeedsMeReason: reason})
		if firstOpen < 0 && !finished(t.State) {
			firstOpen = i
		}
	}
	anyClaimable := false
	for _, t := range build {
		if t.State == domain.Cancelled {
			continue
		}
		v.Build.Total++
		if t.State == domain.Done {
			v.Build.Done++
		}
		anyClaimable = anyClaimable || claimable(t)
	}
	if firstOpen >= 0 {
		v.WaitingForFactory = claimable(stages[firstOpen])
	} else {
		v.WaitingForFactory = anyClaimable && !slices.ContainsFunc(tickets, func(t domain.Ticket) bool {
			_, claimed := liveClaim(t)
			return claimed
		})
	}
	requirementSetStrip(v, e, stages, build, firstOpen)
	return v
}

// requirementSetStrip sets v's Stage strip, its current segment, needs and default
// Ticket from its Stage Tickets (in Stage order) and Build Tickets (in
// claim order), firstOpen being the index of the first open Stage or -1,
// and v's Map, Build and WaitingForFactory already set.
func requirementSetStrip(v *api.RequirementView, e *evaluation, stages, build []domain.Ticket, firstOpen int) {
	id := func(t domain.Ticket) string { return DisplayID(e.p.Key, t.Number) }
	v.Segments, v.Needs = []api.Segment{}, []api.RequirementNeed{}
	// A Requirement made from a Map has no grilling Stage: the Map did that.
	// One with neither was submitted Straight to Spec (ADR 0036).
	if v.Map != "" {
		v.Segments = append(v.Segments, api.Segment{Stage: api.SegmentStage(domain.StageGrilling), Progress: api.ProgressDone, Map: v.Map})
	} else if StageOf(stages[0].Labels) != domain.StageGrilling {
		v.Segments = append(v.Segments, api.Segment{Stage: api.SegmentStage(domain.StageGrilling), Progress: api.ProgressSkipped})
	}
	for i, t := range stages {
		reason, _ := e.reason(t)
		seg := api.Segment{Stage: api.SegmentStage(StageOf(t.Labels)), Ticket: id(t), Reason: reason}
		switch {
		case t.State == domain.Done:
			seg.Progress = api.ProgressDone
		case t.State == domain.Cancelled:
			seg.Progress = api.ProgressSkipped
		case reason != "":
			seg.Progress = api.ProgressNeeds
		case i == firstOpen && v.WaitingForFactory:
			seg.Progress = api.ProgressQueued
		case t.State == domain.Ready || t.State == domain.Draft:
			seg.Progress = api.ProgressWaiting
		default:
			seg.Progress = api.ProgressRunning
		}
		v.Segments = append(v.Segments, seg)
		if reason != "" {
			v.Needs = append(v.Needs, api.RequirementNeed{Ticket: seg.Ticket, Reason: reason, Stage: StageOf(t.Labels)})
		}
	}
	stageNeeds := len(v.Needs)
	for _, t := range build {
		if reason, _ := e.reason(t); reason != "" {
			v.Needs = append(v.Needs, api.RequirementNeed{Ticket: id(t), Reason: reason})
		}
	}
	buildNeeds := v.Needs[stageNeeds:]
	seg := api.Segment{Stage: api.SegmentBuild}
	inBuild := firstOpen < 0
	switch {
	case v.Build.Total > 0 && v.Build.Done == v.Build.Total && len(buildNeeds) == 0:
		seg.Progress = api.ProgressDone
	case len(buildNeeds) > 0:
		seg.Progress, seg.Reason = api.ProgressNeeds, buildNeeds[0].Reason
	case inBuild && v.WaitingForFactory:
		seg.Progress = api.ProgressQueued
	case inBuild && v.Build.Total > 0:
		seg.Progress = api.ProgressRunning
	default:
		seg.Progress = api.ProgressWaiting
	}
	v.Segments = append(v.Segments, seg)

	at := slices.IndexFunc(v.Segments, func(s api.Segment) bool { return s.Progress != api.ProgressDone && s.Progress != api.ProgressSkipped })
	if at < 0 {
		at = len(v.Segments) - 1
	}
	v.Current = v.Segments[at].Stage
	v.DefaultTicket = requirementDefaultTicket(v, v.Segments[at], build, id)
}

// requirementDefaultTicket is the Ticket the Requirement opens on, given its current
// segment and its Build Tickets: the first of its needs, else the current
// segment's, else the first unfinished Build Ticket, else the last of
// them, else the last segment's with one; "" when there is none.
func requirementDefaultTicket(v *api.RequirementView, current api.Segment, build []domain.Ticket, id func(domain.Ticket) string) string {
	if len(v.Needs) > 0 {
		return v.Needs[0].Ticket
	}
	if current.Ticket != "" {
		return current.Ticket
	}
	if i := slices.IndexFunc(build, func(t domain.Ticket) bool { return !finished(t.State) }); i >= 0 {
		return id(build[i])
	}
	if len(build) > 0 {
		return id(build[len(build)-1])
	}
	for _, s := range slices.Backward(v.Segments) {
		if s.Ticket != "" {
			return s.Ticket
		}
	}
	return ""
}
