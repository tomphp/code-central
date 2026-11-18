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
	g, err := f.client.GetStageGuide(ctx, f.project, core.StageGuideInput{Stage: stage})
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
	core.RetroLabel:          {skill: "retro"},
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

// sessionLogs tells a Retro where to read the sessions of the Claims it
// looks back on. It is Claude Code's rule, so the Factory states it, as
// the one place that names upstream tools, and the Guide doesn't. The
// context report (internal/contextreport) finds sessions by the same
// rule, but is a separate main package, so it isn't shared.
const sessionLogs = "Claude Code keeps each session's log in its projects folder, `~/.claude/projects` (or `$CLAUDE_CONFIG_DIR/projects` when that is set), at `<projects folder>/<working_dir with every non-alphanumeric character replaced by '-'>/<session_id>.jsonl`, where working_dir and session_id are those of the Claim whose session you read; that session's subagents' logs are under `<session_id>/subagents/` beside it, one `.jsonl` each."

// conflictFinish ends the system prompt of a conflict Claim, in place of
// the Stage Guide. Its first words mark the session for the context
// report (internal/contextreport). It keeps what finish requires of an
// implementation Ticket: the branch and its head commit.
const conflictFinish = "Conflict Claim: the first prompt holds the work. When it is done, `finish` with `branch` (the Ticket's ID) and `commit` (the branch's head commit), and a summary of how each conflict was resolved and the test result. The Ticket goes to In Review; stop working it."

// severalConflictFinish is conflictFinish in several Repositories, where
// finish records each other Repository committed in as well.
const severalConflictFinish = "Conflict Claim: the first prompt holds the work. When it is done, `finish` with `branch` (the Ticket's ID) and `commit` (the branch's head commit) for the Home Repository and `repositories` for each other one you committed in, and a summary of how each conflict was resolved and the test result. The Ticket goes to In Review; stop working it."

// conflict is the files merging a Repository's base into the Ticket's
// branch in its worktree w conflicts in.
type conflict struct {
	w     repositoryWorktree
	files []string
}

// conflictsIn is the conflicts of tk's branch with the base in each
// Repository of ws that has them (conflictFiles).
func (f *factory) conflictsIn(ws worktreeSet, stage string, tk core.TicketView) []conflict {
	var out []conflict
	for _, w := range ws {
		if files := f.conflictFiles(w.r, stage, tk); len(files) > 0 {
			out = append(out, conflict{w, files})
		}
	}
	return out
}

// rebases reports whether Rebase is in force (ADR 0067) in any
// Repository of conflicts, so that a conflict Claim is asked to rebase its
// branch, not merge the base into it. Without the Project, it merges.
func (f *factory) rebases(ctx context.Context, conflicts []conflict) bool {
	if len(conflicts) == 0 {
		return false
	}
	p, err := f.client.GetProject(ctx, f.project)
	if err != nil {
		f.printf("warning: can't read the Landing Policy, so the conflict Claim is asked to merge: %v", err)
		return false
	}
	policies := p.LandingPolicies()
	for _, cf := range conflicts {
		if cf.w.r.policy(policies).RebaseInForce() {
			return true
		}
	}
	return false
}

// conflictFiles is the files merging r's base's tip into the Ticket's
// existing branch in r conflicts in, for an implementation Ticket; none
// for any other, for a branch not yet cut (but a Spec merge Ticket's,
// checked at the Spec Branch), or when the merge is clean.
func (f *factory) conflictFiles(r *repository, stage string, tk core.TicketView) []string {
	branch := workBranch(stage, tk.ID)
	if stage != core.ImplementationStage || branch == "" {
		return nil
	}
	head, err := r.git.commit("refs/heads/" + branch)
	if err != nil {
		// A Spec merge Ticket, marked or one opened before the mark, has
		// no branch yet: it is cut from the Spec Branch, which is what
		// conflicts.
		if !tk.SpecLanded || tk.Title != core.SpecMergeTitle(tk.Spec.ID) {
			return nil
		}
		if head, err = f.repos.specBranchTip(r, tk.Spec.ID); err != nil {
			return nil
		}
	}
	base := f.repos.baseOf(r)
	baseTip, err := r.git.commit("refs/heads/" + base)
	if err != nil {
		return nil
	}
	_, conflicts, err := r.git.mergeTree(baseTip, head)
	if err != nil {
		f.printf("warning: %s: can't check the branch against %s%s: %v", tk.ID, base, r.where(), err)
		return nil
	}
	return conflicts
}

// mapResearch is the guide of a Map's research Ticket.
const mapResearch = "map-research"

// firstPrompt is the prompt that starts a Claim's first turn: the
// guide's skill as a slash command with the Ticket's ID alone as its
// arguments, since Claude Code repeats a skill's arguments in the expanded
// skill. A guide that starts no skill is pointed at the system prompt
// (systemPrompt), which holds everything else.
//
// A conflict Claim (conflicts non-empty) starts no skill: the prompt
// holds the instructions for resolving a merge of the base into the
// Ticket's branch, or a rebase of the branch onto the base when rebase,
// Rebase being in force (ADR 0067), which is all the work there is.
func firstPrompt(c claim, stage, base string, conflicts []string, rebase bool) string {
	if len(conflicts) > 0 {
		var b strings.Builder
		do := fmt.Sprintf("Merge %s into the branch", base)
		if rebase {
			do = fmt.Sprintf("Rebase the branch onto %s", base)
		}
		fmt.Fprintf(&b, "Ticket %s's branch, %s, conflicts with the base, %s. %s and resolve the conflicts in these files:\n\n", c.Ticket, c.Ticket, base, do)
		for _, f := range conflicts {
			fmt.Fprintf(&b, "- %s\n", f)
		}
		b.WriteString("\nResolve them keeping the intent of both sides. Run `make typecheck` and the tests the resolution touches. Commit on the branch, with no push and no PR, and finish the Ticket.")
		return b.String()
	}
	if sk := stageSkills[stage]; sk.skill != "" {
		return fmt.Sprintf("/%s %s", sk.skill, c.Ticket)
	}
	return fmt.Sprintf("Work Ticket %s, as the Factory's instructions and its Stage Guide in your system prompt say.", c.Ticket)
}

// severalFirstPrompt is firstPrompt for a Claim with its worktrees ws in
// several Repositories (ADR 0048): a conflict Claim's names the files
// conflicting in each Repository, and either ends with the Repositories
// list, where the session finds each one.
func severalFirstPrompt(c claim, stage string, ws worktreeSet, conflicts []conflict, rebase bool) string {
	p := firstPrompt(c, stage, "", nil, false)
	if len(conflicts) > 0 {
		var b strings.Builder
		do := "merge its base into the branch"
		if rebase {
			do = "rebase the branch onto its base"
		}
		fmt.Fprintf(&b, "Ticket %s's branch, %s, conflicts with the base in these Repositories. In each one's worktree, %s and resolve the conflicts in these files:\n", c.Ticket, c.Ticket, do)
		for _, cf := range conflicts {
			fmt.Fprintf(&b, "\n%s, base %s:\n", cf.w.r.name, cf.w.base)
			for _, f := range cf.files {
				fmt.Fprintf(&b, "- %s\n", f)
			}
		}
		b.WriteString("\nResolve them keeping the intent of both sides. Run `make typecheck` and the tests the resolution touches. Commit on the branch in each worktree, with no push and no PR, and finish the Ticket.")
		p = b.String()
	}
	return p + "\n\n" + repositoriesList(ws)
}

// repositoriesList is the Repositories list that ends a first prompt in
// several Repositories: one line for each, with its worktree, Base and
// the branch to commit on, or that it is detached for a Stage.
func repositoriesList(ws worktreeSet) string {
	var lines []string
	for _, w := range ws {
		on := "branch " + w.branch
		if w.branch == "" {
			on = "detached at its tip"
		}
		lines = append(lines, fmt.Sprintf("- %s: worktree %s, Base %s, %s", w.label(), w.path, w.base, on))
	}
	return "Repositories:\n" + strings.Join(lines, "\n")
}

// systemPrompt is what every turn of a Claim adds to claude's system
// prompt: the Ticket and Claim, how the Factory runs it in its worktree
// wt, and the guide from the claim. stage is the guide's name. Claude Code
// records the system prompt at a session's first request and reuses it on
// every resume until the session is compacted, so it reaches the agent
// once, and again after a compaction since every turn passes it.
//
// For a conflict Claim (conflicts non-empty) the Stage Guide is replaced
// by a short finish instruction, since the first prompt holds the work.
func systemPrompt(c claim, stage, wt string, g core.StageGuideView, conflicts []string) string {
	return claimIntro(c, stage, wt) + guideOrFinish(g, len(conflicts) > 0)
}

// severalSystemPrompt is systemPrompt for a Claim with its worktrees ws
// in several Repositories (ADR 0048): it adds where they are and how to
// commit in them and finish, before the guide.
func severalSystemPrompt(c claim, stage string, ws worktreeSet, g core.StageGuideView, conflicted bool) string {
	var several string
	if branch := workBranch(stage, c.Ticket); branch != "" {
		several = fmt.Sprintf("This Project has several Repositories, and this Ticket has a worktree in each on branch %s, listed at the end of your first prompt; this session starts in the Home Repository's. Commit each Repository's changes in its own worktree, on branch %s, and finish with `repositories`: one entry for each Repository you committed in, with its branch and head commit.", branch, branch)
	} else {
		several = "This Project has several Repositories, and this Ticket has a detached, read-only worktree in each at its Base's tip, listed at the end of your first prompt; this session starts in the Home Repository's."
	}
	end := guideOrFinish(g, false)
	if conflicted {
		end = "\n\n" + severalConflictFinish
	}
	return claimIntro(c, stage, ws.home().path) + "\n\n" + several + end
}

// claimIntro starts a Claim's system prompt: its Ticket and Claim, and
// how the Factory runs it in its worktree wt.
func claimIntro(c claim, stage, wt string) string {
	var b strings.Builder
	sk := stageSkills[stage]
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
		if !slices.Contains(core.Stages(), core.StageName(stage)) {
			what, writer = "this "+stage+" Ticket", "It"
		}
		fmt.Fprintf(&b, "You are running under code-central-factory, in a detached, read-only worktree at the base's tip, for %s. %s Finish the Ticket when it is done. %s writes nothing to the repository: no files, no commits and no branches.", what, claimed, writer)
		if stage == core.RetroLabel {
			b.WriteString("\n\n" + sessionLogs)
		}
	}
	fmt.Fprintf(&b, "\n\nYour first call is `get_ticket` with `ticket: %s`. The code-central tools are deferred: load their schemas with ToolSearch (`select:mcp__code-central__get_ticket,...`) before you call them, and pass `ticket` on the writes that take it.", c.Ticket)
	return b.String()
}

// guideOrFinish ends a Claim's system prompt: the Stage Guide g, or for a
// conflict Claim a short finish instruction in its place.
func guideOrFinish(g core.StageGuideView, conflicted bool) string {
	if conflicted {
		return "\n\n" + conflictFinish
	}
	return fmt.Sprintf("\n\nThe Stage Guide for %s follows: what this Ticket must produce and how to finish it.\n\n%s", g.Stage, g.Guide)
}
