// The Model Choice (Spec code-central-301, ADR 0024): Code Central works
// out what a Ticket's Tags set; the Factory fills the rest from its own
// --model and --effort, and passes only what is set, so the rest is
// Claude Code's default. It is worked out afresh before every turn.

package factory

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// turnModel is what a turn is started with, after the Factory's
// fallbacks: empty means Claude Code's default, so no flag.
type turnModel struct {
	model, effort string
}

// withFlags is mc with each value it leaves unset taken from the
// Factory's flag. A nil mc sets nothing.
func (f *factory) withFlags(mc *core.ModelChoice) turnModel {
	var tm turnModel
	if mc != nil {
		tm = turnModel{model: mc.Model.Value, effort: mc.Effort.Value}
	}
	if tm.model == "" {
		tm.model = f.o.model
	}
	if tm.effort == "" {
		tm.effort = f.o.effort
	}
	return tm
}

// resumeModel is what c's resumed turn starts with, read afresh, so a Tag
// changed while it waited applies. ok is false when Code Central can't
// say, and the resume waits a poll.
func (f *factory) resumeModel(ctx context.Context, c claim) (turnModel, bool) {
	mc, err := f.client.GetModelChoice(ctx, f.project, core.GetModelChoiceInput{Ticket: core.Ref(c.Ticket)})
	if err != nil {
		if ctx.Err() == nil {
			f.claimf(c, "%s: get_model_choice: %v: waiting a poll before resuming", c.Ticket, err)
		}
		return turnModel{}, false
	}
	return f.withFlags(&mc), true
}

// reportModel records on c's Claim what its turn was started with.
func (f *factory) reportModel(ctx context.Context, c claim, tm turnModel) {
	in := core.ReportModelChoiceInput{Ticket: core.Ref(c.Ticket), ClaimID: c.ClaimID, Model: tm.model, Effort: tm.effort}
	if _, err := f.client.ReportModelChoice(ctx, f.project, in); err != nil && ctx.Err() == nil {
		f.claimf(c, "%s: report_model_choice: %v", c.Ticket, err)
	}
}
