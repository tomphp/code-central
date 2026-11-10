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
	SourceTicketTag      ChoiceSource = "ticket_tag"
	SourceProjectKind    ChoiceSource = "project_kind"
	SourceProjectDefault ChoiceSource = "project_default"
	SourceNone           ChoiceSource = "none"
)

// ChoiceValue is one of a Model Choice's values with its source; Value is
// empty when the source is none. Kind names the kind of work whose
// Models row set it, only when the source is project_kind.
type ChoiceValue struct {
	Value  string       `json:"value,omitempty"`
	Source ChoiceSource `json:"source"`
	Kind   string       `json:"kind,omitempty"`
}

// ModelChoice is the model and effort a Ticket's next turn runs with, as
// far as Code Central sets them, each worked out on its own.
type ModelChoice struct {
	Model  ChoiceValue `json:"model"`
	Effort ChoiceValue `json:"effort"`
}

// modelChoice works out the Model Choice of t, on Project p, highest
// tier first: t's Tag, p's Models row for t's kind of work, then p's
// default row. It is worked out afresh for every turn, never kept per
// Claim.
func modelChoice(p store.Project, t store.Ticket) ModelChoice {
	name := GuideName(t.Labels)
	kind := p.Models.Kind(name)
	return ModelChoice{
		Model:  choose(t.Tags[ModelTag], name, kind.Model, p.Models.Default.Model),
		Effort: choose(t.Tags[EffortTag], name, kind.Effort, p.Models.Default.Effort),
	}
}

// choose is the first value set, from the Tag, the row for the kind of
// work, and the default row, with its source.
func choose(tag, kindName, kindValue, def string) ChoiceValue {
	switch {
	case tag != "":
		return ChoiceValue{Value: tag, Source: SourceTicketTag}
	case kindValue != "":
		return ChoiceValue{Value: kindValue, Source: SourceProjectKind, Kind: kindName}
	case def != "":
		return ChoiceValue{Value: def, Source: SourceProjectDefault}
	}
	return ChoiceValue{Source: SourceNone}
}

// effortLevels are the valid effort levels for a Project: its Reported
// Models' levels, or the seeded ones until a report has given some.
func effortLevels(r store.Reader, key string) []string {
	p, _ := r.Project(key)
	return validLevels(p.ReportedModels)
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
		p, ok := r.Project(key)
		if !ok {
			return unknownProject(key)
		}
		t, err := resolveTicket(r, key, "get_model_choice", "ticket", in.Ticket)
		if err != nil {
			return err
		}
		mc = modelChoice(p, t)
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
	for field, v := range map[string]string{"model": model, "effort": effort} {
		if strings.ContainsAny(v, "\r\n") {
			return Written[TicketView]{}, invalid(field, "a %s is single-line", field)
		}
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
