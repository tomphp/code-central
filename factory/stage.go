package factory

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/tomoram/code-central/core"
)

// stageOf is the Stage a Ticket's stage:* Label marks it as, or "" for an
// implementation Ticket.
func stageOf(t core.TicketView) string {
	for _, l := range t.Labels {
		if name, ok := strings.CutPrefix(l, core.StagePrefix); ok && slices.Contains(core.Stages, name) {
			return name
		}
	}
	return ""
}

// stageGuide is the Stage Guide from the claim, or, from a service whose
// claim gives none, the one get_stage_guide serves.
func (f *factory) stageGuide(ctx context.Context, fromClaim *core.StageGuideView, stage string) core.StageGuideView {
	if fromClaim != nil {
		return *fromClaim
	}
	if stage == "" {
		stage = core.ImplementationStage
	}
	g, err := f.client.GetStageGuide(ctx, core.StageGuideInput{Stage: stage})
	if err != nil {
		f.printf("get_stage_guide %s: %v", stage, err)
		return core.StageGuideView{Stage: stage}
	}
	return g
}

// stageWorktree gives Stage Ticket id its worktree at wt: detached at the
// base's tip, added when it isn't there and moved to the current tip when
// it is.
// A dirty one is left as it is, as a failure: the Stage wrote files it
// shouldn't have.
func (f *factory) stageWorktree(id, wt string) error {
	if _, err := os.Stat(wt); err != nil {
		_, err := f.gitOut(f.repo, "worktree", "add", "-q", "--detach", wt, f.base)
		return err
	}
	status, err := f.gitOut(wt, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return err
	}
	if status != "" {
		head, _ := f.gitOut(wt, "rev-parse", "--short", "HEAD")
		return &failure{core.ReportFailureInput{
			WhatBroke:       fmt.Sprintf("The Stage's worktree is dirty, so the Factory can't start the Stage. Changed files:\n\n%s", status),
			Why:             fmt.Sprintf("A Stage writes nothing to the repository, but %s has uncommitted changes, most likely written by an earlier Claim's session. The Factory moves a Stage's worktree to %s's tip at each new Claim only when it is clean.", wt, f.base),
			WhatToFix:       fmt.Sprintf("Look at the changes in %s and keep anything worth keeping elsewhere (a Stage's output belongs in Code Central). Then clean the worktree and send the Ticket back.", wt),
			WhereWorkStands: fmt.Sprintf("No session ran under this Claim. The worktree %s is detached at %s, with the changes above left as they were.", wt, head),
		}}
	}
	_, err = f.gitOut(wt, "checkout", "-q", "--detach", f.base)
	return err
}

// failure is a reason to stop a Claim with a Failure Report.
type failure struct{ report core.ReportFailureInput }

func (e *failure) Error() string { return e.report.WhatBroke }

// reportFailure posts the failure for Claim c as its holder.
func (f *factory) reportFailure(ctx context.Context, c claim, fl *failure) {
	in := fl.report
	in.Ticket, in.ClaimID = core.Ref(c.Ticket), c.ClaimID
	if _, err := f.client.ReportFailure(ctx, f.project, in); err != nil {
		f.printf("%s: report_failure: %v", c.Ticket, err)
		return
	}
	f.printf("%s: Failed: %s", c.Ticket, strings.SplitN(in.WhatBroke, "\n", 2)[0])
}

// stageSkills is the skill each Stage starts, as a slash command.
var stageSkills = map[string]string{
	"grilling": "grilling",
	"spec":     "to-spec",
	"tickets":  "to-tickets",
	"":         "implement",
}

// firstPrompt is the prompt that starts a Claim's first turn: the Stage's
// skill as a slash command, the Ticket and Claim, how the Factory runs
// it, and the Stage Guide from the claim.
func firstPrompt(c claim, stage string, g core.StageGuideView) string {
	var b strings.Builder
	fmt.Fprintf(&b, "/%s %s\n\nTicket: %s\nClaim: %s\n\n", stageSkills[stage], c.Ticket, c.Ticket, c.ClaimID)
	if stage == "" {
		fmt.Fprintf(&b, "You are running under code-central-factory, in this Ticket's own worktree on branch %s. Work the Ticket under Claim %s: commit on this branch only, with no push and no PR, and finish the Ticket when it is done.", c.Ticket, c.ClaimID)
	} else {
		if stage == "grilling" {
			b.WriteString("Run /domain-modeling alongside /grilling, to sharpen the Requirement's terms as you grill.\n\n")
		}
		fmt.Fprintf(&b, "You are running under code-central-factory, in a detached, read-only worktree at the base's tip. Work this %s Stage under Claim %s and finish the Ticket when it is done. A Stage writes nothing to the repository: no files, no commits and no branches.", stage, c.ClaimID)
	}
	fmt.Fprintf(&b, "\n\nThe Stage Guide for %s follows: what this Ticket must produce and how to finish it. If you lose it, after compaction say, read it again with get_stage_guide, stage %q.\n\n%s", g.Stage, g.Stage, g.Guide)
	return b.String()
}
