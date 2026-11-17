package factory

import (
	"context"
	"fmt"
	"slices"
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

// specBranchMerge is a Ticket's merge into its Spec Branch in r: its work
// there, head, into the Spec Branch at tip, making tree; merged when the
// Spec Branch holds head already. Once the merge is made, tip is the
// merge commit, the Spec Branch's new tip.
type specBranchMerge struct {
	r               *repository
	head, tip, tree string
	merged          bool
}

// mergeIntoSpecBranch merges an Accepted Spec-landed Ticket's work into
// its Spec Branch in each of ws, its Changed Repositories, and reports
// the merge to Code Central once its work is in every one. Nothing
// reaches a base: the Spec lands later. It works out every merge first:
// a conflict in any merges it into none and sends the Ticket back to
// Ready (ADR 0049).
func (f *factory) mergeIntoSpecBranch(ctx context.Context, tk core.TicketView, ws []changedWork) {
	branch := specBranch(tk.Spec.ID)
	ref := "refs/heads/" + branch
	var merges []specBranchMerge
	var conflicts []landConflict
	for _, w := range ws {
		r := w.r
		head, what, err := f.workIn(r, tk)
		if err != nil {
			f.cantLandIn(ctx, tk, r, what+" isn't in this repository.")
			return
		}
		tip, err := f.specBranchTip(r, tk.Spec.ID)
		if err != nil {
			f.cantLandIn(ctx, tk, r, err.Error()+".")
			return
		}
		m := specBranchMerge{r: r, head: head, tip: tip, merged: r.git.isAncestor(head, ref)}
		if !m.merged {
			var files []string
			if m.tree, files, err = r.git.mergeTree(tip, head); err != nil {
				f.printf("warning: %s: can't merge%s: %v", tk.ID, r.where(), err)
				return
			}
			if len(files) > 0 {
				conflicts = append(conflicts, landConflict{r: r, head: head, into: branch, tip: tip, files: files})
				continue
			}
		}
		merges = append(merges, m)
	}
	if len(conflicts) > 0 {
		f.sendBackConflicts(ctx, tk, "Spec Branch", conflicts)
		return
	}
	for i, m := range merges {
		if m.merged {
			continue
		}
		merge, err := m.r.git.commitTree(m.tree, fmt.Sprintf("Merge %s: %s", tk.ID, tk.Title), m.tip, m.head)
		if err != nil {
			f.printf("warning: %s: can't make the merge commit%s: %v", tk.ID, m.r.where(), err)
			return
		}
		if err := m.r.git.updateRef(ref, merge, m.tip); err != nil {
			f.cantLandIn(ctx, tk, m.r, fmt.Sprintf("the Spec Branch %s moved while the Factory was merging, or git refused: %v", branch, err))
			return
		}
		merges[i].tip = merge
	}
	if len(merges) > 0 {
		f.reportSpecMerge(ctx, tk, branch, merges)
	}
}

// reportSpecMerge tells Code Central tk's work is in its Spec Branch in
// each of merges' Repositories, naming the first's tip.
func (f *factory) reportSpecMerge(ctx context.Context, tk core.TicketView, branch string, merges []specBranchMerge) {
	in := core.ReportSpecMergeInput{Ticket: core.Ref(tk.ID), Commit: merges[0].tip}
	if r := merges[0].r; !r.home {
		in.Repository = r.name
	}
	if _, err := f.client.ReportSpecMerge(ctx, f.project, in); err != nil {
		if ctx.Err() == nil {
			f.printf("%s: report_spec_merge: %v", tk.ID, err)
		}
		return
	}
	for _, m := range merges {
		f.printf("%s: merged into %s at %s%s", tk.ID, branch, short(m.tip), m.r.where())
	}
}

// waitsOnSpecBranch reports whether an Accepted tk goes through its Spec
// Branch rather than into the base: it is Spec-landed and its work isn't
// in every base yet (such as work accepted before Land per Spec was turned
// on, which lands as plain Merge). Core makes a Ticket Spec-landed only
// with Land per Spec in force.
func (f *factory) waitsOnSpecBranch(tk core.TicketView) bool {
	return tk.SpecLanded && !f.workInEveryBase(tk)
}

// workInEveryBase reports whether tk's work is in the base of each of its
// Changed Repositories it hasn't landed in.
func (f *factory) workInEveryBase(tk core.TicketView) bool {
	ws, why := f.changedWorkOf(tk)
	if why != "" {
		return false
	}
	for _, w := range ws {
		if w.landed != "" {
			continue
		}
		if head, _, err := f.workIn(w.r, tk); err != nil || !f.inBaseOf(w.r, head) {
			return false
		}
	}
	return true
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
// base, in each Repository its Tickets changed, and lands the Spec's
// waiting Tickets as a group, by the Push and CI gate path. A Spec's
// Tickets are the Accepted ones the Factory has merged into its Spec
// Branch and not yet landed.
func (f *factory) landSpecs(ctx context.Context, p core.ProjectView) {
	if !f.home.policy(p.LandingPolicies()).MergesSpecBranches() {
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
// pull request. With several Repositories each base is checked by
// pushSpec, for the Repositories the Spec changed.
func (f *factory) pushSpecs(ctx context.Context, accepted []core.TicketView, p core.ProjectView) {
	if !f.home.policy(p.LandingPolicies()).PushesSpecBranches() || !f.several() && !f.stateOf(f.home).current {
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

// pushSpec pushes one Spec's Spec Branch, in each Repository its waiting
// Tickets changed, when Code Central says the Spec is ready. Every Spec
// Branch is checked against its base first: a conflict in any pushes none
// and gets the Spec merge Ticket instead (ADR 0049, ADR 0050). One whose
// pull request has merged, or that is in its base, is left to landing.
func (f *factory) pushSpec(ctx context.Context, id string, p core.ProjectView) {
	spec, tickets, ok := f.readySpec(ctx, id)
	if !ok || f.anyUncloned(tickets) {
		return
	}
	sort.SliceStable(tickets, func(i, j int) bool { return acceptedAt(tickets[i]).Before(acceptedAt(tickets[j])) })
	policies := p.LandingPolicies()
	branch := specBranch(id)
	var plans, conflicts []specPlan
	for _, r := range f.readyRepositories() {
		in := ticketsIn(tickets, r)
		if len(in) == 0 || slices.ContainsFunc(in, func(tk core.TicketView) bool { return pullRequestMergedIn(tk, r) }) {
			continue // landing sees to it
		}
		if f.held(r, policies) != "" || !f.stateOf(r).current {
			return
		}
		tip, err := r.git.commit("refs/heads/" + branch)
		if err != nil || f.inBaseOf(r, tip) {
			continue
		}
		baseTip, err := r.git.commit("refs/heads/" + f.baseOf(r))
		if err != nil {
			return
		}
		pl := specPlan{r: r, tickets: in, tip: tip, baseTip: baseTip}
		if _, pl.conflicts, err = r.git.mergeTree(baseTip, tip); err != nil {
			f.printf("warning: %s: can't merge %s%s: %v", id, branch, r.where(), err)
			return
		}
		if len(pl.conflicts) > 0 {
			conflicts = append(conflicts, pl)
		}
		plans = append(plans, pl)
	}
	if len(conflicts) > 0 {
		f.openSpecConflict(ctx, spec, conflicts)
		return
	}
	for _, pl := range plans {
		pushed, why := f.push(ctx, pl.r, p.Landing.Remote, branch, pl.tip)
		if why != "" {
			for _, tk := range pl.tickets {
				f.cantPushIn(ctx, tk, pl.r, why)
			}
			continue
		}
		if pushed {
			f.printf("%s: pushed %s to %s%s", id, branch, p.Landing.Remote, pl.r.where())
		}
		for _, tk := range pl.tickets {
			f.reportPushed(ctx, tk, policies, pl.r, branch, f.remoteTip(pl.r, p.Landing.Remote, branch, pl.tip))
		}
	}
}

// pullRequestMergedIn reports whether tk's pull request in r has merged.
func pullRequestMergedIn(tk core.TicketView, r *repository) bool {
	v, _ := viewIn(tk, r)
	return hasMerged(v.PullRequest)
}

// readySpec is the Spec id with its waiting Tickets, when Code Central
// says it is ready to land; ok is false when it isn't, or it or one of
// its Tickets can't be read.
func (f *factory) readySpec(ctx context.Context, id string) (spec core.SpecDetail, tickets []core.TicketView, ok bool) {
	spec, err := f.client.GetSpec(ctx, f.project, core.GetSpecInput{Spec: core.Ref(id), Parts: []string{"tickets"}})
	if err != nil {
		if ctx.Err() == nil {
			f.printf("%s: get_spec: %v", id, err)
		}
		return spec, nil, false
	}
	if spec.LandPerSpec == nil || !spec.LandPerSpec.Ready {
		return spec, nil, false
	}
	for _, w := range spec.LandPerSpec.Waiting {
		tk, err := f.client.GetTicket(ctx, f.project, core.Ref(w))
		if err != nil {
			if ctx.Err() == nil {
				f.printf("%s: get_ticket: %v", w, err)
			}
			return spec, nil, false
		}
		tickets = append(tickets, tk)
	}
	return spec, tickets, true
}

// landSpec lands one Spec when Code Central says it is ready.
func (f *factory) landSpec(ctx context.Context, id string, p core.ProjectView) {
	spec, tickets, ok := f.readySpec(ctx, id)
	if !ok {
		return
	}
	for _, tk := range tickets {
		if tk.SpecMerge == nil || !tk.SpecMerge.Counts {
			// Accepted after this poll merged into Spec Branches: the
			// Spec lands once the next poll has merged it.
			return
		}
	}
	f.mergeSpec(ctx, spec, tickets, p)
}

// anyUncloned reports whether one of tickets has a Changed Repository it
// hasn't landed in with no clone, whose Spec Branch can't be checked: its
// Spec waits, as the Ticket's own line says.
func (f *factory) anyUncloned(tickets []core.TicketView) bool {
	return slices.ContainsFunc(tickets, func(tk core.TicketView) bool {
		_, why := f.changedWorkOf(tk)
		return why != ""
	})
}

// ticketsIn is those of tickets that changed r and haven't landed there.
func ticketsIn(tickets []core.TicketView, r *repository) []core.TicketView {
	var out []core.TicketView
	for _, tk := range tickets {
		if v, ok := viewIn(tk, r); ok && v.Landed == nil {
			out = append(out, tk)
		}
	}
	return out
}

// specPlan is how the Factory lands a Spec in r, worked out before it
// lands it in any Repository: tickets are the Spec's waiting Tickets that
// changed r and haven't landed there. They land at old, a merge it made
// into a base the Project no longer names; at once when the base holds
// the Spec Branch's tip already; or by merging the Spec Branch at tip
// into the base at baseTip, making tree, unless that conflicts.
type specPlan struct {
	r                  *repository
	tickets            []core.TicketView
	old                *mergeOn
	tip, baseTip, tree string
	inBase             bool
	conflicts          []string
}

// mergeSpec merges the Spec Branch of spec into the base of each
// Repository its waiting Tickets changed, and lands them there as a
// group. It works out every Repository's merge first: a conflict in any
// merges it nowhere and opens the one Spec merge Ticket, naming each (ADR
// 0049), though a merge made on an earlier poll still goes on to its push
// and land. Then, in landing order, it makes the same "Merge spec" commit
// in each, advances the base, pushes and lands, and stops at a Repository
// it can't land in yet, to go on from there next poll. With several
// Repositories, one with a broken base or no clone holds the whole Spec
// back, as it can't be checked. It reports whether
// the Tickets are handled this poll: false means every Spec Branch is
// already in its base or one can't be read, and the Tickets land through
// the plain path, as their work is in the base.
func (f *factory) mergeSpec(ctx context.Context, spec core.SpecDetail, tickets []core.TicketView, p core.ProjectView) (handled bool) {
	sort.SliceStable(tickets, func(i, j int) bool { return acceptedAt(tickets[i]).Before(acceptedAt(tickets[j])) })
	policies := p.LandingPolicies()
	if f.anyUncloned(tickets) {
		return true
	}
	var plans, conflicts []specPlan
	for _, r := range f.readyRepositories() {
		in := ticketsIn(tickets, r)
		if len(in) == 0 {
			continue
		}
		if f.several() && f.held(r, policies) != "" {
			return true // its Tickets say why they wait
		}
		pl, readable, ok := f.planSpec(spec.ID, r, in)
		if !ok {
			return readable
		}
		if len(pl.conflicts) > 0 {
			conflicts = append(conflicts, pl)
		}
		plans = append(plans, pl)
	}
	if !slices.ContainsFunc(plans, func(pl specPlan) bool { return !pl.inBase }) {
		// Merged on an earlier poll whose landing then failed: those Tickets
		// land through the plain path, as their work is in every base.
		return false
	}
	if len(conflicts) > 0 {
		f.openSpecConflict(ctx, spec, conflicts)
	}
	for _, pl := range plans {
		if len(conflicts) > 0 && pl.old == nil && !pl.inBase {
			// Nothing is merged while a Spec Branch conflicts, but a merge
			// made already goes on to its push and land.
			continue
		}
		if !f.landSpecIn(ctx, spec, p, pl) {
			break
		}
	}
	return true
}

// planSpec works out how the Spec id lands tickets in r. ok is false when
// it can't, and then handled is mergeSpec's answer: false when the Spec
// Branch can't be read, true with a line saying why otherwise.
func (f *factory) planSpec(id string, r *repository, tickets []core.TicketView) (pl specPlan, handled, ok bool) {
	pl = specPlan{r: r, tickets: tickets}
	tip, err := r.git.commit("refs/heads/" + specBranch(id))
	if err != nil {
		return pl, false, false
	}
	pl.tip = tip
	if old, ok := f.mergedOnOldBase(r, tickets[0].ID); ok {
		// Merged before the base changed, so finishing on its push or CI.
		pl.old = &old
		return pl, true, true
	}
	base := f.baseOf(r)
	if pl.baseTip, err = r.git.commit("refs/heads/" + base); err != nil {
		f.printf("warning: %s: the base, %s, isn't a branch%s, so the Spec can't land", id, base, r.where())
		return pl, true, false
	}
	if f.inBaseOf(r, tip) {
		pl.inBase = true
		return pl, true, true
	}
	if pl.tree, pl.conflicts, err = r.git.mergeTree(pl.baseTip, tip); err != nil {
		f.printf("warning: %s: can't merge %s%s: %v", id, specBranch(id), r.where(), err)
		return pl, true, false
	}
	return pl, true, true
}

// landSpecIn lands spec's Tickets in pl's Repository as pl says, making
// the Spec's merge and advancing the base first when it needs one. It
// reports whether landing can go on to the next Repository.
func (f *factory) landSpecIn(ctx context.Context, spec core.SpecDetail, p core.ProjectView, pl specPlan) bool {
	r, base := pl.r, f.baseOf(pl.r)
	switch {
	case pl.old != nil:
		// It lands on the base it was merged into.
		return f.landEachAt(ctx, pl.tickets, p, r, pl.old.base, pl.old.commit)
	case pl.inBase:
		// Merged here on an earlier poll whose landing then failed.
		commit := pl.tip
		if m, ok := f.stateOf(r).mergedOn[pl.tickets[0].ID]; ok && m.base == base {
			commit = m.commit
		}
		return f.landEachAt(ctx, pl.tickets, p, r, base, commit)
	}
	merge, err := r.git.commitTree(pl.tree, fmt.Sprintf("Merge spec %s: %s", spec.ID, spec.Title), pl.baseTip, pl.tip)
	if err != nil {
		f.printf("warning: %s: can't make the merge commit%s: %v", spec.ID, r.where(), err)
		return false
	}
	if why := f.advanceBase(r, pl.baseTip, merge); why != "" {
		for _, tk := range pl.tickets {
			f.cantLandIn(ctx, tk, r, why)
		}
		return false
	}
	f.printf("%s: merged %s into %s at %s%s", spec.ID, specBranch(spec.ID), base, short(merge), r.where())
	f.printf("%s: landing its Tickets%s: %s", spec.ID, r.where(), joinIDs(pl.tickets))
	for _, tk := range pl.tickets {
		f.stateOf(r).mergedOn[tk.ID] = mergeOn{base, merge}
	}
	return f.landEachAt(ctx, pl.tickets, p, r, base, merge)
}

// landEachAt lands each of tickets in r at commit, which is in r's base,
// and reports whether landing can go on to the next Repository: every
// one's push and land call went through.
func (f *factory) landEachAt(ctx context.Context, tickets []core.TicketView, p core.ProjectView, r *repository, base, commit string) bool {
	ok := true
	for _, tk := range tickets {
		ok = f.landPushed(ctx, tk, p, r, base, commit) && ok
	}
	return ok
}

// landSpecsOff is the poll's landing under Merge with Land per Spec
// turned off part-way, or a Spec gaining the long-lived Label: every Spec
// Branch holding Accepted Tickets merged and not landed, which Land per
// Spec no longer covers, goes into the base, ready or not, and those
// Tickets land as a group. It returns the Tickets it handled, which the plain path
// leaves alone this poll.
func (f *factory) landSpecsOff(ctx context.Context, accepted []core.TicketView, p core.ProjectView) map[string]bool {
	handled := map[string]bool{}
	if !f.home.policy(p.LandingPolicies()).MergesLeftoverSpecBranches() {
		return handled
	}
	groups := map[string][]core.TicketView{}
	var specs []string
	for _, tk := range accepted {
		if tk.SpecMerge == nil || !tk.SpecMerge.Counts {
			continue
		}
		if tk.SpecLanded && p.LandingPolicies().LandPerSpecInForce() {
			continue
		}
		if f.workInEveryBase(tk) {
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
// open one already there. With several Repositories it names each of
// conflicts' Repositories, with its files and both tips.
func (f *factory) openSpecConflict(ctx context.Context, spec core.SpecDetail, conflicts []specPlan) {
	several := f.several()
	in := core.OpenSpecMergeInput{Spec: core.Ref(spec.ID)}
	var names []string
	if several {
		for _, pl := range conflicts {
			in.Repositories = append(in.Repositories, core.SpecMergeConflict{Name: pl.r.name, Conflicts: pl.conflicts, BaseTip: pl.baseTip, SpecBranchTip: pl.tip})
			names = append(names, pl.r.name)
		}
	} else {
		in.Conflicts, in.BaseTip, in.SpecBranchTip = conflicts[0].conflicts, conflicts[0].baseTip, conflicts[0].tip
	}
	res, err := f.client.OpenSpecMerge(ctx, f.project, in)
	if err != nil {
		if ctx.Err() == nil {
			f.printf("%s: open_spec_merge: %v", spec.ID, err)
		}
		return
	}
	// Where the Spec Branch conflicts: in each named Repository, or with
	// the Home Repository alone, with the base in these files.
	branch, with, again := specBranch(spec.ID), f.baseBranch()+" in "+strings.Join(in.Conflicts, ", "), f.baseBranch()+" again"
	if several {
		with = "the base in " + strings.Join(names, ", ")
		again = with + " again"
	}
	switch res.Outcome {
	case core.SpecMergeOpened:
		f.printf("%s: %s conflicts with %s: opened %s to merge the base into it", spec.ID, branch, with, res.Ticket)
	case core.SpecMergeCommented:
		f.printf("%s: %s conflicts with %s: commented on %s", spec.ID, branch, again, res.Ticket)
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
func (f *factory) sweepSpecBranches(ctx context.Context, policies core.LandingPolicies) {
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
				f.sweepSpecBranch(r, branch, *spec, policies)
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
func (f *factory) sweepSpecBranch(r *repository, branch string, spec core.SpecDetail, policies core.LandingPolicies) {
	policy := r.policy(policies)
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
