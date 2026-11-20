package factory

import (
	"context"
	"fmt"
	"strings"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

// afterTurn acts on how a turn ended, and returns the State the Ticket
// was left in, as the Factory read it or moved it; "" when it read none.
// A turn the Factory stopped, or one that ended because the Factory is
// stopping, is left alone.
func (f *factory) afterTurn(ctx context.Context, t *turn, out turnOutcome) domain.State {
	if ctx.Err() != nil || out.Kind == turnStopped || t.stopped.Load() {
		return ""
	}
	c := t.claim
	end := out.End
	if out.Kind == turnLoginFailed {
		f.stopFor(fmt.Sprintf("claude's login has expired or is missing (authentication_failed, on %s): log in with `claude /login`, then start the Factory again", c.Ticket))
		return ""
	}
	if out.Kind != turnOutage {
		f.turnWentWell(t)
	}
	tk, err := f.client.GetTicket(ctx, f.project, api.Ref(c.Ticket))
	if err != nil {
		f.claimf(c, "%s: get_ticket: %v", c.Ticket, err)
		return ""
	}
	var denials []denial
	if end.Result != nil {
		denials = end.Result.PermissionDenials
	}
	// The Claim may already be dropped by a poll that saw the finish, so
	// liveness here is read from the Ticket alone.
	ended, by := claimEnded(tk, c)
	if ended || tk.State != domain.InProgress {
		// The session ended its turn itself (finish, a Round, give_up):
		// its denials are for the human to see.
		if len(denials) > 0 {
			f.commentDenials(ctx, t, denials)
		}
		if checksFinish(by, tk.State) {
			if _, failed := checkFinish(ctx, f.repos, f.client, f.project, tk); failed {
				return domain.Failed
			}
		}
		return tk.State
	}
	if !f.live(c.ClaimID) {
		return tk.State
	}
	switch {
	case out.Kind == turnOutage:
		f.cutOff(c, end.apiError())
	case out.Kind == turnModelRejected:
		if f.reportRejectedModel(ctx, t, end) {
			return domain.Failed
		}
	case len(denials) > 0:
		if f.postDenials(ctx, t, denials) {
			return domain.NeedsAnswer
		}
	default:
		if f.reportStuck(ctx, t, end) {
			return domain.Failed
		}
	}
	return tk.State
}

// reportStuck files a Failure Report, as the holder, for a turn that
// exited on its own with its Claim still In Progress, and reports
// whether it did.
func (f *factory) reportStuck(ctx context.Context, t *turn, end turnEnd) bool {
	c := t.claim
	why := "The agent's last text:\n\n" + end.LastText
	if strings.TrimSpace(end.LastText) == "" {
		why = "The agent printed no text."
	}
	fix := fmt.Sprintf("Read the session (claude --resume %s, in %s) to see why it stopped, fix the cause and send the Ticket back.", c.SessionID, t.dir)
	if end.Result == nil && len(f.o.claudeArgs) > 0 {
		fix += fmt.Sprintf(" The turn started with the Claude Args %s: if claude rejected one, fix it and restart the Factory.", joinArgs(f.o.claudeArgs))
	}
	return f.reportFailure(ctx, c, &failure{api.ReportFailureInput{
		WhatBroke:       fmt.Sprintf("The turn exited with its Claim still In Progress: %s.", end.how()),
		Why:             why,
		WhatToFix:       fix,
		WhereWorkStands: f.whereWorkStands(t.dir),
	}})
}

// whereWorkStands names the worktree, its branch and its last commit.
func (f *factory) whereWorkStands(dir string) string {
	branch := "a detached HEAD"
	repo := f.repos.home.git
	if b, err := repo.CurrentBranch(dir); err == nil {
		branch = "branch " + b
	}
	id, subject, err := repo.LastCommit(dir)
	last := strings.TrimSpace(id + " " + subject)
	if err != nil {
		last = "unknown (" + err.Error() + ")"
	}
	return fmt.Sprintf("The worktree %s is on %s. Its last commit is %s.", dir, branch, last)
}
