package factory

import (
	"context"
	"fmt"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// specBranch is the name of a Spec's Spec Branch, where its Spec-landed
// Tickets' work gathers under Land per Spec.
func specBranch(spec string) string { return "spec/" + spec }

// specBranchTip makes sure the Spec Branch of spec exists and returns its
// tip. A missing one is created at the base's tip; one already in the
// base (after an earlier landing of the Spec) is fast-forwarded to the
// base's tip first. The branch is never checked out: only its ref moves.
func (f *factory) specBranchTip(spec string) (string, error) {
	ref := "refs/heads/" + specBranch(spec)
	baseTip, err := f.repository.commit("refs/heads/" + f.base)
	if err != nil {
		return "", fmt.Errorf("the base, %s, isn't a branch", f.base)
	}
	tip, err := f.repository.commit(ref)
	if err != nil {
		if err := f.repository.updateRef(ref, baseTip, ""); err != nil {
			return "", fmt.Errorf("can't create the Spec Branch %s (a branch named %q may be in the way): %v", specBranch(spec), "spec", err)
		}
		return baseTip, nil
	}
	if tip != baseTip && f.inBase(tip) {
		if err := f.repository.updateRef(ref, baseTip, tip); err != nil {
			return "", fmt.Errorf("can't move the Spec Branch %s to %s's tip: %v", specBranch(spec), f.base, err)
		}
		return baseTip, nil
	}
	return tip, nil
}

// specBranchFailed is the failure for a Spec Branch the Factory can't
// create or move at a Claim.
func specBranchFailed(wt string, err error) *failure {
	return &failure{core.ReportFailureInput{
		WhatBroke:       fmt.Sprintf("The Factory couldn't set up this Ticket's Spec Branch: %v", err),
		Why:             "Under Land per Spec a Ticket's branch is cut from its Spec's Spec Branch, which the Factory creates and moves by ref, and git refused.",
		WhatToFix:       "Read the git error above and fix what it names in the repository (a branch called spec, a stale lock file). Then send the Ticket back.",
		WhereWorkStands: fmt.Sprintf("No session ran under this Claim. The Factory stopped before adding the worktree %s and touched nothing after the failure.", wt),
	}}
}

// mergeIntoSpecBranch merges an Accepted Spec-landed Ticket's work into
// its Spec Branch and reports the merge to Code Central. Nothing reaches
// the base: the Spec lands later. A conflict sends the Ticket back to
// Ready.
func (f *factory) mergeIntoSpecBranch(ctx context.Context, tk core.TicketView) {
	head, what, err := f.workOf(tk)
	if err != nil {
		f.cantLandYet(ctx, tk, what+" isn't in this repository.")
		return
	}
	branch := specBranch(tk.Spec.ID)
	ref := "refs/heads/" + branch
	tip, err := f.specBranchTip(tk.Spec.ID)
	if err != nil {
		f.cantLandYet(ctx, tk, err.Error()+".")
		return
	}
	if f.repository.isAncestor(head, ref) {
		f.reportSpecMerge(ctx, tk, branch, tip)
		return
	}
	tree, conflicts, err := f.repository.mergeTree(tip, head)
	if err != nil {
		f.printf("warning: %s: can't merge: %v", tk.ID, err)
		return
	}
	if len(conflicts) > 0 {
		f.sendBack(ctx, tk, head, branch, tip, conflicts)
		return
	}
	merge, err := f.repository.commitTree(tree, fmt.Sprintf("Merge %s: %s", tk.ID, tk.Title), tip, head)
	if err != nil {
		f.printf("warning: %s: can't make the merge commit: %v", tk.ID, err)
		return
	}
	if err := f.repository.updateRef(ref, merge, tip); err != nil {
		f.cantLandYet(ctx, tk, fmt.Sprintf("the Spec Branch %s moved while the Factory was merging, or git refused: %v", branch, err))
		return
	}
	f.reportSpecMerge(ctx, tk, branch, merge)
}

// reportSpecMerge tells Code Central tk's work is in the Spec Branch at
// commit.
func (f *factory) reportSpecMerge(ctx context.Context, tk core.TicketView, branch, commit string) {
	if _, err := f.client.ReportSpecMerge(ctx, f.project, core.ReportSpecMergeInput{Ticket: core.Ref(tk.ID), Commit: commit}); err != nil {
		if ctx.Err() == nil {
			f.printf("%s: report_spec_merge: %v", tk.ID, err)
		}
		return
	}
	f.printf("%s: merged into %s at %s", tk.ID, branch, short(commit))
}

// waitsOnSpecBranch reports whether an Accepted tk goes through its Spec
// Branch rather than into the base: it is Spec-landed and its work isn't
// in the base yet (such as work accepted before Land per Spec was turned
// on, which lands as plain Merge).
func (f *factory) waitsOnSpecBranch(tk core.TicketView, landing core.LandingView) bool {
	if !tk.SpecLanded || landing.Method != store.LandingMerge || !landing.LandPerSpec {
		return false
	}
	head, _, err := f.workOf(tk)
	return err != nil || !f.inBase(head)
}

// cutBranchWorktree gives a claimed Ticket its worktree on branch. A new
// branch is cut from the base's tip, or for a Spec-landed Ticket from its
// Spec Branch's tip; an existing branch is reused as it is.
func (f *factory) cutBranchWorktree(tk core.TicketView, branch, wt string) error {
	from := f.base
	if tk.SpecLanded {
		if _, err := f.repository.commit("refs/heads/" + branch); err != nil {
			tip, err := f.specBranchTip(tk.Spec.ID)
			if err != nil {
				return specBranchFailed(wt, err)
			}
			from = tip
		}
	}
	return f.branchWorktree(branch, wt, from)
}
