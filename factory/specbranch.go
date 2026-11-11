package factory

import (
	"context"
	"fmt"
	"sort"
	"strings"

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

// landSpecs merges the Spec Branch of every Spec ready to land into the
// base and lands the Spec's waiting Tickets as a group, by the Push and
// CI gate path. A Spec's Tickets are the Accepted ones the Factory has
// merged into its Spec Branch and not yet landed.
func (f *factory) landSpecs(ctx context.Context, landing core.LandingView) {
	if landing.Method != store.LandingMerge || !landing.LandPerSpec {
		return
	}
	waiting := map[string]bool{}
	var specs []string
	for _, tk := range f.landable(ctx, store.Accepted) {
		if tk.SpecLanded && !waiting[tk.Spec.ID] {
			waiting[tk.Spec.ID] = true
			specs = append(specs, tk.Spec.ID)
		}
	}
	for _, id := range specs {
		if f.halted(ctx) || f.paused() {
			return
		}
		f.landSpec(ctx, id, landing)
	}
}

// landSpec lands one Spec when Code Central says it is ready.
func (f *factory) landSpec(ctx context.Context, id string, landing core.LandingView) {
	spec, err := f.client.GetSpec(ctx, f.project, core.GetSpecInput{Spec: core.Ref(id), Parts: []string{"tickets"}})
	if err != nil {
		if ctx.Err() == nil {
			f.printf("%s: get_spec: %v", id, err)
		}
		return
	}
	if spec.Requirement == nil || spec.Requirement.LandPerSpec == nil || !spec.Requirement.LandPerSpec.Ready {
		return
	}
	var tickets []core.TicketView
	for _, w := range spec.Requirement.LandPerSpec.Waiting {
		tk, err := f.client.GetTicket(ctx, f.project, core.Ref(w))
		if err != nil {
			if ctx.Err() == nil {
				f.printf("%s: get_ticket: %v", w, err)
			}
			return
		}
		tickets = append(tickets, tk)
	}
	sort.SliceStable(tickets, func(i, j int) bool { return acceptedAt(tickets[i]).Before(acceptedAt(tickets[j])) })
	ref := "refs/heads/" + specBranch(id)
	tip, err := f.repository.commit(ref)
	if err != nil {
		return
	}
	baseTip, err := f.repository.commit("refs/heads/" + f.base)
	if err != nil {
		f.printf("warning: %s: the base, %s, isn't a branch, so the Spec can't land", id, f.base)
		return
	}
	// Merged on an earlier poll whose landing then failed: those Tickets
	// land through the plain path, as their work is in the base.
	if f.inBase(tip) {
		return
	}
	tree, conflicts, err := f.repository.mergeTree(baseTip, tip)
	if err != nil {
		f.printf("warning: %s: can't merge %s: %v", id, specBranch(id), err)
		return
	}
	if len(conflicts) > 0 {
		f.openSpecConflict(ctx, spec, tip, baseTip, conflicts)
		return
	}
	merge, err := f.repository.commitTree(tree, fmt.Sprintf("Merge spec %s: %s", id, spec.Title), baseTip, tip)
	if err != nil {
		f.printf("warning: %s: can't make the merge commit: %v", id, err)
		return
	}
	if why := f.advanceBase(baseTip, merge); why != "" {
		for _, tk := range tickets {
			f.cantLandYet(ctx, tk, why)
		}
		return
	}
	f.printf("%s: merged %s into %s at %s", id, specBranch(id), f.base, short(merge))
	f.printf("%s: landing its Tickets: %s", id, joinIDs(tickets))
	for _, tk := range tickets {
		f.landPushed(ctx, tk, landing, merge)
	}
}

func joinIDs(tickets []core.TicketView) string {
	ids := make([]string, len(tickets))
	for i, tk := range tickets {
		ids[i] = tk.ID
	}
	return strings.Join(ids, ", ")
}

// openSpecConflict opens a Ready Ticket on the Spec to merge the base
// into its Spec Branch, or comments on the open one already there.
func (f *factory) openSpecConflict(ctx context.Context, spec core.SpecDetail, tip, baseTip string, conflicts []string) {
	title := fmt.Sprintf("Merge the base into the Spec Branch of %s", spec.ID)
	var b strings.Builder
	fmt.Fprintf(&b, "The Spec Branch %s conflicts with the base, %s, in these files:\n\n", specBranch(spec.ID), f.base)
	for _, c := range conflicts {
		fmt.Fprintf(&b, "- %s\n", c)
	}
	fmt.Fprintf(&b, "\nThe base's tip is %s and the Spec Branch's tip is %s. Merge the base into this branch and resolve the conflicts.\n", baseTip, tip)
	body := b.String()

	rows, err := f.client.FindTickets(ctx, f.project, core.FindTicketsInput{Spec: core.Ref(spec.ID), Title: title})
	if err != nil {
		if ctx.Err() == nil {
			f.printf("%s: find_tickets: %v", spec.ID, err)
		}
		return
	}
	for _, r := range rows {
		if r.Title != title || r.State == store.Done || r.State == store.Cancelled {
			continue
		}
		existing, err := f.client.GetTicket(ctx, f.project, core.Ref(r.ID))
		if err != nil {
			return
		}
		for _, c := range existing.Comments {
			if c.Body == body {
				return
			}
		}
		if _, err := f.client.Comment(ctx, f.project, core.CommentInput{Ticket: core.Ref(r.ID), Body: body}); err != nil && ctx.Err() == nil {
			f.printf("%s: comment: %v", r.ID, err)
			return
		}
		f.printf("%s: %s conflicts with %s again: commented on %s", spec.ID, specBranch(spec.ID), f.base, r.ID)
		return
	}
	made, err := f.client.CreateTicket(ctx, f.project, core.CreateTicketInput{Spec: core.Ref(spec.ID), Title: title, Description: body, State: store.Ready})
	if err != nil {
		if ctx.Err() == nil {
			f.printf("%s: create_ticket: %v", spec.ID, err)
		}
		return
	}
	f.printf("%s: %s conflicts with %s in %s: opened %s to merge the base into it", spec.ID, specBranch(spec.ID), f.base, strings.Join(conflicts, ", "), made.ID)
}
