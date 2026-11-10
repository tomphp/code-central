// The Model Choice (Spec code-central-301, ADR 0024): the model and
// effort a Factory session's turn runs with. Code Central works out the
// tiers the Project and the Ticket set, as plain strings it never
// interprets beyond checking effort; the Factory fills what is left from
// its own flags and turns the result into claude's flags.

package core

import (
	"context"
	"slices"
	"strings"

	"github.com/tomoram/code-central/store"
)

// The Tags on a Ticket that set its Model Choice. On a Spec they are
// ordinary Tags.
const (
	ModelTag  = "model"
	EffortTag = "effort"
)

// seededEffortLevels are the valid effort levels until the Factory has
// reported its claude's, so effort can be set on a fresh store.
var seededEffortLevels = []string{"low", "medium", "high", "xhigh", "max"}

// ChoiceSource says which tier set a Model Choice value.
type ChoiceSource string

// The sources Code Central works out. SourceNone leaves the value to the
// Factory: its flag, or else Claude Code's default.
const (
	SourceTicketTag ChoiceSource = "ticket_tag"
	SourceNone      ChoiceSource = "none"
)

// ChoiceValue is one of a Model Choice's values with its source; Value is
// empty when the source is none.
type ChoiceValue struct {
	Value  string       `json:"value,omitempty"`
	Source ChoiceSource `json:"source"`
}

// ModelChoice is the model and effort a Ticket's next turn runs with, as
// far as Code Central sets them, each worked out on its own.
type ModelChoice struct {
	Model  ChoiceValue `json:"model"`
	Effort ChoiceValue `json:"effort"`
}

// modelChoice works out t's Model Choice, highest tier first. It is
// worked out afresh for every turn, never kept per Claim.
func modelChoice(t store.Ticket) ModelChoice {
	return ModelChoice{Model: choose(t, ModelTag), Effort: choose(t, EffortTag)}
}

// choose is the value of the Tag named tag on t, or the next tier down.
// The Project's Models rows slot in between the Tag and none.
func choose(t store.Ticket, tag string) ChoiceValue {
	if v := t.Tags[tag]; v != "" {
		return ChoiceValue{Value: v, Source: SourceTicketTag}
	}
	return ChoiceValue{Source: SourceNone}
}

// effortLevels are the valid effort levels for a Project.
func effortLevels(store.Reader, string) []string {
	return seededEffortLevels
}

// checkEffort rejects an effort outside the Project's valid levels, in
// field, listing them.
func checkEffort(r store.Reader, key, field, effort string) error {
	levels := effortLevels(r, key)
	if !slices.Contains(levels, effort) {
		return invalid(field, "effort %q isn't a valid level; use one of %s", effort, strings.Join(levels, ", "))
	}
	return nil
}

// GetModelChoiceInput is the Factory's get_model_choice(ticket).
type GetModelChoiceInput struct {
	Ticket Ref `json:"ticket"`
}

// GetModelChoice is the Factory's get_model_choice: the Model Choice for
// the Ticket's next turn, which it reads before every resume.
func (c *Core) GetModelChoice(_ context.Context, key string, in GetModelChoiceInput) (ModelChoice, error) {
	var mc ModelChoice
	err := c.store.View(func(r store.Reader) error {
		if _, ok := r.Project(key); !ok {
			return unknownProject(key)
		}
		t, err := resolveTicket(r, key, "get_model_choice", "ticket", in.Ticket)
		if err != nil {
			return err
		}
		mc = modelChoice(t)
		return nil
	})
	return mc, err
}

// ReportModelChoiceInput is the Factory's report_model_choice(ticket,
// claim_id, model, effort, full?): what a turn was started with, after
// the Factory's fallbacks, empty meaning Claude Code's default.
type ReportModelChoiceInput struct {
	Ticket  Ref    `json:"ticket"`
	ClaimID string `json:"claim_id"`
	Model   string `json:"model,omitempty"`
	Effort  string `json:"effort,omitempty"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// ReportModelChoice is the Factory's report_model_choice: it records the
// model and effort on the live Claim, overwriting the previous turn's.
func (c *Core) ReportModelChoice(_ context.Context, key string, in ReportModelChoiceInput) (Written[TicketView], error) {
	model, effort := strings.TrimSpace(in.Model), strings.TrimSpace(in.Effort)
	if strings.ContainsAny(model, "\r\n") || strings.ContainsAny(effort, "\r\n") {
		return Written[TicketView]{}, invalid("model", "a model and an effort are single-line")
	}
	return c.updateTicket(key, "report_model_choice", in.Ticket, in.Full, func(t *store.Ticket) (bool, error) {
		cl, err := holder(t, key, in.ClaimID)
		if err != nil {
			return false, err
		}
		if cl.Model == model && cl.Effort == effort {
			return false, nil
		}
		cl.Model, cl.Effort = model, effort
		return true, nil
	})
}
