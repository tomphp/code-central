package factory

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/tomoram/code-central/core"
)

// stageGuide is the Stage Guide from the claim, or, from a service whose
// claim gives none, the one get_stage_guide serves. stage is the guide's
// name, one of core.GuideNames.
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

// guideSkill is the skill a guide starts, as a slash command name, and
// the skill to run alongside it, if any.
type guideSkill struct{ skill, alongside string }

// stageSkills is the skill each guide the Factory runs starts, by guide
// name; an empty skill starts none, so the prompt is the guide alone. It
// is the only place that names upstream skills. A Ticket whose guide
// isn't here is left unclaimed.
var stageSkills = map[string]guideSkill{
	"grilling":               {"grilling", "domain-modeling"},
	"spec":                   {skill: "to-spec"},
	"tickets":                {skill: "to-tickets"},
	"map-chart":              {skill: "wayfinder"},
	"map-grilling":           {"grilling", "domain-modeling"},
	mapResearch:              {skill: "research"},
	"map-task":               {},
	core.PrototypeLabel:      {skill: "prototype"},
	core.ImplementationStage: {skill: "implement"},
}

// runs reports whether the Factory runs a Ticket with the named guide.
func runs(guide string) bool {
	_, ok := stageSkills[guide]
	return ok
}

// workBranch is the branch a Ticket with the named guide works on: its
// own ID for implementation, which lands, and prototype/<ticket> for a
// Prototype or research/<ticket> for a Map's research, which are local
// and never land. Any other guide works detached, on no branch.
func workBranch(guide, ticket string) string {
	switch guide {
	case core.ImplementationStage:
		return ticket
	case core.PrototypeLabel:
		return core.PrototypeLabel + "/" + ticket
	case mapResearch:
		return "research/" + ticket
	}
	return ""
}

// mapResearch is the guide of a Map's research Ticket.
const mapResearch = "map-research"

// firstPrompt is the prompt that starts a Claim's first turn: the guide's
// skill as a slash command, the Ticket and Claim, how the Factory runs
// it in its worktree wt, and the guide from the claim. stage is the
// guide's name.
func firstPrompt(c claim, stage, wt string, g core.StageGuideView) string {
	var b strings.Builder
	sk := stageSkills[stage]
	if sk.skill != "" {
		fmt.Fprintf(&b, "/%s %s\n\n", sk.skill, c.Ticket)
	}
	fmt.Fprintf(&b, "Ticket: %s\nClaim: %s\n\n", c.Ticket, c.ClaimID)
	claimed := fmt.Sprintf("The Factory has already claimed this Ticket for you: Claim %s is yours, so don't call claim, and pass %s as claim_id on every write that needs it.", c.ClaimID, c.ClaimID)
	switch branch := workBranch(stage, c.Ticket); stage {
	case core.ImplementationStage:
		fmt.Fprintf(&b, "You are running under code-central-factory, in this Ticket's own worktree on branch %s. %s Commit on this branch only, with no push and no PR, and finish the Ticket when it is done.", branch, claimed)
	case core.PrototypeLabel:
		fmt.Fprintf(&b, "You are running under code-central-factory, in this Ticket's own worktree, %s, on the local branch %s. %s Commit on this branch only, with no push and no PR. Record the prototype Tag as `%s@<commit> %s`, with the branch's head commit; open the first Round with a \"Try it\" Question; and finish with no branch, since a Prototype never lands.", wt, branch, claimed, branch, wt)
	case mapResearch:
		fmt.Fprintf(&b, "You are running under code-central-factory, in this Ticket's own worktree, %s, on the local branch %s. %s Commit on this branch only, with no push and no PR. Record the research Tag as `%s@<commit>`, with the branch's head commit, and finish with no branch, since research never lands.", wt, branch, claimed, branch)
	default:
		if sk.alongside != "" {
			fmt.Fprintf(&b, "Run /%s alongside /%s, to sharpen the terms as you grill.\n\n", sk.alongside, sk.skill)
		}
		what, writer := "the "+stage+" Stage", "A Stage"
		if !slices.Contains(core.Stages, stage) {
			what, writer = "this "+stage+" Ticket", "It"
		}
		fmt.Fprintf(&b, "You are running under code-central-factory, in a detached, read-only worktree at the base's tip, for %s. %s Finish the Ticket when it is done. %s writes nothing to the repository: no files, no commits and no branches.", what, claimed, writer)
	}
	fmt.Fprintf(&b, "\n\nThe Stage Guide for %s follows: what this Ticket must produce and how to finish it. If you lose it, after compaction say, read it again with get_stage_guide, stage %q.\n\n%s", g.Stage, g.Stage, g.Guide)
	return b.String()
}
