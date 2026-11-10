// The Model Choice (Spec code-central-301, ADR 0024): Code Central works
// out what a Ticket's Tags set; the Factory fills the rest from its own
// --model and --effort, and passes only what is set, so the rest is
// Claude Code's default. It is worked out afresh before every turn.

package factory

import (
	"context"
	"fmt"
	"strings"

	"github.com/tomoram/code-central/core"
)

// turnChoice is the Model Choice a turn is started with, after the
// Factory's fallbacks: empty means Claude Code's default, so no flag.
// modelFrom and effortFrom say where each was set, for a Failure Report.
type turnChoice struct {
	model, effort         string
	modelFrom, effortFrom choiceFrom
}

// sourceFlag is the source of a value the Factory took from its own
// --model or --effort, beside the sources Code Central works out.
const sourceFlag core.ChoiceSource = "factory_flag"

// choiceFrom is where a Model Choice value was set: its source, on a
// Ticket of kind of work kind.
type choiceFrom struct {
	source core.ChoiceSource
	kind   string
}

// withFlags is mc, for a Ticket of kind of work kind, with each value it
// leaves unset taken from the Factory's flag. A nil mc sets nothing.
func (f *factory) withFlags(kind string, mc *core.ModelChoice) turnChoice {
	var set core.ModelChoice
	if mc != nil {
		set = *mc
	}
	tm := turnChoice{model: set.Model.Value, effort: set.Effort.Value,
		modelFrom: choiceFrom{set.Model.Source, kind}, effortFrom: choiceFrom{set.Effort.Source, kind}}
	if tm.model == "" && f.o.model != "" {
		tm.model, tm.modelFrom.source = f.o.model, sourceFlag
	}
	if tm.effort == "" && f.o.effort != "" {
		tm.effort, tm.effortFrom.source = f.o.effort, sourceFlag
	}
	return tm
}

// describe names where the value name ("model" or "effort") was set, and
// says how to change it there to one that claude accepts, as accepted
// puts it.
func (s choiceFrom) describe(name, accepted string) (where, fix string) {
	switch s.source {
	case sourceFlag:
		return fmt.Sprintf("the Factory's `--%s` flag", name),
			fmt.Sprintf("Start the Factory again with a `--%s` that %s, or none, then send the Ticket back.", name, accepted)
	case core.SourceTicketTag:
		return fmt.Sprintf("the Ticket's `%s` Tag", name),
			fmt.Sprintf("Set the Ticket's `%s` Tag to one that %s, or remove it (in the Ticket panel, or with `set_tags` or `remove_tags`), then send the Ticket back.", name, accepted)
	case core.SourceProjectKind:
		return "the Project's Models row for " + s.kind,
			fmt.Sprintf("In Project Options, set the %s in the Models row for %s to one that %s, or to Default, then send the Ticket back.", name, s.kind, accepted)
	case core.SourceProjectDefault:
		return "the Project's default Models row",
			fmt.Sprintf("In Project Options, set the %s in the default Models row to one that %s, or to Default, then send the Ticket back.", name, accepted)
	}
	return "nothing, so Claude Code's default",
		fmt.Sprintf("Set the Ticket's `%s` Tag, or a Models row in Project Options, to one that %s, then send the Ticket back.", name, accepted)
}

// effortAccepted reports whether tm's effort is one claude accepts, and
// if not files a Failure Report naming it and where it was set, so no
// turn starts: claude ignores an effort it doesn't accept and runs at its
// default.
func (f *factory) effortAccepted(ctx context.Context, c claim, dir string, tm turnChoice) bool {
	if f.accepts.takesEffort(tm.effort) {
		return true
	}
	list := strings.Join(f.accepts.levels, ", ")
	where, fix := tm.effortFrom.describe("effort", "claude accepts ("+list+")")
	f.reportFailure(ctx, c, &failure{core.ReportFailureInput{
		WhatBroke:       fmt.Sprintf("The effort `%s`, set by %s, isn't one this Factory's claude accepts (%s), so no turn was started.", tm.effort, where, list),
		Why:             "claude ignores an `--effort` it doesn't accept and runs at its default effort, so the Factory checks a turn's effort against the levels `claude --help` lists before starting it. A value set while other levels were valid stays set when a later report drops its level.",
		WhatToFix:       fix,
		WhereWorkStands: f.whereWorkStands(dir),
	}})
	return false
}

// reportRejectedModel files a Failure Report for t, whose turn claude
// ended with model_not_found, naming the model and where it was set.
func (f *factory) reportRejectedModel(ctx context.Context, t *turn, end turnEnd) bool {
	tm := t.choice
	model := "`" + tm.model + "`"
	if tm.model == "" {
		model = "its default"
	}
	where, fix := tm.modelFrom.describe("model", "claude accepts")
	return f.reportFailure(ctx, t.claim, &failure{core.ReportFailureInput{
		WhatBroke:       fmt.Sprintf("claude rejected the model %s, set by %s (model_not_found).", model, where),
		Why:             "The API answered the turn with model_not_found (404): the model doesn't exist, or this account can't use it. claude says:\n\n" + end.LastText,
		WhatToFix:       fix,
		WhereWorkStands: f.whereWorkStands(t.dir),
	}})
}

// resumeChoice is what c's resumed turn starts with, read afresh, so a Tag
// changed while it waited applies. ok is false when Code Central can't
// say, and the resume waits a poll.
func (f *factory) resumeChoice(ctx context.Context, c claim) (turnChoice, bool) {
	mc, err := f.client.GetModelChoice(ctx, f.project, core.GetModelChoiceInput{Ticket: core.Ref(c.Ticket)})
	if err != nil {
		if ctx.Err() == nil {
			f.claimf(c, "%s: get_model_choice: %v: waiting a poll before resuming", c.Ticket, err)
		}
		return turnChoice{}, false
	}
	return f.withFlags(f.claimInfo(c).stage, &mc), true
}

// reportChoice records on c's Claim what its turn was started with.
func (f *factory) reportChoice(ctx context.Context, c claim, tm turnChoice) {
	in := core.ReportModelChoiceInput{Ticket: core.Ref(c.Ticket), ClaimID: c.ClaimID, Model: tm.model, Effort: tm.effort}
	if _, err := f.client.ReportModelChoice(ctx, f.project, in); err != nil && ctx.Err() == nil {
		f.claimf(c, "%s: report_model_choice: %v", c.Ticket, err)
	}
}
