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

	"github.com/tomoram/code-central/domain"
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

// modelChoice works out the Model Choice of t, on Project p, highest
// tier first: t's Tag, p's Models row for t's kind of work, then p's
// default row. It is worked out afresh for every turn, never kept per
// Claim.
func modelChoice(p domain.Project, t domain.Ticket) ModelChoice {
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

// ReportModelChoice is the Factory's report_model_choice: it records the
// model and effort on the live Claim, overwriting the previous turn's.
func (c *Core) ReportModelChoice(_ context.Context, key string, in ReportModelChoiceInput) (Written[TicketView], error) {
	model, effort := strings.TrimSpace(in.Model), strings.TrimSpace(in.Effort)
	for field, v := range map[string]string{"model": model, "effort": effort} {
		if strings.ContainsAny(v, "\r\n") {
			return Written[TicketView]{}, invalid(field, "a %s is single-line", field)
		}
	}
	return c.updateTicket(key, "report_model_choice", in.Ticket, in.Full, func(t *domain.Ticket) (bool, error) {
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
