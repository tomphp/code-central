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

// specBranchTip makes sure the Spec Branch of spec exists in r and
// returns its tip. A missing one is created at r's base's tip; one
// already in the base (after an earlier landing of the Spec) is
// fast-forwarded to the base's tip first. The branch is never checked
// out: only its ref moves.
func (f *factory) specBranchTip(r *repository, spec string) (string, error) {
	ref := "refs/heads/" + specBranch(spec)
	base := f.baseOf(r)
	baseTip, err := r.git.commit("refs/heads/" + base)
	if err != nil {
		return "", fmt.Errorf("the base, %s, isn't a branch", base)
	}
	tip, err := r.git.commit(ref)
	if err != nil {
		if err := r.git.updateRef(ref, baseTip, ""); err != nil {
			return "", fmt.Errorf("can't create the Spec Branch %s (a branch named %q may be in the way): %v", specBranch(spec), "spec", err)
		}
		return baseTip, nil
	}
	if tip != baseTip && f.inBaseOf(r, tip) {
		if err := r.git.updateRef(ref, baseTip, tip); err != nil {
			return "", fmt.Errorf("can't move the Spec Branch %s to %s's tip: %v", specBranch(spec), base, err)
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
	tip, err := f.specBranchTip(f.home, tk.Spec.ID)
	if err != nil {
		f.cantLandYet(ctx, tk, err.Error()+".")
		return
	}
	if f.home.git.isAncestor(head, ref) {
		f.reportSpecMerge(ctx, tk, branch, tip)
		return
	}
	tree, conflicts, err := f.home.git.mergeTree(tip, head)
	if err != nil {
		f.printf("warning: %s: can't merge: %v", tk.ID, err)
		return
	}
	if len(conflicts) > 0 {
		f.sendBack(ctx, tk, head, branch, tip, conflicts)
		return
	}
	merge, err := f.home.git.commitTree(tree, fmt.Sprintf("Merge %s: %s", tk.ID, tk.Title), tip, head)
	if err != nil {
		f.printf("warning: %s: can't make the merge commit: %v", tk.ID, err)
		return
	}
	if err := f.home.git.updateRef(ref, merge, tip); err != nil {
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
// on, which lands as plain Merge). Core makes a Ticket Spec-landed only
// with Land per Spec in force.
func (f *factory) waitsOnSpecBranch(tk core.TicketView) bool {
	if !tk.SpecLanded {
		return false
	}
	head, _, err := f.workOf(tk)
	return err != nil || !f.inBase(head)
}

// cutBranchWorktree gives a claimed Ticket its worktree in r on branch. A
// new branch is cut from r's base's tip, or for a Spec-landed Ticket from
// its Spec Branch's tip in r; an existing branch is reused as it is.
func (f *factory) cutBranchWorktree(r *repository, tk core.TicketView, branch, wt string) error {
	from := f.baseOf(r)
	if tk.SpecLanded {
		if _, err := r.git.commit("refs/heads/" + branch); err != nil {
			tip, err := f.specBranchTip(r, tk.Spec.ID)
			if err != nil {
				return specBranchFailed(wt, err)
			}
			from = tip
		}
	}
	return f.branchWorktree(r, branch, wt, from)
}

// landSpecs merges the Spec Branch of every Spec ready to land into the
// base and lands the Spec's waiting Tickets as a group, by the Push and
// CI gate path. A Spec's Tickets are the Accepted ones the Factory has
// merged into its Spec Branch and not yet landed.
func (f *factory) landSpecs(ctx context.Context, p core.ProjectView) {
	if !p.LandingPolicy().MergesSpecBranches() {
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
		f.landSpec(ctx, id, p)
	}
}

// pushSpecs, under Pull Request with Land per Spec on, pushes the Spec
// Branch of every Spec ready to land, once the base is level with the
// remote's, for the Pull Request Watcher to open one pull request on. A
// Spec Branch that conflicts with the base gets its Spec merge Ticket
// instead. A later merge into it is pushed again, which updates the
// pull request.
func (f *factory) pushSpecs(ctx context.Context, accepted []core.TicketView, p core.ProjectView) {
	if !p.LandingPolicy().PushesSpecBranches() || !f.stateOf(f.home).current {
		return
	}
	seen := map[string]bool{}
	for _, tk := range accepted {
		if !tk.SpecLanded || seen[tk.Spec.ID] {
			continue
		}
		seen[tk.Spec.ID] = true
		if f.halted(ctx) || f.paused() {
			return
		}
		f.pushSpec(ctx, tk.Spec.ID, p)
	}
}

// pushSpec pushes one Spec's Spec Branch when Code Central says the Spec
// is ready.
func (f *factory) pushSpec(ctx context.Context, id string, p core.ProjectView) {
	spec, err := f.client.GetSpec(ctx, f.project, core.GetSpecInput{Spec: core.Ref(id), Parts: []string{"tickets"}})
	if err != nil {
		if ctx.Err() == nil {
			f.printf("%s: get_spec: %v", id, err)
		}
		return
	}
	if spec.LandPerSpec == nil || !spec.LandPerSpec.Ready {
		return
	}
	var tickets []core.TicketView
	for _, w := range spec.LandPerSpec.Waiting {
		tk, err := f.client.GetTicket(ctx, f.project, core.Ref(w))
		if err != nil {
			if ctx.Err() == nil {
				f.printf("%s: get_ticket: %v", w, err)
			}
			return
		}
		if tk.PullRequest != nil && tk.PullRequest.State == store.PRMerged {
			return // landing sees to it
		}
		tickets = append(tickets, tk)
	}
	sort.SliceStable(tickets, func(i, j int) bool { return acceptedAt(tickets[i]).Before(acceptedAt(tickets[j])) })
	branch := specBranch(id)
	tip, err := f.home.git.commit("refs/heads/" + branch)
	if err != nil || f.inBase(tip) {
		return
	}
	baseTip, err := f.home.git.commit("refs/heads/" + f.baseBranch())
	if err != nil {
		return
	}
	_, conflicts, err := f.home.git.mergeTree(baseTip, tip)
	if err != nil {
		f.printf("warning: %s: can't merge %s: %v", id, branch, err)
		return
	}
	if len(conflicts) > 0 {
		f.openSpecConflict(ctx, spec, tip, baseTip, conflicts)
		return
	}
	pushed, why := f.push(ctx, f.home, p.Landing.Remote, branch, tip)
	if why != "" {
		for _, tk := range tickets {
			f.cantPushYet(ctx, tk, why)
		}
		return
	}
	if pushed {
		f.printf("%s: pushed %s to %s", id, branch, p.Landing.Remote)
	}
	for _, tk := range tickets {
		f.reportPushed(ctx, tk, p.LandingPolicy(), f.home, branch, f.remoteTip(f.home, p.Landing.Remote, branch, tip))
	}
}

// landSpec lands one Spec when Code Central says it is ready.
func (f *factory) landSpec(ctx context.Context, id string, p core.ProjectView) {
	spec, err := f.client.GetSpec(ctx, f.project, core.GetSpecInput{Spec: core.Ref(id), Parts: []string{"tickets"}})
	if err != nil {
		if ctx.Err() == nil {
			f.printf("%s: get_spec: %v", id, err)
		}
		return
	}
	if spec.LandPerSpec == nil || !spec.LandPerSpec.Ready {
		return
	}
	var tickets []core.TicketView
	for _, w := range spec.LandPerSpec.Waiting {
		tk, err := f.client.GetTicket(ctx, f.project, core.Ref(w))
		if err != nil {
			if ctx.Err() == nil {
				f.printf("%s: get_ticket: %v", w, err)
			}
			return
		}
		if tk.SpecMerge == nil || !tk.SpecMerge.Counts {
			// Accepted after this poll merged into Spec Branches: the
			// Spec lands once the next poll has merged it.
			return
		}
		tickets = append(tickets, tk)
	}
	f.mergeSpec(ctx, spec, tickets, p)
}

// mergeSpec merges the Spec Branch of spec into the base and lands its
// waiting Tickets as a group. It reports whether the Tickets are handled
// this poll: false means the Spec Branch is already in the base or can't
// be read, and the Tickets land through the plain path, as their work is
// in the base.
func (f *factory) mergeSpec(ctx context.Context, spec core.SpecDetail, tickets []core.TicketView, p core.ProjectView) (handled bool) {
	id := spec.ID
	sort.SliceStable(tickets, func(i, j int) bool { return acceptedAt(tickets[i]).Before(acceptedAt(tickets[j])) })
	ref := "refs/heads/" + specBranch(id)
	tip, err := f.home.git.commit(ref)
	if err != nil {
		return false
	}
	if old, ok := f.mergedOnOldBase(f.home, firstID(tickets)); ok {
		// Merged before the base changed, so finishing on its push or CI:
		// it lands on the base it was merged into.
		for _, tk := range tickets {
			f.landPushed(ctx, tk, p, f.home, old.base, old.commit)
		}
		return true
	}
	baseTip, err := f.home.git.commit("refs/heads/" + f.baseBranch())
	if err != nil {
		f.printf("warning: %s: the base, %s, isn't a branch, so the Spec can't land", id, f.baseBranch())
		return true
	}
	// Merged on an earlier poll whose landing then failed: those Tickets
	// land through the plain path, as their work is in the base.
	if f.inBase(tip) {
		return false
	}
	tree, conflicts, err := f.home.git.mergeTree(baseTip, tip)
	if err != nil {
		f.printf("warning: %s: can't merge %s: %v", id, specBranch(id), err)
		return true
	}
	if len(conflicts) > 0 {
		f.openSpecConflict(ctx, spec, tip, baseTip, conflicts)
		return true
	}
	merge, err := f.home.git.commitTree(tree, fmt.Sprintf("Merge spec %s: %s", id, spec.Title), baseTip, tip)
	if err != nil {
		f.printf("warning: %s: can't make the merge commit: %v", id, err)
		return true
	}
	if why := f.advanceBase(f.home, baseTip, merge); why != "" {
		for _, tk := range tickets {
			f.cantLandYet(ctx, tk, why)
		}
		return true
	}
	f.printf("%s: merged %s into %s at %s", id, specBranch(id), f.baseBranch(), short(merge))
	f.printf("%s: landing its Tickets: %s", id, joinIDs(tickets))
	for _, tk := range tickets {
		f.stateOf(f.home).mergedOn[tk.ID] = mergeOn{f.baseBranch(), merge}
		f.landPushed(ctx, tk, p, f.home, f.baseBranch(), merge)
	}
	return true
}

// landSpecsOff is the poll's landing under Merge with Land per Spec
// turned off part-way, or a Spec gaining the long-lived Label: every Spec
// Branch holding Accepted Tickets merged and not landed, which Land per
// Spec no longer covers, goes into the base, ready or not, and those
// Tickets land as a group. It returns the Tickets it handled, which the plain path
// leaves alone this poll.
func (f *factory) landSpecsOff(ctx context.Context, accepted []core.TicketView, p core.ProjectView) map[string]bool {
	handled := map[string]bool{}
	if !p.LandingPolicy().MergesLeftoverSpecBranches() {
		return handled
	}
	groups := map[string][]core.TicketView{}
	var specs []string
	for _, tk := range accepted {
		if tk.SpecMerge == nil || !tk.SpecMerge.Counts {
			continue
		}
		if tk.SpecLanded && p.LandingPolicy().LandPerSpecInForce() {
			continue
		}
		if head, _, err := f.workOf(tk); err == nil && f.inBase(head) {
			continue
		}
		if len(groups[tk.Spec.ID]) == 0 {
			specs = append(specs, tk.Spec.ID)
		}
		groups[tk.Spec.ID] = append(groups[tk.Spec.ID], tk)
	}
	for _, id := range specs {
		if f.halted(ctx) || f.paused() {
			break
		}
		spec, err := f.client.GetSpec(ctx, f.project, core.GetSpecInput{Spec: core.Ref(id), Parts: []string{"tickets"}})
		if err != nil {
			if ctx.Err() == nil {
				f.printf("%s: get_spec: %v", id, err)
			}
			for _, tk := range groups[id] {
				handled[tk.ID] = true
			}
			continue
		}
		if f.mergeSpec(ctx, spec, groups[id], p) {
			for _, tk := range groups[id] {
				handled[tk.ID] = true
			}
		}
	}
	return handled
}

func joinIDs(tickets []core.TicketView) string {
	ids := make([]string, len(tickets))
	for i, tk := range tickets {
		ids[i] = tk.ID
	}
	return strings.Join(ids, ", ")
}

// openSpecConflict asks Code Central to open the Spec merge Ticket that
// merges the base into the Spec Branch, or to comment the conflict on the
// open one already there.
func (f *factory) openSpecConflict(ctx context.Context, spec core.SpecDetail, tip, baseTip string, conflicts []string) {
	res, err := f.client.OpenSpecMerge(ctx, f.project, core.OpenSpecMergeInput{Spec: core.Ref(spec.ID), Conflicts: conflicts, BaseTip: baseTip, SpecBranchTip: tip})
	if err != nil {
		if ctx.Err() == nil {
			f.printf("%s: open_spec_merge: %v", spec.ID, err)
		}
		return
	}
	switch res.Outcome {
	case core.SpecMergeOpened:
		f.printf("%s: %s conflicts with %s in %s: opened %s to merge the base into it", spec.ID, specBranch(spec.ID), f.baseBranch(), strings.Join(conflicts, ", "), res.Ticket)
	case core.SpecMergeCommented:
		f.printf("%s: %s conflicts with %s again: commented on %s", spec.ID, specBranch(spec.ID), f.baseBranch(), res.Ticket)
	}
}

// specLanded reports whether a Ticket of spec has landed, which under
// Pull Request means its pull request merged: the Spec Branch then holds
// nothing the base lacks, even after a squash merge.
func (f *factory) specLanded(spec core.SpecDetail) bool {
	for _, r := range spec.Tickets {
		if r.State == store.Done {
			return true
		}
	}
	return false
}

// sweepSpecBranches deletes, in every Repository with a clone, each Spec
// Branch that is in that Repository's base once every Ticket on its Spec
// is Done or Cancelled. One not in the base, as when the whole Spec is
// cancelled, is kept, with a line printed once.
func (f *factory) sweepSpecBranches(ctx context.Context, policy core.LandingPolicy) {
	specs := map[string]*core.SpecDetail{}
	for _, r := range f.readyRepositories() {
		branches, err := r.git.specBranches()
		if err != nil {
			continue
		}
		for _, branch := range branches {
			if ctx.Err() != nil {
				return
			}
			id := strings.TrimPrefix(branch, "spec/")
			spec, ok := specs[id]
			if !ok {
				spec = f.finishedSpec(ctx, id)
				specs[id] = spec
			}
			if spec != nil {
				f.sweepSpecBranch(r, branch, *spec, policy)
			}
		}
	}
}

// finishedSpec is the Spec id when every Ticket on it is Done or
// Cancelled, and nil when one isn't, it has none, or it can't be read.
func (f *factory) finishedSpec(ctx context.Context, id string) *core.SpecDetail {
	spec, err := f.client.GetSpec(ctx, f.project, core.GetSpecInput{Spec: core.Ref(id), Parts: []string{"tickets"}})
	if err != nil || len(spec.Tickets) == 0 {
		return nil
	}
	for _, r := range spec.Tickets {
		if r.State != store.Done && r.State != store.Cancelled {
			return nil
		}
	}
	return &spec
}

// sweepSpecBranch deletes branch, the Spec Branch of the finished spec in
// r, when it is in r's base, and otherwise says once that it is kept.
func (f *factory) sweepSpecBranch(r *repository, branch string, spec core.SpecDetail, policy core.LandingPolicy) {
	id, base := strings.TrimPrefix(branch, "spec/"), f.baseOf(r)
	ref := "refs/heads/" + branch
	tip, err := r.git.commit(ref)
	if err != nil {
		return
	}
	where := r.where()
	if !f.inBaseOf(r, tip) && !(policy.LandsByPullRequest() && f.specLanded(spec)) {
		if warned := ref + where; !f.warned[warned] {
			f.warned[warned] = true
			f.printf("%s: every Ticket is finished but %s%s isn't in %s: kept", id, branch, where, base)
		}
		return
	}
	if err := r.git.deleteRef(ref, tip); err != nil {
		f.printf("warning: %s: can't delete %s%s: %v", id, branch, where, err)
		return
	}
	f.printf("%s: deleted %s%s, which is in %s", id, branch, where, base)
}

// firstID is the ID of the first of tickets, or "".
func firstID(tickets []core.TicketView) string {
	if len(tickets) == 0 {
		return ""
	}
	return tickets[0].ID
}
