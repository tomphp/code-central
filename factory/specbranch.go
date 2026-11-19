package factory

import (
	"context"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
)

// specRefOf is spec as a Ticket names its Spec, with its Spec Branch, the
// branch where its Spec-landed Tickets' work gathers under Land per Spec,
// as core names it: its branch Tag's value, else spec/<id>.
func specRefOf(spec core.SpecDetail) core.SpecRef {
	return core.SpecRef{ID: spec.ID, Title: spec.Title, Branch: core.SpecBranch(spec.ID, spec.Tags)}
}

// specBranchPushedIn is the Repositories where the Factory pushed spec's
// Spec Branch, as its Tickets' pushed commits say.
func specBranchPushedIn(spec core.SpecDetail) []string {
	if spec.LandPerSpec == nil {
		return nil
	}
	return spec.LandPerSpec.Pushed
}

// specBranchTip makes sure the Spec Branch branch exists in r and returns
// its tip. A missing one is created at r's base's tip; one already in the
// base (after an earlier landing of the Spec) is fast-forwarded to the
// base's tip first. The branch is never checked out: only its ref moves.
func (rs *repositories) specBranchTip(r *repository, branch string) (string, error) {
	ref := "refs/heads/" + branch
	base := rs.baseOf(r)
	baseTip, err := r.git.commit("refs/heads/" + base)
	if err != nil {
		return "", fmt.Errorf("the base, %s, isn't a branch", base)
	}
	tip, err := r.git.commit(ref)
	if err != nil {
		if err := r.git.updateRef(ref, baseTip, ""); err != nil {
			return "", fmt.Errorf("can't create the Spec Branch %s%s: %v", branch, inTheWayHint(branch), err)
		}
		return baseTip, nil
	}
	if tip != baseTip && rs.inBaseOf(r, tip) {
		if err := r.git.updateRef(ref, baseTip, tip); err != nil {
			return "", fmt.Errorf("can't move the Spec Branch %s to %s's tip: %v", branch, base, err)
		}
		return baseTip, nil
	}
	return tip, nil
}

// inTheWayHint is the hint, for a Spec Branch git can't create, of the
// branches that may be in the way: one named for any folder of its name, as
// spec is for spec/<id>, and a/b or a for a/b/c. A name with no folder gets
// none.
func inTheWayHint(branch string) string {
	var dirs []string
	for dir := path.Dir(branch); dir != "."; dir = path.Dir(dir) {
		dirs = append([]string{fmt.Sprintf("%q", dir)}, dirs...)
	}
	if len(dirs) == 0 {
		return ""
	}
	return fmt.Sprintf(" (a branch named %s may be in the way)", strings.Join(dirs, " or "))
}

// specBranchFailed is the failure for a Spec Branch the Factory can't
// create or move at a Claim.
func specBranchFailed(wt string, err error) *failure {
	return &failure{core.ReportFailureInput{
		WhatBroke:       fmt.Sprintf("The Factory couldn't set up this Ticket's Spec Branch: %v", err),
		Why:             "Under Land per Spec a Ticket's branch is cut from its Spec's Spec Branch, which the Factory creates and moves by ref, and git refused.",
		WhatToFix:       "Read the git error above and fix what it names in the repository (a branch in the way of the Spec Branch's name, a stale lock file). Then send the Ticket back.",
		WhereWorkStands: fmt.Sprintf("No session ran under this Claim. The Factory stopped before adding the worktree %s and touched nothing after the failure.", wt),
	}}
}

// specBranchMerge is a Ticket's merge into its Spec Branch in r: its work
// there, head, into the Spec Branch at tip, making tree, or under Rebase
// replayed onto it as replay says, or, for a Spec merge Ticket under
// Rebase, the Spec Branch moved to its rebased result as replacement
// says; merged when the Spec Branch holds head already, or held it
// replayed and the merge is reported. Once the merge, replay or move is
// made, tip is the Spec Branch's new tip. replayed says the work went in
// by a replay, and replaced that the Spec Branch was moved to it.
type specBranchMerge struct {
	r                          *repository
	head, tip, tree            string
	replay                     *replay
	replacement                *specBranchReplacement
	merged, replayed, replaced bool
}

// mergeIntoSpecBranch merges an Accepted Spec-landed Ticket's work into
// its Spec Branch in each of ws, the Changed Repositories it lands in by
// Spec Branch, but for one it has landed in, and reports the merge to
// Code Central once its work is in every one, unless it is reported
// already: a Repository can come under Land per Spec after the Ticket was
// merged elsewhere, as when its Own Landing changes. Where Rebase is in
// force it replays the work onto the Spec Branch instead (planReplay),
// moving the Ticket's branch to the replayed commits and fast-forwarding
// the Spec Branch to them. Work replayed, once reported, is on its Spec
// Branch even after the Spec Branch is rewritten, so it is never
// replayed twice. A Spec merge Ticket's work under Rebase is the Spec
// Branch rebased onto the base: the Spec Branch moves to it instead, with
// what it gained while the Ticket was open replayed onto it first
// (planSpecBranchReplacement); when that conflicts, the Ticket is sent
// back to bring those commits onto its branch itself, and its fork point
// moves on to them (takeSpecBranchAsBrought). Nothing reaches a base:
// the Spec lands later. It works
// out every merge first: a conflict in any merges it into none and sends
// the Ticket back to Ready (ADR 0049). ok is false when it didn't merge
// into every one, so the Ticket's other landing steps wait.
func (l *lander) mergeIntoSpecBranch(ctx context.Context, tk core.TicketView, policies core.LandingPolicies, ws []changedWork) (ok bool) {
	branch := tk.Spec.Branch
	ref := "refs/heads/" + branch
	var merges []specBranchMerge
	var conflicts []landConflict
	reported := tk.SpecMerge != nil && tk.SpecMerge.Counts
	for _, w := range ws {
		r := w.r
		head, replayed, what, err := l.replayedWorkIn(r, tk)
		if err != nil {
			l.cantLandIn(ctx, tk, r, what+" isn't in this repository.")
			return false
		}
		if w.landed != "" && r.git.isAncestor(head, w.landed) {
			continue
		}
		tip, err := l.repos.specBranchTip(r, branch)
		if err != nil {
			l.cantLandIn(ctx, tk, r, err.Error()+".")
			return false
		}
		// Replayed work, once reported, is merged though a rewrite of the
		// Spec Branch left it off.
		m := specBranchMerge{r: r, head: head, tip: tip, replayed: replayed, merged: r.git.isAncestor(head, ref) || (replayed && reported)}
		if !m.merged {
			var files []string
			rebase := r.policy(policies).RebaseInForce()
			switch {
			case rebase && tk.IsSpecMergeTicket():
				// Its agent rebased the Spec Branch onto the base: the
				// Spec Branch moves to that result, with whatever it
				// gained meanwhile replayed onto it.
				var why string
				if m.replacement, files, why, err = planSpecBranchReplacement(r, tk.ID, branch, head, tip); err != nil {
					l.printf("warning: %s: can't replay %s onto its rebased result%s: %v", tk.ID, branch, r.where(), err)
					return false
				}
				if why != "" {
					l.cantLandIn(ctx, tk, r, why)
					return false
				}
				if len(files) > 0 {
					conflicts = append(conflicts, landConflict{r: r, head: head, into: branch, tip: tip, fork: m.replacement.fork, files: files, rebase: true, replaying: true, replacing: true})
					continue
				}
			case rebase:
				var why string
				if m.replay, files, why, err = l.planReplay(r, tk.ID, branchIn(tk, r), head, tip); err != nil {
					l.printf("warning: %s: can't replay%s: %v", tk.ID, r.where(), err)
					return false
				}
				if why != "" {
					l.cantLandIn(ctx, tk, r, why)
					return false
				}
			default:
				if m.tree, files, err = r.git.mergeTree(tip, head); err != nil {
					l.printf("warning: %s: can't merge%s: %v", tk.ID, r.where(), err)
					return false
				}
			}
			if len(files) > 0 {
				conflicts = append(conflicts, landConflict{r: r, head: head, into: branch, tip: tip, files: files, rebase: rebase, replaying: rebase})
				continue
			}
		}
		merges = append(merges, m)
	}
	if len(conflicts) > 0 {
		if l.sendBackConflicts(ctx, tk, "Spec Branch", conflicts) {
			l.takeSpecBranchAsBrought(tk, conflicts)
		}
		return false
	}
	for i, m := range merges {
		if m.merged {
			continue
		}
		var next string
		switch {
		case m.replacement != nil:
			next, merges[i].replaced = m.replacement.next, true
		case m.replay != nil:
			if err := m.replay.apply(m.r, tk.ID, m.tip); err != nil {
				l.cantLandIn(ctx, tk, m.r, err.Error()+".")
				return false
			}
			next, merges[i].replayed = m.replay.tip, true
		default:
			merge, err := m.r.git.commitTree(m.tree, fmt.Sprintf("Merge %s: %s", tk.ID, tk.Title), m.tip, m.head)
			if err != nil {
				l.printf("warning: %s: can't make the merge commit%s: %v", tk.ID, m.r.where(), err)
				return false
			}
			next = merge
		}
		if err := m.r.git.updateRef(ref, next, m.tip); err != nil {
			l.cantLandIn(ctx, tk, m.r, fmt.Sprintf("the Spec Branch %s moved while the Factory was merging, or git refused: %v", branch, err))
			return false
		}
		merges[i].tip = next
		reported = false
	}
	if len(merges) > 0 && !reported {
		l.reportSpecMerge(ctx, tk, branch, merges)
	}
	return true
}

// takeSpecBranchAsBrought moves a Spec merge Ticket's fork point to its
// Spec Branch's tip in each Repository where replaying the Spec Branch's
// later commits onto its head conflicted, now that it is sent back to
// bring them onto its branch itself: when it finishes again, the branch
// holds them, and only what the Spec Branch gains from here on is
// replayed onto it. Otherwise the same commits would conflict again
// however they were resolved.
func (l *lander) takeSpecBranchAsBrought(tk core.TicketView, conflicts []landConflict) {
	for _, c := range conflicts {
		if !c.replacing {
			continue
		}
		if err := setForkPoint(c.r, tk.ID, c.tip); err != nil {
			l.printf("warning: %s: can't move its fork point to %s%s: %v", tk.ID, short(c.tip), c.r.where(), err)
		}
	}
}

// reportSpecMerge tells Code Central tk's work is in its Spec Branch in
// each of merges' Repositories, naming the first's tip.
func (l *lander) reportSpecMerge(ctx context.Context, tk core.TicketView, branch string, merges []specBranchMerge) {
	in := core.ReportSpecMergeInput{Ticket: core.Ref(tk.ID), Commit: merges[0].tip}
	if r := merges[0].r; !r.home {
		in.Repository = r.name
	}
	if _, err := l.client.ReportSpecMerge(ctx, l.project, in); err != nil {
		if ctx.Err() == nil {
			l.printf("%s: report_spec_merge: %v", tk.ID, err)
		}
		return
	}
	for _, m := range merges {
		switch {
		case m.replaced:
			l.printf("%s: moved %s to its rebased result at %s%s", tk.ID, branch, short(m.tip), m.r.where())
		case m.replayed:
			l.printf("%s: replayed onto %s at %s%s", tk.ID, branch, short(m.tip), m.r.where())
		default:
			l.printf("%s: merged into %s at %s%s", tk.ID, branch, short(m.tip), m.r.where())
		}
	}
}

// waitsOnSpecBranch reports whether an Accepted tk goes through its Spec
// Branch rather than into the base: it is Spec-landed and its work isn't
// in every base yet (such as work accepted before Land per Spec was turned
// on, which lands as plain Merge). Core makes a Ticket Spec-landed only
// with Land per Spec in force.
func (l *lander) waitsOnSpecBranch(tk core.TicketView) bool {
	return tk.SpecLanded && !l.workInEveryBase(tk)
}

// workInEveryBase reports whether tk's work is in the base of each of its
// Changed Repositories it hasn't landed in.
func (l *lander) workInEveryBase(tk core.TicketView) bool {
	ws, why := l.changedWorkOf(tk)
	if why != "" {
		return false
	}
	for _, w := range ws {
		if w.landed != "" {
			continue
		}
		if head, _, err := l.workIn(w.r, tk); err != nil || !l.repos.inBaseOf(w.r, head) {
			return false
		}
	}
	return true
}

// landSpecs lands every Spec ready to land in each Repository its
// Tickets changed, by that Repository's Landing Policy (Spec
// code-central-890): under Merge it merges the Spec Branch into the base
// and lands the Spec's waiting Tickets there as a group, by the Push and
// CI gate path; under Pull Request it pushes the Spec Branch for the
// Spec's one pull request, and the Tickets land there as it merges; under
// Branch, where Land per Spec has no effect, each Ticket lands once the
// human merges it. A Spec's Tickets are the Accepted ones the Factory has
// merged into its Spec Branch and not yet landed.
func (l *lander) landSpecs(ctx context.Context, p core.ProjectView) {
	if !p.LandingPolicies().LandPerSpecInForce() {
		return
	}
	waiting := map[string]bool{}
	var specs []string
	for _, tk := range l.landable(ctx, domain.Accepted) {
		if tk.SpecLanded && !waiting[tk.Spec.ID] {
			waiting[tk.Spec.ID] = true
			specs = append(specs, tk.Spec.ID)
		}
	}
	for _, id := range specs {
		if l.hooks.stopped(ctx) {
			return
		}
		l.landSpec(ctx, id, p)
	}
}

// planSpecPush works out how spec's Spec Branch is pushed in r,
// under Pull Request, for tickets, those of its waiting Tickets that
// changed r: push is false when one of their pull requests there has
// merged, as landing sees to it, or the Spec Branch is missing or in the
// base already. ok is false when the Spec can't land this poll: r's base
// is broken or not level with the remote's, or the merge can't be tried.
// Where Rebase is in force, a Spec Branch about to be pushed, its tip not
// on the remote yet, that doesn't descend from the fetched base's tip is
// rebased onto it first (rebaseSpecBranch), and a conflict there is the
// plan's; one pushed already isn't rebased again as the base moves, so
// its CI doesn't rerun on each move.
func (l *lander) planSpecPush(spec core.SpecRef, r *repository, tickets []core.TicketView, policies core.LandingPolicies) (pl specPlan, push, ok bool) {
	if slices.ContainsFunc(tickets, func(tk core.TicketView) bool { return pullRequestMergedIn(tk, r) }) {
		return pl, false, true
	}
	if l.held(r, policies) != "" || !l.stateOf(r).current {
		return pl, false, false
	}
	id, branch := spec.ID, spec.Branch
	tip, err := r.git.commit("refs/heads/" + branch)
	if err != nil || l.repos.inBaseOf(r, tip) {
		return pl, false, true
	}
	base := l.repos.baseOf(r)
	baseTip, err := r.git.commit("refs/heads/" + base)
	if err != nil {
		return pl, false, false
	}
	pl = specPlan{r: r, tickets: tickets, tip: tip, baseTip: baseTip, push: true, rebase: r.policy(policies).RebaseInForce()}
	if pl.rebase && !r.git.isAncestor(tip, "refs/remotes/"+r.remote+"/"+branch) && !r.git.isAncestor(baseTip, tip) {
		rebased, conflicts, why := l.rebaseSpecBranch(r, baseTip, tip)
		switch {
		case len(conflicts) > 0:
			pl.conflicts = conflicts
			return pl, true, true
		case why != "":
			l.printf("warning: %s: can't rebase %s onto %s%s: %s", id, branch, base, r.where(), why)
			return pl, false, false
		}
		pl.rebasedFrom, pl.tip = tip, rebased
		return pl, true, true
	}
	if _, pl.conflicts, err = r.git.mergeTree(baseTip, tip); err != nil {
		l.printf("warning: %s: can't merge %s%s: %v", id, branch, r.where(), err)
		return pl, false, false
	}
	return pl, true, true
}

// pushSpecBranch pushes spec's Spec Branch as pl says, to its
// Repository's Push remote, and reports it as each of pl's Tickets'
// pushed commit there, for the Pull Request Watcher to open the Spec's
// one pull request on. A later merge into it is pushed again, which
// updates the pull request; under Rebase, where it was rebased onto the
// base and the remote holds its older tip, the push forces with lease,
// and the new tip is each Ticket's pushed commit, so CI results on the
// old one no longer count. It never pushes, forced or not, over a branch
// of that name the Factory didn't push (foreignSpecBranch); pushedIn is
// the Repositories where it did.
func (l *lander) pushSpecBranch(ctx context.Context, spec core.SpecRef, pushedIn []string, policies core.LandingPolicies, pl specPlan) {
	id, branch := spec.ID, spec.Branch
	why, ok := l.foreignSpecBranch(ctx, pl.r, branch, pl.tip, pushedIn)
	if !ok {
		return
	}
	var pushed bool
	if why == "" {
		pushed, why = l.push(ctx, pl.r, pl.r.remote, branch, pl.tip, pl.rebase)
	}
	if why != "" {
		for _, tk := range pl.tickets {
			l.cantPushIn(ctx, tk, pl.r, why)
		}
		return
	}
	if pushed {
		l.pushedSpecBranches[pl.r.name+"@"+branch] = true
		l.printf("%s: pushed %s to %s%s", id, branch, pl.r.remote, pl.r.where())
	}
	for _, tk := range pl.tickets {
		l.reportPushed(ctx, tk, policies, pl.r, branch, l.remoteTip(pl.r, pl.r.remote, branch, pl.tip))
	}
}

// foreignSpecBranch is why the Factory won't push the Spec Branch branch,
// at tip, to r's Push remote: the remote has a branch of that name the
// Factory didn't push, as none of the Spec's Tickets has a pushed commit
// reported on it in r (pushedIn), this run didn't push it there, and it
// isn't at tip, so a push would replace, or under Rebase force over,
// someone else's work. The human
// deletes or renames it there, as the Spec's branch Tag is locked by
// now. A remote-tracking ref the remote's branch was deleted from since
// is deleted too, and the push goes ahead. why is "" when the push can
// go ahead; ok is false when the remote can't be asked, retried next
// poll.
func (l *lander) foreignSpecBranch(ctx context.Context, r *repository, branch, tip string, pushedIn []string) (why string, ok bool) {
	tracking := "refs/remotes/" + r.remote + "/" + branch
	theirs, err := r.git.commit(tracking)
	if err != nil || theirs == tip || slices.Contains(pushedIn, r.name) || l.pushedSpecBranches[r.name+"@"+branch] {
		return "", true
	}
	has, err := r.git.remoteHas(ctx, r.remote, branch)
	if err != nil {
		if ctx.Err() == nil {
			l.printf("warning: can't ask %s for %s%s, so it isn't pushed this poll: %v", r.remote, branch, r.where(), err)
		}
		return "", false
	}
	if !has {
		if err := r.git.deleteRef(tracking, theirs); err != nil {
			l.printf("warning: can't forget %s, deleted from %s%s, so %s isn't pushed this poll: %v", tracking, r.remote, r.where(), branch, err)
			return "", false
		}
		return "", true
	}
	return fmt.Sprintf("%s already has a branch named %s that the Factory didn't push, so it won't push the Spec Branch over it. Delete or rename it on %s, and the Factory pushes the Spec Branch on a later poll.", r.remote, branch, r.remote), true
}

// pullRequestMergedIn reports whether tk's pull request in r has merged.
func pullRequestMergedIn(tk core.TicketView, r *repository) bool {
	v, _ := viewIn(tk, r)
	return hasMerged(v.PullRequest)
}

// readySpec is the Spec id with its waiting Tickets, when Code Central
// says it is ready to land; ok is false when it isn't, or it or one of
// its Tickets can't be read.
func (l *lander) readySpec(ctx context.Context, id string) (spec core.SpecDetail, tickets []core.TicketView, ok bool) {
	spec, err := l.client.GetSpec(ctx, l.project, core.GetSpecInput{Spec: core.Ref(id), Parts: []string{"tickets"}})
	if err != nil {
		if ctx.Err() == nil {
			l.printf("%s: get_spec: %v", id, err)
		}
		return spec, nil, false
	}
	if spec.LandPerSpec == nil || !spec.LandPerSpec.Ready {
		return spec, nil, false
	}
	for _, w := range spec.LandPerSpec.Waiting {
		tk, err := l.client.GetTicket(ctx, l.project, core.Ref(w))
		if err != nil {
			if ctx.Err() == nil {
				l.printf("%s: get_ticket: %v", w, err)
			}
			return spec, nil, false
		}
		tickets = append(tickets, tk)
	}
	return spec, tickets, true
}

// landSpec lands one Spec when Code Central says it is ready.
func (l *lander) landSpec(ctx context.Context, id string, p core.ProjectView) {
	spec, tickets, ok := l.readySpec(ctx, id)
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
	l.mergeSpec(ctx, spec, tickets, p, domain.LandingPolicy.MergesSpecBranches, true)
}

// anyUncloned reports whether one of tickets has a Changed Repository it
// hasn't landed in with no clone, whose Spec Branch can't be checked: its
// Spec waits, as the Ticket's own line says.
func (l *lander) anyUncloned(tickets []core.TicketView) bool {
	return slices.ContainsFunc(tickets, func(tk core.TicketView) bool {
		_, why := l.changedWorkOf(tk)
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
// changed r and haven't landed there. With push, under Pull Request, the
// Spec Branch at tip is pushed for the Spec's pull request, unless it
// conflicts with the base at baseTip. Otherwise they land at old, a merge
// it made into a base the Project no longer names; at once when the base
// holds the Spec Branch's tip already; or by merging the Spec Branch at
// tip into the base at baseTip, making tree, unless that conflicts. Under
// Rebase, tip may be the Spec Branch rebased onto baseTip, from
// rebasedFrom, where the Spec Branch is until it is moved to tip; rebase
// says Rebase is in force in r.
type specPlan struct {
	r                               *repository
	tickets                         []core.TicketView
	old                             *mergeOn
	tip, baseTip, tree, rebasedFrom string
	inBase, push, rebase            bool
	conflicts                       []string
}

// mergeSpec lands spec's waiting Tickets, tickets, in each Repository
// they changed: where its Landing Policy holds for mergesThere, it merges
// the Spec Branch into the base and lands them there as a group; with
// pushes, where it is under Pull Request, it pushes the Spec Branch for
// the Spec's pull request (planSpecPush). It works out every Repository's
// merge first: a conflict in any merges and pushes it nowhere and opens
// the one Spec merge Ticket, naming each (ADR 0049), though a merge made
// on an earlier poll still goes on to its push and land. Where Rebase is
// in force and the Spec Branch doesn't descend from the base's tip, the
// merge worked out is of the Spec Branch rebased onto it, and with no
// conflict anywhere each Spec Branch is moved to its rebase first
// (moveRebasedSpecBranches). Then, in landing order, it makes the same
// "Merge spec" commit in each, advances the base, pushes and lands, or
// pushes the Spec Branch, and stops at a Repository it can't land in yet,
// to go on from there next poll. With several Repositories, one with a
// broken base or no clone holds the whole Spec back, as it can't be
// checked. It reports whether the Tickets are handled this poll: false
// means every Spec Branch and the Tickets' work in it is already in its
// base (landsHere), or one can't be read, and the Tickets land through
// the plain path, as their work is in the base.
func (l *lander) mergeSpec(ctx context.Context, spec core.SpecDetail, tickets []core.TicketView, p core.ProjectView, mergesThere func(domain.LandingPolicy) bool, pushes bool) (handled bool) {
	sort.SliceStable(tickets, func(i, j int) bool { return acceptedAt(tickets[i]).Before(acceptedAt(tickets[j])) })
	policies, ref := p.LandingPolicies(), specRefOf(spec)
	if l.anyUncloned(tickets) {
		return true
	}
	var plans, conflicts []specPlan
	for _, r := range l.repos.ready() {
		own := r.policy(policies)
		push := pushes && own.PushesSpecBranches()
		in := ticketsIn(tickets, r)
		if len(in) == 0 || !push && !mergesThere(own) {
			continue
		}
		var pl specPlan
		var readable, ok bool
		switch {
		case push:
			if pl, push, ok = l.planSpecPush(ref, r, in, policies); !ok {
				return true
			}
			if !push {
				continue
			}
		case l.repos.several() && l.held(r, policies) != "":
			return true // its Tickets say why they wait
		default:
			if pl, readable, ok = l.planSpec(ref, r, in, own.RebaseInForce()); !ok {
				return readable
			}
		}
		if len(pl.conflicts) > 0 {
			conflicts = append(conflicts, pl)
		}
		plans = append(plans, pl)
	}
	if !slices.ContainsFunc(plans, l.landsHere) {
		// Merged on an earlier poll whose landing then failed: those Tickets
		// land through the plain path, as their work is in every base.
		return false
	}
	if len(conflicts) > 0 {
		l.openSpecConflict(ctx, spec, conflicts)
	} else if !l.moveRebasedSpecBranches(ref, plans) {
		return true
	}
	for _, pl := range plans {
		switch {
		case len(conflicts) > 0 && (pl.push || pl.old == nil && !pl.inBase):
			// Nothing is merged or pushed while a Spec Branch conflicts, but
			// a merge made already goes on to its push and land.
		case pl.push:
			l.pushSpecBranch(ctx, ref, specBranchPushedIn(spec), policies, pl)
		case !l.landSpecIn(ctx, spec, p, pl):
			return true
		}
	}
	return true
}

// planSpec works out how spec lands tickets in r. With rebase,
// Rebase being in force there, a Spec Branch that doesn't descend from
// the base's tip is rebased onto it first (rebaseSpecBranch), and a
// conflict there is the plan's. ok is false when it can't, and then
// handled is mergeSpec's answer: false when the Spec Branch can't be
// read, true with a line saying why otherwise.
func (l *lander) planSpec(spec core.SpecRef, r *repository, tickets []core.TicketView, rebase bool) (pl specPlan, handled, ok bool) {
	id, branch := spec.ID, spec.Branch
	pl = specPlan{r: r, tickets: tickets, rebase: rebase}
	tip, err := r.git.commit("refs/heads/" + branch)
	if err != nil {
		return pl, false, false
	}
	pl.tip = tip
	if old, ok := l.mergedOnOldBase(r, tickets[0].ID); ok {
		// Merged before the base changed, so finishing on its push or CI.
		pl.old = &old
		return pl, true, true
	}
	base := l.repos.baseOf(r)
	if pl.baseTip, err = r.git.commit("refs/heads/" + base); err != nil {
		l.printf("warning: %s: the base, %s, isn't a branch%s, so the Spec can't land", id, base, r.where())
		return pl, true, false
	}
	if l.repos.inBaseOf(r, tip) {
		pl.inBase = true
		return pl, true, true
	}
	if rebase && !r.git.isAncestor(pl.baseTip, tip) {
		rebased, conflicts, why := l.rebaseSpecBranch(r, pl.baseTip, tip)
		switch {
		case len(conflicts) > 0:
			pl.conflicts = conflicts
			return pl, true, true
		case why != "":
			l.printf("warning: %s: can't rebase %s onto %s%s: %s", id, branch, base, r.where(), why)
			return pl, true, false
		}
		pl.rebasedFrom, pl.tip = tip, rebased
	}
	if pl.tree, pl.conflicts, err = r.git.mergeTree(pl.baseTip, pl.tip); err != nil {
		l.printf("warning: %s: can't merge %s%s: %v", id, branch, r.where(), err)
		return pl, true, false
	}
	return pl, true, true
}

// landsHere reports whether mergeSpec lands pl's Tickets, rather than the
// plain path: pl pushes the Spec Branch, or the base doesn't hold it, or
// it does but not a Ticket's work there, as when the work was replayed
// onto the Spec Branch under Rebase and the Spec Branch since rebased
// onto the base.
func (l *lander) landsHere(pl specPlan) bool {
	if pl.push || !pl.inBase {
		return true
	}
	return slices.ContainsFunc(pl.tickets, func(tk core.TicketView) bool {
		head, _, err := l.workIn(pl.r, tk)
		return err == nil && !l.repos.inBaseOf(pl.r, head)
	})
}

// moveRebasedSpecBranches moves spec's Spec Branch in each of plans'
// Repositories where it was rebased onto the base to its rebased tip,
// from where it was, and reports whether every one moved. One that moved
// meanwhile, or that git refuses, holds the Spec to the next poll, which
// works it out again.
func (l *lander) moveRebasedSpecBranches(spec core.SpecRef, plans []specPlan) bool {
	id, branch := spec.ID, spec.Branch
	for _, pl := range plans {
		if pl.rebasedFrom == "" {
			continue
		}
		if err := pl.r.git.updateRef("refs/heads/"+branch, pl.tip, pl.rebasedFrom); err != nil {
			l.printf("warning: %s: can't move %s to its rebase onto %s%s: %v", id, branch, l.repos.baseOf(pl.r), pl.r.where(), err)
			return false
		}
		l.printf("%s: rebased %s onto %s at %s%s", id, branch, l.repos.baseOf(pl.r), short(pl.tip), pl.r.where())
	}
	return true
}

// landSpecIn lands spec's Tickets in pl's Repository as pl says, making
// the Spec's merge and advancing the base first when it needs one. It
// reports whether landing can go on to the next Repository.
func (l *lander) landSpecIn(ctx context.Context, spec core.SpecDetail, p core.ProjectView, pl specPlan) bool {
	r, base := pl.r, l.repos.baseOf(pl.r)
	switch {
	case pl.old != nil:
		// It lands on the base it was merged into.
		return l.landEachAt(ctx, pl.tickets, p, r, pl.old.base, pl.old.commit)
	case pl.inBase:
		// Merged here on an earlier poll whose landing then failed.
		commit := pl.tip
		if m, ok := l.stateOf(r).mergedOn[pl.tickets[0].ID]; ok && m.base == base {
			commit = m.commit
		}
		return l.landEachAt(ctx, pl.tickets, p, r, base, commit)
	}
	merge, err := r.git.commitTree(pl.tree, fmt.Sprintf("Merge spec %s: %s", spec.ID, spec.Title), pl.baseTip, pl.tip)
	if err != nil {
		l.printf("warning: %s: can't make the merge commit%s: %v", spec.ID, r.where(), err)
		return false
	}
	if why := l.advanceBase(r, pl.baseTip, merge); why != "" {
		for _, tk := range pl.tickets {
			l.cantLandIn(ctx, tk, r, why)
		}
		return false
	}
	l.printf("%s: merged %s into %s at %s%s", spec.ID, core.SpecBranch(spec.ID, spec.Tags), base, short(merge), r.where())
	l.printf("%s: landing its Tickets%s: %s", spec.ID, r.where(), joinIDs(pl.tickets))
	for _, tk := range pl.tickets {
		l.stateOf(r).mergedOn[tk.ID] = mergeOn{base, merge}
	}
	return l.landEachAt(ctx, pl.tickets, p, r, base, merge)
}

// landEachAt lands each of tickets in r at commit, which is in r's base,
// and reports whether landing can go on to the next Repository: every
// one's push and land call went through.
func (l *lander) landEachAt(ctx context.Context, tickets []core.TicketView, p core.ProjectView, r *repository, base, commit string) bool {
	ok := true
	for _, tk := range tickets {
		ok = l.landPushed(ctx, tk, p, r, base, commit) && ok
	}
	return ok
}

// landSpecsOff is the poll's landing, in each Repository under Merge,
// with Land per Spec turned off part-way, or a Spec gaining the
// long-lived Label: every Spec Branch there holding Accepted Tickets
// merged and not landed, which Land per Spec no longer covers, goes into
// the base, ready or not, and those Tickets land there as a group. It
// returns the Tickets it handled, which the plain path leaves alone this
// poll.
func (l *lander) landSpecsOff(ctx context.Context, accepted []core.TicketView, p core.ProjectView) map[string]bool {
	handled := map[string]bool{}
	if !p.LandingPolicies().Any(domain.LandingPolicy.MergesLeftoverSpecBranches) {
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
		if l.workInEveryBase(tk) {
			continue
		}
		if len(groups[tk.Spec.ID]) == 0 {
			specs = append(specs, tk.Spec.ID)
		}
		groups[tk.Spec.ID] = append(groups[tk.Spec.ID], tk)
	}
	for _, id := range specs {
		if l.hooks.stopped(ctx) {
			break
		}
		spec, err := l.client.GetSpec(ctx, l.project, core.GetSpecInput{Spec: core.Ref(id), Parts: []string{"tickets"}})
		if err != nil {
			if ctx.Err() == nil {
				l.printf("%s: get_spec: %v", id, err)
			}
			for _, tk := range groups[id] {
				handled[tk.ID] = true
			}
			continue
		}
		if l.mergeSpec(ctx, spec, groups[id], p, domain.LandingPolicy.MergesLeftoverSpecBranches, false) {
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
func (l *lander) openSpecConflict(ctx context.Context, spec core.SpecDetail, conflicts []specPlan) {
	several := l.repos.several()
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
	res, err := l.client.OpenSpecMerge(ctx, l.project, in)
	if err != nil {
		if ctx.Err() == nil {
			l.printf("%s: open_spec_merge: %v", spec.ID, err)
		}
		return
	}
	// Where the Spec Branch conflicts: in each named Repository, or with
	// the Home Repository alone, with the base in these files.
	branch, with, again := core.SpecBranch(spec.ID, spec.Tags), l.repos.baseBranch()+" in "+strings.Join(in.Conflicts, ", "), l.repos.baseBranch()+" again"
	if several {
		with = "the base in " + strings.Join(names, ", ")
		again = with + " again"
	}
	do := "merge the base into it"
	if slices.ContainsFunc(conflicts, func(pl specPlan) bool { return pl.rebase }) {
		do = "rebase it onto the base"
	}
	switch res.Outcome {
	case core.SpecMergeOpened:
		l.printf("%s: %s conflicts with %s: opened %s to %s", spec.ID, branch, with, res.Ticket, do)
	case core.SpecMergeCommented:
		l.printf("%s: %s conflicts with %s: commented on %s", spec.ID, branch, again, res.Ticket)
	}
}

// specLanded reports whether a Ticket of spec has landed, which under
// Pull Request means its pull request merged: the Spec Branch then holds
// nothing the base lacks, even after a squash merge.
func (l *lander) specLanded(spec core.SpecDetail) bool {
	for _, r := range spec.Tickets {
		if r.State == domain.Done {
			return true
		}
	}
	return false
}

// sweepSpecBranches deletes, in every Repository with a clone, each Spec
// Branch that is in that Repository's base once every Ticket on its Spec
// is Done or Cancelled. One not in the base, as when the whole Spec is
// cancelled, is kept, with a line printed once. The branch a Spec's branch
// Tag names is swept the same way.
func (l *lander) sweepSpecBranches(ctx context.Context, policies core.LandingPolicies) {
	specs := map[string]*core.SpecDetail{}
	for _, r := range l.repos.ready() {
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
				spec = l.finishedSpec(ctx, id)
				specs[id] = spec
			}
			if spec != nil {
				l.sweepSpecBranch(r, id, branch, *spec, policies)
			}
		}
	}
	l.sweepTaggedSpecBranches(ctx, policies)
}

// sweepTaggedSpecBranches is sweepSpecBranches' work for the Specs with a
// branch Tag, finished ones included, in each Repository whose clone has
// the branch the Tag names.
func (l *lander) sweepTaggedSpecBranches(ctx context.Context, policies core.LandingPolicies) {
	rows, err := l.client.FindSpecs(ctx, l.project, core.FindSpecsInput{Tags: map[string]string{core.BranchTag: ""}, Finished: true})
	if err != nil {
		return
	}
	for _, row := range rows {
		branch := row.Tags[core.BranchTag]
		if branch == "" {
			continue
		}
		var spec *core.SpecDetail
		for _, r := range l.repos.ready() {
			if ctx.Err() != nil {
				return
			}
			if _, err := r.git.commit("refs/heads/" + branch); err != nil {
				continue
			}
			if spec == nil {
				if spec = l.finishedSpec(ctx, row.ID); spec == nil {
					break
				}
			}
			l.sweepSpecBranch(r, row.ID, branch, *spec, policies)
		}
	}
}

// finishedSpec is the Spec id when every Ticket on it is Done or
// Cancelled, and nil when one isn't, it has none, or it can't be read.
func (l *lander) finishedSpec(ctx context.Context, id string) *core.SpecDetail {
	spec, err := l.client.GetSpec(ctx, l.project, core.GetSpecInput{Spec: core.Ref(id), Parts: []string{"tickets"}})
	if err != nil || len(spec.Tickets) == 0 {
		return nil
	}
	for _, r := range spec.Tickets {
		if r.State != domain.Done && r.State != domain.Cancelled {
			return nil
		}
	}
	return &spec
}

// sweepSpecBranch deletes branch, the Spec Branch of the finished spec in
// r, when it is in r's base, and otherwise says once that it is kept.
func (l *lander) sweepSpecBranch(r *repository, id, branch string, spec core.SpecDetail, policies core.LandingPolicies) {
	policy := r.policy(policies)
	base := l.repos.baseOf(r)
	ref := "refs/heads/" + branch
	tip, err := r.git.commit(ref)
	if err != nil {
		return
	}
	where := r.where()
	if !l.repos.inBaseOf(r, tip) && !(policy.LandsByPullRequest() && l.specLanded(spec)) {
		if warned := ref + where; !l.warned[warned] {
			l.warned[warned] = true
			l.printf("%s: every Ticket is finished but %s%s isn't in %s: kept", id, branch, where, base)
		}
		return
	}
	if err := r.git.deleteRef(ref, tip); err != nil {
		l.printf("warning: %s: can't delete %s%s: %v", id, branch, where, err)
		return
	}
	l.printf("%s: deleted %s%s, which is in %s", id, branch, where, base)
}
