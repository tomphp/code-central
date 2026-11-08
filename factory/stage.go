package factory

import (
	"context"
	"fmt"
	"strings"

	"github.com/tomoram/code-central/core"
)

// stageGuide is the Stage Guide from the claim, or, from a service whose
// claim gives none, the one get_stage_guide serves. stage is the guide's
// name: a Stage or core.ImplementationStage.
func (f *factory) stageGuide(ctx context.Context, fromClaim *core.StageGuideView, stage string) core.StageGuideView {
	if fromClaim != nil {
		return *fromClaim
	}
	g, err := f.client.GetStageGuide(ctx, core.StageGuideInput{Stage: stage})
	if err != nil {
		f.printf("get_stage_guide %s: %v", stage, err)
		return core.StageGuideView{Stage: stage}
	}
	return g
}

// failure is a reason to stop a Claim with a Failure Report.
type failure struct{ report core.ReportFailureInput }

func (e *failure) Error() string { return e.report.WhatBroke }

// reportFailure posts the failure for Claim c as its holder, and reports
// whether it did.
func (f *factory) reportFailure(ctx context.Context, c claim, fl *failure) bool {
	in := fl.report
	in.Ticket, in.ClaimID = core.Ref(c.Ticket), c.ClaimID
	if _, err := f.client.ReportFailure(ctx, f.project, in); err != nil {
		f.claimf(c, "%s: report_failure: %v", c.Ticket, err)
		return false
	}
	f.claimf(c, "%s: Failed: %s", c.Ticket, firstLine(in.WhatBroke))
	return true
}

// stageSkills is the skill each Stage, and implementation, starts, as a
// slash command.
var stageSkills = map[string]string{
	"grilling":               "grilling",
	"spec":                   "to-spec",
	"tickets":                "to-tickets",
	core.ImplementationStage: "implement",
}

// firstPrompt is the prompt that starts a Claim's first turn: the Stage's
// skill as a slash command, the Ticket and Claim, how the Factory runs
// it, and the Stage Guide from the claim. stage is a Stage or
// core.ImplementationStage.
func firstPrompt(c claim, stage string, g core.StageGuideView) string {
	var b strings.Builder
	fmt.Fprintf(&b, "/%s %s\n\nTicket: %s\nClaim: %s\n\n", stageSkills[stage], c.Ticket, c.Ticket, c.ClaimID)
	claimed := fmt.Sprintf("The Factory has already claimed this Ticket for you: Claim %s is yours, so don't call claim, and pass %s as claim_id on every write that needs it.", c.ClaimID, c.ClaimID)
	if stage == core.ImplementationStage {
		fmt.Fprintf(&b, "You are running under code-central-factory, in this Ticket's own worktree on branch %s. %s Commit on this branch only, with no push and no PR, and finish the Ticket when it is done.", c.Ticket, claimed)
	} else {
		if stage == "grilling" {
			b.WriteString("Run /domain-modeling alongside /grilling, to sharpen the Requirement's terms as you grill.\n\n")
		}
		fmt.Fprintf(&b, "You are running under code-central-factory, in a detached, read-only worktree at the base's tip, for the %s Stage. %s Finish the Ticket when it is done. A Stage writes nothing to the repository: no files, no commits and no branches.", stage, claimed)
	}
	fmt.Fprintf(&b, "\n\nThe Stage Guide for %s follows: what this Ticket must produce and how to finish it. If you lose it, after compaction say, read it again with get_stage_guide, stage %q.\n\n%s", g.Stage, g.Stage, g.Guide)
	return b.String()
}
