package core

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/tomoram/code-central/store"
)

// Requirements (Spec code-central-66, ADR 0007). A Requirement is a Spec
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
}

// SubmitRequirementResult is the new Requirement: its Spec and its three
// Stage Tickets, in order.
type SubmitRequirementResult struct {
	Spec   SpecView     `json:"spec"`
	Stages []TicketView `json:"stages"`
}

// stageTitles are each Stage Ticket's title, before the Requirement's.
var stageTitles = map[string]string{
	"grilling": "Grilling",
	"spec":     "Spec",
	"tickets":  "Tickets",
}

// stageDescriptions say what each Stage does, given the Spec's ID.
var stageDescriptions = map[string]string{
	"grilling": "Grill the human on the Requirement in Spec [%[1]s](%[1]s), until every decision, new term and open point is settled.\n",
	"spec":     "Write the Spec for the Requirement in Spec [%[1]s](%[1]s), from its text and the grilling Ticket's Rounds and resolution.\n",
	"tickets":  "Slice the Spec [%[1]s](%[1]s) into implementation Tickets.\n",
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
				Number: n, Title: stageTitles[stage] + ": " + title, State: store.Ready, Spec: specN,
				Labels: []string{StageLabel(stage)}, BlockedBy: blockedBy, Created: now, Updated: now,
				Description: fmt.Sprintf(stageDescriptions[stage], specID),
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
		res.Spec = specView(key, s)
		for _, n := range stageNs {
			t, _ := r.Ticket(key, n)
			res.Stages = append(res.Stages, c.ticketView(r, key, t))
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
