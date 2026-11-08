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
	// Full asks for the Spec and Stage Tickets in place of their
	// Acknowledgements.
	Full bool `json:"full,omitempty"`
}

// SubmitRequirementResult is the new Requirement: its Spec and its three
// Stage Tickets, in order, each acknowledged, or with full each the record.
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

// SubmitRequirement is submit_requirement(text). It writes as human
// whoever calls, all four records or none.
func (c *Core) SubmitRequirement(_ context.Context, key string, in SubmitRequirementInput) (SubmitRequirementResult, error) {
	title := requirementTitle(in.Text)
	if title == "" {
		return SubmitRequirementResult{}, invalid("text", "a Requirement needs text; its first line becomes the Spec's title")
	}
	var specN int
	var stageNs []int
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		now := c.now()
		specN = w.NextNumber(key)
		im := store.Import{Specs: []store.Spec{{Number: specN, Title: title, Created: now, Updated: now, Body: in.Text}}}
		specID := DisplayID(key, specN)
		for i, stage := range Stages {
			n := specN + 1 + i
			var blockedBy []int
			if i > 0 {
				blockedBy = []int{n - 1}
			}
			im.Tickets = append(im.Tickets, store.Ticket{
				Number: n, Title: stageTickets[stage].title + ": " + title, State: store.Ready, Spec: specN,
				Labels: []string{StageLabel(stage)}, BlockedBy: blockedBy, Created: now, Updated: now,
				Description: fmt.Sprintf(stageTickets[stage].description, specID),
				Moves:       []store.Move{{To: store.Ready, Actor: string(Human), Time: now, Via: "submit_requirement"}},
			})
			stageNs = append(stageNs, n)
		}
		return w.PutImport(key, im)
	})
	if err != nil {
		return SubmitRequirementResult{}, err
	}
	var res SubmitRequirementResult
	err = c.store.View(func(r store.Reader) error {
		s, _ := r.Spec(key, specN)
		var err error
		if res.Spec, err = acknowledgeCreate(specView(key, store.Spec{}), specView(key, s), in.Full); err != nil {
			return err
		}
		for _, n := range stageNs {
			t, _ := r.Ticket(key, n)
			stage, err := acknowledgeCreate(c.ticketView(r, key, store.Ticket{}), c.ticketView(r, key, t), in.Full)
			if err != nil {
				return err
			}
			res.Stages = append(res.Stages, stage)
		}
		return nil
	})
	return res, err
}

// requirementTitle is the text's first non-blank line, without any
// Markdown heading marks; empty when the text is blank.
func requirementTitle(text string) string {
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

// requirementView is the requirement block for a Spec with these Tickets
// (in claim order), or nil when none of them is a Stage Ticket.
func (c *Core) requirementView(r store.Reader, key string, tickets []store.Ticket) *RequirementView {
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
	// claimable is a Ticket a Factory could claim now; one waiting on the
	// human doesn't count.
	claimable := func(t store.Ticket) bool {
		reason, _ := c.reasonOf(r, key, t)
		return t.State == store.Ready && !blocked(r, key, t) && reason == ""
	}
	v := &RequirementView{Stages: []StageView{}}
	current := -1
	for i, t := range stages {
		reason, _ := c.reasonOf(r, key, t)
		v.Stages = append(v.Stages, StageView{Name: StageOf(t.Labels), Ticket: DisplayID(key, t.Number), State: t.State, NeedsMeReason: reason})
		if current < 0 && t.State != store.Done {
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
