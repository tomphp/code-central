package factory

import (
	"context"
	"fmt"
	"strings"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// afterTurn acts on how a turn ended, and returns the State the Ticket
// was left in, as the Factory read it or moved it; "" when it read none.
// A turn the Factory stopped, or one that ended because the Factory is
// stopping, is left alone.
func (f *factory) afterTurn(ctx context.Context, t *turn, end turnEnd) store.State {
	if ctx.Err() != nil || t.stopped.Load() {
		return ""
	}
	c := t.claim
	apiErr := end.apiError()
	if apiErr == authFailed {
		f.stopFor(fmt.Sprintf("claude's login has expired or is missing (authentication_failed, on %s): log in with `claude /login`, then start the Factory again", c.Ticket))
		return ""
	}
	if !outageErrors[apiErr] {
		f.turnWentWell(t)
	}
	tk, err := f.client.GetTicket(ctx, f.project, core.Ref(c.Ticket))
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
	ended, _ := claimEnded(tk, c)
	if ended || tk.State != store.InProgress {
		// The session ended its turn itself (finish, a Round, give_up):
		// its denials are for the human to see.
		if len(denials) > 0 {
			f.commentDenials(ctx, t, denials)
		}
		return tk.State
	}
	if !f.live(c.ClaimID) {
		return tk.State
	}
	if outageErrors[apiErr] {
		f.cutOff(c, apiErr)
		return tk.State
	}
	if apiErr == modelNotFound {
		if f.reportRejectedModel(ctx, t, end) {
			return store.Failed
		}
		return tk.State
	}
	if len(denials) > 0 {
		if f.postDenials(ctx, t, denials) {
			return store.NeedsAnswer
		}
		return tk.State
	}
	if f.reportStuck(ctx, t, end) {
		return store.Failed
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
	return f.reportFailure(ctx, c, &failure{core.ReportFailureInput{
		WhatBroke:       fmt.Sprintf("The turn exited with its Claim still In Progress: %s.", end.how()),
		Why:             why,
		WhatToFix:       fix,
		WhereWorkStands: f.whereWorkStands(t.dir),
	}})
}

// how is how the turn ended: its exit code or signal, subtype and
// terminal reason.
func (t turnEnd) how() string {
	s := t.exit()
	if t.Result == nil {
		return s + ", with no result message"
	}
	s += ", subtype " + t.Result.Subtype
	if t.Result.TerminalReason != "" {
		s += ", terminal_reason " + t.Result.TerminalReason
	}
	return s
}

// whereWorkStands names the worktree, its branch and its last commit.
func (f *factory) whereWorkStands(dir string) string {
	branch := "a detached HEAD"
	if b, err := f.gitOut(dir, "symbolic-ref", "--short", "-q", "HEAD"); err == nil && b != "" {
		branch = "branch " + b
	}
	last, err := f.gitOut(dir, "log", "-1", "--format=%h %s")
	if err != nil {
		last = "unknown (" + err.Error() + ")"
	}
	return fmt.Sprintf("The worktree %s is on %s. Its last commit is %s.", dir, branch, last)
}
