// The Model Choice (Spec code-central-301, ADR 0024): Code Central works
// out what a Ticket's Tags set; the Factory fills the rest from its own
// --model and --effort, and passes only what is set, so the rest is
// Claude Code's default. It is worked out afresh before every turn.

package factory

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/tomoram/code-central/core"
)

// turnChoice is the Model Choice a turn is started with, after the
// Factory's fallbacks: empty means Claude Code's default, so no flag.
// modelFrom and effortFrom say where each was set, for a Failure Report.
type turnChoice struct {
	model, effort         string
	modelFrom, effortFrom setting
}

// setting is where a Model Choice value was set: by Code Central's
// source, on the Ticket of kind kind, or else by the Factory's flag.
type setting struct {
	source core.ChoiceSource
	kind   string
	flag   bool
}

// withFlags is mc, for c, with each value it leaves unset taken from the
// Factory's flag. A nil mc sets nothing.
func (f *factory) withFlags(c claim, mc *core.ModelChoice) turnChoice {
	var set core.ModelChoice
	if mc != nil {
		set = *mc
	}
	kind := f.claimInfo(c).stage
	tm := turnChoice{model: set.Model.Value, effort: set.Effort.Value,
		modelFrom: setting{source: set.Model.Source, kind: kind}, effortFrom: setting{source: set.Effort.Source, kind: kind}}
	if tm.model == "" && f.o.model != "" {
		tm.model, tm.modelFrom.flag = f.o.model, true
	}
	if tm.effort == "" && f.o.effort != "" {
		tm.effort, tm.effortFrom.flag = f.o.effort, true
	}
	return tm
}

// where names where the value name ("model" or "effort") was set.
func (s setting) where(name string) string {
	switch {
	case s.flag:
		return fmt.Sprintf("the Factory's `--%s` flag", name)
	case s.source == core.SourceTicketTag:
		return fmt.Sprintf("the Ticket's `%s` Tag", name)
	case s.source == core.SourceProjectKind:
		return "the Project's Models row for " + s.kind
	case s.source == core.SourceProjectDefault:
		return "the Project's default Models row"
	}
	return "nothing, so Claude Code's default"
}

// change says how to change the value name, set where s says, to one
// that is as to says.
func (s setting) change(name, to string) string {
	switch {
	case s.flag:
		return fmt.Sprintf("Start the Factory again with a `--%s` that %s, or none, then send the Ticket back.", name, to)
	case s.source == core.SourceTicketTag:
		return fmt.Sprintf("Set the Ticket's `%s` Tag to one that %s, or remove it (in the Ticket panel, or with `set_tags` or `remove_tags`), then send the Ticket back.", name, to)
	case s.source == core.SourceProjectKind:
		return fmt.Sprintf("In Project Options, set the %s in the Models row for %s to one that %s, or to Default, then send the Ticket back.", name, s.kind, to)
	case s.source == core.SourceProjectDefault:
		return fmt.Sprintf("In Project Options, set the %s in the default Models row to one that %s, or to Default, then send the Ticket back.", name, to)
	}
	return fmt.Sprintf("Set the Ticket's `%s` Tag, or a Models row in Project Options, to one that %s, then send the Ticket back.", name, to)
}

// effortAccepted reports whether tm's effort is one claude accepts, and
// if not files a Failure Report naming it and where it was set, so no
// turn starts: claude ignores an effort it doesn't accept and runs at its
// default. Without levels from claude --help it can't say, so it takes
// any.
func (f *factory) effortAccepted(ctx context.Context, c claim, dir string, tm turnChoice) bool {
	levels := f.accepts.levels
	if tm.effort == "" || len(levels) == 0 || slices.Contains(levels, tm.effort) {
		return true
	}
	list := strings.Join(levels, ", ")
	f.reportFailure(ctx, c, &failure{core.ReportFailureInput{
		WhatBroke:       fmt.Sprintf("The effort `%s`, set by %s, isn't one this Factory's claude accepts (%s), so no turn was started.", tm.effort, tm.effortFrom.where("effort"), list),
		Why:             "claude ignores an `--effort` it doesn't accept and runs at its default effort, so the Factory checks a turn's effort against the levels `claude --help` lists before starting it. A value set while other levels were valid stays set when a later report drops its level.",
		WhatToFix:       tm.effortFrom.change("effort", "claude accepts ("+list+")"),
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
	return f.reportFailure(ctx, t.claim, &failure{core.ReportFailureInput{
		WhatBroke:       fmt.Sprintf("claude rejected the model %s, set by %s (model_not_found).", model, tm.modelFrom.where("model")),
		Why:             "The API answered the turn with model_not_found (404): the model doesn't exist, or this account can't use it. claude says:\n\n" + end.LastText,
		WhatToFix:       tm.modelFrom.change("model", "claude accepts"),
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
	return f.withFlags(c, &mc), true
}

// reportChoice records on c's Claim what its turn was started with.
func (f *factory) reportChoice(ctx context.Context, c claim, tm turnChoice) {
	in := core.ReportModelChoiceInput{Ticket: core.Ref(c.Ticket), ClaimID: c.ClaimID, Model: tm.model, Effort: tm.effort}
	if _, err := f.client.ReportModelChoice(ctx, f.project, in); err != nil && ctx.Err() == nil {
		f.claimf(c, "%s: report_model_choice: %v", c.Ticket, err)
	}
}
