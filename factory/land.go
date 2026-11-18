package factory

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// landAndSweep is a poll's landing: it reads the Landing Method and the
// Repositories, reports the Home's Push remote when it needs to, makes
// sure each Repository has a clone, lands what it can unless the Factory
// is paused or halted, and sweeps the worktrees the Factory is done with.
func (f *factory) landAndSweep(ctx context.Context) {
	p, ok := f.landing(ctx)
	if ok {
		p = f.reportPushRemote(ctx, p)
		f.syncRepositories(ctx, p)
		f.locateBases(ctx, p)
	}
	if ok && !f.paused() && !f.halted(ctx) {
		f.land(ctx, p)
	}
	policies := p.LandingPolicies()
	f.sweep(ctx, policies)
	f.sweepSpecBranches(ctx, policies)
}

// landing reads the Project, whose Landing Method, with its Push option, the
// human can change at any time, so it is read again on every poll, with
// whether CI Tracking is on. Its Landing Policy is what those make. ok is
// false when it can't be read, and then nothing lands this poll. A poll that
// reads it also makes the report of what claude accepts, until one is taken.
func (f *factory) landing(ctx context.Context) (p core.ProjectView, ok bool) {
	p, err := f.client.GetProject(ctx, f.project)
	if err != nil {
		if ctx.Err() == nil {
			f.printf("warning: can't read the Project's Landing Method, so nothing lands this poll: %v", err)
		}
		return core.ProjectView{}, false
	}
	f.setBase(p.Landing.Base)
	if !f.modelsReported {
		f.modelsReported = f.reportModels(ctx)
	}
	return p, true
}

// reportPushRemote reports the URL of the Home's own Push remote in p, or
// "" when its clone has none, making the Home local-only (Spec
// code-central-890): at start, when the name changes, and when the remote
// has gone while p has the Home pushed; one added later waits for the next
// start. It returns p as the report left it.
func (f *factory) reportPushRemote(ctx context.Context, p core.ProjectView) core.ProjectView {
	i := slices.IndexFunc(p.Repositories, func(v core.RepositoryView) bool { return v.Home })
	if i < 0 {
		return p
	}
	home := p.Repositories[i]
	url, err := f.repos.home.git.remoteURL(home.PushRemote)
	if err != nil {
		url = ""
	}
	if f.remoteReported && home.PushRemote == f.reportedRemote && (url != "" || home.LocalOnly) {
		return p
	}
	got, err := f.client.ReportPushRemote(ctx, f.project, core.ReportPushRemoteInput{URL: url, Full: true})
	if err != nil {
		if ctx.Err() == nil && !f.reportRemoteFailing {
			f.printf("warning: can't report the Push remote's URL to Code Central: %v", err)
		}
		f.reportRemoteFailing = true
		return p
	}
	f.reportRemoteFailing = false
	f.reportedRemote, f.remoteReported = home.PushRemote, true
	if got.Record == nil {
		return p
	}
	return *got.Record
}

// land lands Accepted Tickets one at a time, oldest accept first (ADR
// 0006), each in each of its Changed Repositories by the route that
// Repository's Landing Policy gives it (landEach), lands ready Specs in
// each Repository by its Landing, and detects merges made by hand. The
// Factory never accepts: only the human moves a Ticket to Accepted.
//
// Each Repository's base is synced apart, under its own Landing Policy,
// so a local-only Repository's never is, and a broken one (missing, or
// under its Merge + Push not level with the remote's) holds back only
// the Tickets landing in it; with the Home Repository alone, it holds
// back everything, as it always has (Spec code-central-848).
func (f *factory) land(ctx context.Context, p core.ProjectView) {
	if f.stateOf(f.repos.home).missing && !f.repos.several() {
		return
	}
	accepted := f.landable(ctx, store.Accepted)
	sort.SliceStable(accepted, func(i, j int) bool { return acceptedAt(accepted[i]).Before(acceptedAt(accepted[j])) })
	var held map[string]bool
	if !f.stateOf(f.repos.home).missing {
		held = f.landSpecsOff(ctx, accepted, p)
	}
	policies := p.LandingPolicies()
	// Each base its Landing Policy keeps level is brought up to the
	// remote's before anything lands: never a local-only one's, which a
	// Home that lost its remote under Pull Request is.
	for _, r := range f.repos.ready() {
		s, own := f.stateOf(r), r.policy(policies)
		if !s.missing && !r.localOnly && own.KeepsBaseLevel() {
			s.current = f.syncBase(ctx, r, r.remote, own.PushesBase(), accepted)
		}
	}
	// Under Merge + Push a base that isn't level lands nothing touching
	// it: not a Ticket, nor a merge into a Spec Branch cut from it.
	homeReady := f.held(f.repos.home, policies) == ""
	if !homeReady && !f.repos.several() {
		return
	}
	if policies.Any(core.LandingPolicy.CIGateHolds) {
		f.reportTip(ctx, policies)
	}
	for _, tk := range accepted {
		if f.halted(ctx) || f.paused() {
			return
		}
		if held[tk.ID] {
			continue
		}
		// The finish check again before the Ticket's first landing step,
		// which a restart or an Auto-accept may have come to before the
		// check after its turn.
		if tk.SpecMerge == nil && !tk.PartlyLanded {
			if passed, _ := f.checkFinish(ctx, tk); !passed {
				continue
			}
		}
		ws, why := f.workToLand(tk, policies)
		if why != "" {
			if key := tk.ID + "@" + why; !f.saidHeld[key] {
				f.saidHeld[key] = true
				f.printf("%s: waits to land: %s", tk.ID, why)
			}
			continue
		}
		f.landEach(ctx, tk, p, ws)
	}
	// Ready Specs land in each Repository by its Landing, and hand merges
	// are detected, each held back only by the broken bases of the
	// Repositories it touches.
	f.landSpecs(ctx, p)
	f.detectHandMerges(ctx, p)
}

// landEach lands tk in each of ws, its Changed Repositories, by the route
// that Repository's own Landing Policy gives it (Spec code-central-890):
// by Merge the Factory merges it into the base there and, with Push on,
// pushes the base before landing it; by Spec Branch it merges it into its
// Spec Branch there; by Pull Request it pushes its branch there for its
// pull request, and lands it there once that is merged; by Branch it
// leaves it for the human to merge there, and lands it there once
// merged. Every merge the Factory makes is worked out before tk lands in
// any Repository: one that conflicts, or can't be worked out this poll,
// moves nothing of tk anywhere (ADR 0049). Then it goes through them in
// the landing order, and stops at a merge it can't land yet, to go on
// from there next poll. tk is Partly landed until it has landed in each.
func (f *factory) landEach(ctx context.Context, tk core.TicketView, p core.ProjectView, ws []changedWork) {
	policies := p.LandingPolicies()
	onSpec := f.waitsOnSpecBranch(tk)
	routes := make([]core.LandingRoute, len(ws))
	var merges, specs []changedWork
	for i, w := range ws {
		routes[i] = w.r.policy(policies).Route(onSpec, pullRequestMergedIn(tk, w.r))
		switch routes[i] {
		case core.RouteMerge:
			merges = append(merges, w)
		case core.RouteSpecBranch:
			specs = append(specs, w)
		}
	}
	plans, ok := f.planMerges(ctx, tk, merges)
	if !ok {
		return
	}
	if len(specs) > 0 && !f.mergeIntoSpecBranch(ctx, tk, specs) {
		return
	}
	for i, w := range ws {
		switch routes[i] {
		case core.RouteMerge:
			if pl, ok := plans[w.r]; ok && !f.mergeAndLand(ctx, tk, p, pl) {
				return
			}
		case core.RoutePullRequest:
			if changedElsewhere(f.repos.home.name, tk) {
				f.landPullRequestIn(ctx, tk, policies, w.r, onSpec)
			} else {
				// Its work is in the Home Repository alone, which lands
				// by the Ticket's own pull request.
				f.landPullRequest(ctx, tk, p)
			}
		case core.RouteBranch:
			f.landMergedBranch(ctx, tk, policies, w)
		}
	}
}

// landMergedBranch lands an Accepted Ticket in w, one of its Changed
// Repositories under the Branch Landing Method, once its work is in that
// Repository's base: the human merged it there. It is Done once landed in
// every one (ADR 0049). Until then the Repository is left as it is, with
// a line saying so once: never merged, never sent back. With Push on
// there, as it is in every Repository but a local-only one, its branch
// there is pushed to the remote meanwhile.
func (f *factory) landMergedBranch(ctx context.Context, tk core.TicketView, policies core.LandingPolicies, w changedWork) {
	r := w.r
	head, what, err := f.workIn(r, tk)
	switch {
	case err == nil && w.landed != "" && r.git.isAncestor(head, w.landed):
		return
	case err == nil && f.repos.inBaseOf(r, head):
		f.printf("%s: merged into %s by hand%s", tk.ID, f.repos.baseOf(r), r.where())
		f.landAt(ctx, tk, r, head)
		return
	case err == nil && r.policy(policies).PushesAcceptedBranch():
		f.pushBranch(ctx, tk, policies, r, head)
	}
	key := tk.ID + "@" + head + r.where()
	if f.leftOnBranch[key] {
		return
	}
	f.leftOnBranch[key] = true
	if err != nil {
		f.printf("warning: %s is Accepted under the Branch Landing Method, but %s isn't in this repository", tk.ID, what)
		return
	}
	merge := branchIn(tk, r)
	if merge == "" {
		merge = short(head)
	}
	f.printf("%s: Accepted under the Branch Landing Method: merge %s into %s%s and the Factory lands it", tk.ID, merge, f.repos.baseOf(r), r.where())
}

// branchIn is tk's branch in r: the one its finish named there.
func branchIn(tk core.TicketView, r *repository) string {
	v, _ := viewIn(tk, r)
	return v.Branch
}

// pushBranch makes sure tk's branch in r, holding commit, is on r's Push
// remote, and comments when it can't be pushed yet, to be retried next
// poll.
func (f *factory) pushBranch(ctx context.Context, tk core.TicketView, policies core.LandingPolicies, r *repository, commit string) {
	branch := branchIn(tk, r)
	if branch == "" {
		f.cantPushYet(ctx, tk, "it was finished without a branch, so there is no branch to push.")
		return
	}
	pushed, why := f.push(ctx, r, r.remote, branch, commit)
	if why != "" {
		f.cantPushIn(ctx, tk, r, why)
		return
	}
	if pushed {
		f.printf("%s: pushed %s to %s%s", tk.ID, branch, r.remote, r.where())
		// Only a push made now: one made before CI Tracking was turned on
		// isn't tracked.
		f.reportPushed(ctx, tk, policies, r, branch, f.remoteTip(r, r.remote, branch, commit))
	}
}

// workOf is the commit of the work the human reviewed: the finish
// commit, or with none, the branch's tip. what names it, and err says
// it isn't in the repository.
func (f *factory) workOf(tk core.TicketView) (commit, what string, err error) {
	work, what := tk.Commit, "its commit "+tk.Commit
	if work == "" {
		work, what = "refs/heads/"+tk.Branch, "its branch "+tk.Branch
	}
	commit, err = f.repos.home.git.commit(work)
	return commit, what, err
}

// workIn is workOf in r: the commit of tk's work there, its finish commit
// for r, or with none, its branch's tip there.
func (f *factory) workIn(r *repository, tk core.TicketView) (commit, what string, err error) {
	if r.home {
		return f.workOf(tk)
	}
	v, ok := viewIn(tk, r)
	if !ok {
		return "", "its work in Repository " + r.name, fmt.Errorf("%s holds no work in Repository %s", tk.ID, r.name)
	}
	work, what := v.Commit, "its commit "+v.Commit+" in Repository "+r.name
	if work == "" {
		work, what = "refs/heads/"+v.Branch, "its branch "+v.Branch+" in Repository "+r.name
	}
	commit, err = r.git.commit(work)
	return commit, what, err
}

// detectHandMerges lands In Review Tickets whose finish commit is in the
// base: the human merged them. Under Merge + Push it lands each only once
// the base is pushed. (land sees to Accepted ones.) A Ticket merged while in any other State
// gets a warning line, once. With several Repositories it lands each
// Changed Repository whose base holds the Ticket's finish commit there,
// and the Ticket is Done once it has landed in all of them. In a
// Repository under Pull Request it is a merged pull request that lands
// the Ticket there, not its finish commit in the base
// (detectMergedPullRequests).
func (f *factory) detectHandMerges(ctx context.Context, p core.ProjectView) {
	policies := p.LandingPolicies()
	for _, tk := range f.landable(ctx, mergeableStates...) {
		if f.halted(ctx) || f.paused() {
			return
		}
		f.detectMergedPullRequests(ctx, tk, policies)
		merged := f.mergedByHand(tk, policies)
		if len(merged) == 0 {
			continue
		}
		if tk.State == store.InReview {
			if passed, _ := f.checkFinish(ctx, tk); !passed {
				continue
			}
			for _, m := range merged {
				f.printf("%s: merged into %s by hand%s", tk.ID, f.repos.baseOf(m.r), m.r.where())
				if !f.landPushed(ctx, tk, p, m.r, f.repos.baseOf(m.r), m.commit) {
					break
				}
			}
			continue
		}
		m := merged[0]
		key := tk.ID + "@" + m.commit
		if !f.warnedMerged[key] {
			f.warnedMerged[key] = true
			f.printf("warning: %s is %s but its finish commit %s is already in %s%s: only an In Review or Accepted Ticket lands", tk.ID, tk.State, short(m.commit), f.repos.baseOf(m.r), m.r.where())
		}
	}
}

// mergedByHand is tk's Changed Repositories, in landing order, whose base
// holds tk's finish commit there and which it hasn't landed in, but for
// one whose base is broken (held) under policies, and one under Pull
// Request, where its pull request lands it.
func (f *factory) mergedByHand(tk core.TicketView, policies core.LandingPolicies) []changedWork {
	ws, why := f.changedWorkOf(tk)
	if why != "" {
		return nil
	}
	var out []changedWork
	for _, w := range ws {
		if w.r.policy(policies).LandsByPullRequest() {
			continue
		}
		if w.commit == "" || f.held(w.r, policies) != "" || !f.repos.inBaseOf(w.r, w.commit) || w.landed != "" && w.r.git.isAncestor(w.commit, w.landed) {
			continue
		}
		out = append(out, w)
	}
	return out
}

// mergeableStates are the States a Ticket with a finish commit can be in
// short of Accepted, Done and Cancelled: every State but those and Draft,
// which comes before any work.
var mergeableStates = slices.DeleteFunc(slices.Clone(store.States), func(s store.State) bool {
	return s == store.Draft || s == store.Accepted || s == store.Done || s == store.Cancelled
})

// inBase reports whether commit is reachable from the Home Repository's
// base.
func (f *factory) inBase(commit string) bool { return f.repos.inBaseOf(f.repos.home, commit) }

// landable is every Ticket in these States, read in full by one
// find_tickets call. If the call fails it lands nothing this poll.
func (f *factory) landable(ctx context.Context, states ...store.State) []core.TicketView {
	out, err := f.client.FindTicketViews(ctx, f.project, core.FindTicketsInput{State: states})
	if err != nil {
		if ctx.Err() == nil {
			f.printf("warning: can't find %v Tickets: %v", states, err)
		}
		return nil
	}
	return out
}

// acceptedAt is when the Ticket last moved to Accepted.
func acceptedAt(tk core.TicketView) time.Time {
	for i := len(tk.Moves) - 1; i >= 0; i-- {
		if tk.Moves[i].To == store.Accepted {
			return tk.Moves[i].Time
		}
	}
	return tk.Updated
}

// changedWork is one of a Ticket's Changed Repositories as the Factory
// lands it: the Repository, the Ticket's finish commit there, and the
// commit the work last landed at there, if it has.
type changedWork struct {
	r              *repository
	commit, landed string
}

// changedWorkOf is tk's Changed Repositories in landing order, or with
// none named, as with the Home Repository alone, the Home Repository. It
// says why not when one it hasn't landed in has no clone.
func (f *factory) changedWorkOf(tk core.TicketView) ([]changedWork, string) {
	if len(tk.Repositories) == 0 {
		return []changedWork{{r: f.repos.home, commit: tk.Commit}}, ""
	}
	var out []changedWork
	for _, v := range tk.Repositories {
		r := f.repos.named(v.Name)
		switch {
		case (r == nil || r.problem != "") && v.Landed != nil:
			// Landed already: nothing to do there.
			continue
		case r == nil || r.problem != "":
			return nil, "Repository " + v.Name + " has no clone"
		}
		w := changedWork{r: r, commit: v.Commit}
		if v.Landed != nil {
			w.landed = v.Landed.Commit
		}
		out = append(out, w)
	}
	return out, ""
}

// workToLand is tk's Changed Repositories to land, or says why tk doesn't
// land this poll: one of them has no clone, or a broken base (held),
// though one it has landed in doesn't hold it back. Every one is checked
// before tk lands in any, whatever its route (ADR 0049).
func (f *factory) workToLand(tk core.TicketView, policies core.LandingPolicies) ([]changedWork, string) {
	ws, why := f.changedWorkOf(tk)
	if why != "" {
		return nil, why
	}
	for _, w := range ws {
		if why := f.held(w.r, policies); why != "" && w.landed == "" {
			return nil, why + w.r.where()
		}
	}
	return ws, ""
}

// mergePlan is how the Factory lands a Ticket's work in one Repository,
// worked out before it lands any: at old, a merge it made into a base the
// Project no longer names; at head, when the base holds it already; or by
// merging head into the base at baseTip, making tree, unless that
// conflicts.
type mergePlan struct {
	r                   *repository
	old                 *mergeOn
	baseTip, head, tree string
	inBase              bool
	conflicts           []string
}

// planMerges works out how an Accepted Ticket's work is merged into the
// base of each of ws, the Changed Repositories it lands in by Merge, that
// it hasn't landed in (ADR 0049), by Repository. ok is false when it
// lands in none this poll: a merge can't be worked out yet, or one
// conflicts, which sends the Ticket back.
func (f *factory) planMerges(ctx context.Context, tk core.TicketView, ws []changedWork) (plans map[*repository]mergePlan, ok bool) {
	plans = map[*repository]mergePlan{}
	var conflicts []landConflict
	for _, w := range ws {
		pl, landed, planned := f.planMerge(ctx, tk, w)
		switch {
		case !planned:
			return nil, false
		case landed:
		case len(pl.conflicts) > 0:
			conflicts = append(conflicts, landConflict{r: pl.r, head: pl.head, into: f.repos.baseOf(pl.r), tip: pl.baseTip, files: pl.conflicts})
		default:
			plans[w.r] = pl
		}
	}
	if len(conflicts) > 0 {
		f.sendBackConflicts(ctx, tk, "base", conflicts)
		return nil, false
	}
	return plans, true
}

// planMerge works out how tk lands in w's Repository, or that it has
// landed there already: its landed commit holds its work. ok is false
// when it can't be worked out this poll, with a comment or a line saying
// why.
func (f *factory) planMerge(ctx context.Context, tk core.TicketView, w changedWork) (pl mergePlan, landed, ok bool) {
	r := w.r
	pl.r = r
	if w.landed != "" {
		if head, _, err := f.workIn(r, tk); err == nil && r.git.isAncestor(head, w.landed) {
			return pl, true, true
		}
	}
	if old, ok := f.mergedOnOldBase(r, tk.ID); ok {
		// Merged before the base changed and not landed yet, so waiting on
		// its push or CI: it finishes on the base it was merged into.
		pl.old = &old
		return pl, false, true
	}
	base := f.repos.baseOf(r)
	baseTip, err := r.git.commit("refs/heads/" + base)
	if err != nil {
		f.cantLandIn(ctx, tk, r, fmt.Sprintf("the base, %s, isn't a branch, so the Factory can't move it.", base))
		return pl, false, false
	}
	head, what, err := f.workIn(r, tk)
	if err != nil {
		f.cantLandIn(ctx, tk, r, what+" isn't in this repository.")
		return pl, false, false
	}
	pl.baseTip, pl.head = baseTip, head
	if f.repos.inBaseOf(r, head) {
		pl.inBase = true
		return pl, false, true
	}
	if pl.tree, pl.conflicts, err = r.git.mergeTree(baseTip, head); err != nil {
		f.printf("warning: %s: can't merge%s: %v", tk.ID, r.where(), err)
		return pl, false, false
	}
	return pl, false, true
}

// mergeAndLand lands tk in pl's Repository as pl says, making the merge
// and advancing the base first when it needs one. It reports whether
// landing can go on to the next Repository: it can't when a step failed
// and is to be tried again next poll.
func (f *factory) mergeAndLand(ctx context.Context, tk core.TicketView, p core.ProjectView, pl mergePlan) bool {
	r, base := pl.r, f.repos.baseOf(pl.r)
	switch {
	case pl.old != nil:
		return f.landPushed(ctx, tk, p, r, pl.old.base, pl.old.commit)
	case pl.inBase:
		return f.landPushed(ctx, tk, p, r, base, pl.head)
	}
	msg := fmt.Sprintf("Merge %s: %s", tk.ID, tk.Title)
	merge, err := r.git.commitTree(pl.tree, msg, pl.baseTip, pl.head)
	if err != nil {
		f.printf("warning: %s: can't make the merge commit%s: %v", tk.ID, r.where(), err)
		return false
	}
	if why := f.advanceBase(r, pl.baseTip, merge); why != "" {
		f.cantLandIn(ctx, tk, r, why)
		return false
	}
	f.stateOf(r).mergedOn[tk.ID] = mergeOn{base, merge}
	return f.landPushed(ctx, tk, p, r, base, merge)
}

// landPushed lands tk in r at commit, which is in r's base, and reports
// whether landing can go on to the next Repository: false when the push
// failed, which leaves tk where it is, commented, or the land call did.
// Under r's Merge with Push on, it first makes sure the remote's base
// holds commit. With CI Tracking on as well, the CI gate holds in each
// Repository CI Tracking watches: tk lands there only once its CI result
// there is passed, at the pushed commit that passed (ADR 0019, ADR 0034),
// so another Repository's red build doesn't hold it back, and the next
// Repository goes on once the push is made, as the gate holds only the
// land. A Repository Not watched lands once pushed, and a local-only one,
// with Push off, once merged.
func (f *factory) landPushed(ctx context.Context, tk core.TicketView, p core.ProjectView, r *repository, base, commit string) bool {
	policies := p.LandingPolicies()
	own := r.policy(policies)
	if !own.PushesBase() {
		return f.landAt(ctx, tk, r, commit)
	}
	v, _ := viewIn(tk, r)
	gated := own.CIGateHolds() && !v.CINotWatched
	if gated && f.pushedBase(r, v.PushedCommit, r.remote, base, commit) {
		f.landOnCI(ctx, tk, r, v)
		return true
	}
	pushed, why := f.push(ctx, r, r.remote, base, commit)
	if why != "" {
		f.cantLandIn(ctx, tk, r, why)
		return false
	}
	if pushed {
		f.printf("%s: pushed %s to %s%s", tk.ID, base, r.remote, r.where())
	}
	if !gated {
		return f.landAt(ctx, tk, r, commit)
	}
	// CI runs on what the push left the remote's base holding. A push
	// made on an earlier poll whose report failed is reported as what it
	// pushed then, not as a later Ticket's push the remote's base holds
	// since. Its CI result is read on a later poll.
	key := tk.ID + "@" + r.name
	switch {
	case pushed:
		commit = f.remoteTip(r, r.remote, base, commit)
	case f.unreported[key] != "":
		commit = f.unreported[key]
	}
	if f.reportPushed(ctx, tk, policies, r, base, commit) {
		delete(f.unreported, key)
	} else {
		f.unreported[key] = commit
	}
	return true
}

// pushedBase reports whether pushed, a Ticket's pushed commit in r, is
// r's base on remote holding commit: pushed and reported already, so what
// the remote's base holds now, maybe a later Ticket's push, mustn't
// replace it. A pushed commit of its branch, from Branch + Push, isn't
// one.
func (f *factory) pushedBase(r *repository, pushed *core.PushedCommitView, remote, base, commit string) bool {
	if pushed == nil {
		return false
	}
	return r.git.isAncestor(commit, pushed.Commit) && r.git.isAncestor(pushed.Commit, "refs/remotes/"+remote+"/"+base)
}

// landOnCI lands tk in r, where the CI gate holds it, once v, its Changed
// Repository there, has a passed CI result, at the pushed commit that
// passed, or once the human chose Land anyway on a failed one there, at
// its latest. Until then it stays where it is, with a line for each CI
// result it waits on.
func (f *factory) landOnCI(ctx context.Context, tk core.TicketView, r *repository, v core.ChangedRepositoryView) {
	pushed := v.PushedCommit.Commit
	if v.CIResult == core.CIPassed {
		f.landAt(ctx, tk, r, v.CICommit)
		return
	}
	if (v.CIResult == core.CIFailed || v.CIResult == core.CICancelled) && v.LandAnyway {
		f.printf("%s: landing anyway at %s%s, as you chose, though its CI %s", tk.ID, short(pushed), r.where(), v.CIResult)
		f.landAt(ctx, tk, r, pushed)
		return
	}
	key := tk.ID + "@" + pushed + "@" + string(v.CIResult)
	if f.saidOnCI[key] {
		return
	}
	f.saidOnCI[key] = true
	if v.CIResult == core.CIFailed {
		f.printf("%s: CI failed at %s%s, so it stays %s", tk.ID, short(pushed), r.where(), tk.State)
		return
	}
	if v.CIResult == core.CICancelled {
		f.printf("%s: CI cancelled at %s%s, so it stays %s", tk.ID, short(pushed), r.where(), tk.State)
		return
	}
	f.printf("%s: waiting on CI at %s%s before it lands", tk.ID, short(pushed), r.where())
}

// viewIn is tk's Changed Repository r as tk's view shows it: with the
// Home Repository alone, made from the Ticket's own fields, which are the
// Home's. ok is false when tk didn't change r.
func viewIn(tk core.TicketView, r *repository) (v core.ChangedRepositoryView, ok bool) {
	if len(tk.Repositories) == 0 && r.home {
		return core.ChangedRepositoryView{Name: r.name, Branch: tk.Branch, Commit: tk.Commit, PushedCommit: tk.PushedCommit,
			CIResult: tk.CIResult, CICommit: tk.CICommit, LandAnyway: tk.LandAnyway, PullRequest: tk.PullRequest}, true
	}
	i := slices.IndexFunc(tk.Repositories, func(v core.ChangedRepositoryView) bool { return v.Name == r.name })
	if i < 0 {
		return core.ChangedRepositoryView{}, false
	}
	return tk.Repositories[i], true
}

// remoteTip is the commit branch's remote-tracking ref on remote holds
// in r, or, without one to read, pushed.
func (f *factory) remoteTip(r *repository, remote, branch, pushed string) string {
	commit, err := r.git.commit("refs/remotes/" + remote + "/" + branch)
	if err != nil {
		return pushed
	}
	return commit
}

// reportPushed reports, when r's Landing Policy, among policies, the
// Project's Repositories', records pushed commits, commit, pushed as
// branch in r, as tk's pushed commit there: the one CI runs on. It is
// reported once, when it isn't tk's pushed commit there already. ok is
// false when the report failed.
func (f *factory) reportPushed(ctx context.Context, tk core.TicketView, policies core.LandingPolicies, r *repository, branch, commit string) (ok bool) {
	own := r.policy(policies)
	if !own.RecordsPushedCommit() {
		return true
	}
	if v, _ := viewIn(tk, r); v.PushedCommit != nil && v.PushedCommit.Commit == commit {
		return true
	}
	in := core.ReportPushedCommitInput{Ticket: core.Ref(tk.ID), Commit: commit, Branch: branch}
	if !r.home {
		in.Repository = r.name
	}
	if _, err := f.client.ReportPushedCommit(ctx, f.project, in); err != nil {
		if ctx.Err() == nil {
			f.printf("%s: report_pushed_commit%s: %v", tk.ID, r.where(), err)
		}
		return false
	}
	if own.TracksCIResults() {
		f.printf("%s: CI tracks %s at %s%s", tk.ID, branch, short(commit), r.where())
	}
	return true
}

// reportTip reports each Repository's remote base tip, once that base is
// synced, as the pushed commit there of each Ticket waiting on CI there
// (Accepted, or In Review and merged there by hand, with a pushed commit
// there and not landed there) whose latest pushed commit there isn't the
// tip but is an ancestor of it: the tip contains the Ticket's work, and if
// it passes, the Ticket lands there (ADR 0034). A tip that doesn't contain
// the work, as after a force push, is reported to no Ticket, and a broken
// base's to none until it is fixed. A Repository whose own Landing Policy
// has no CI gate, a local-only one, has no tip to report. Each tip is
// read from the Repository's own Push remote.
func (f *factory) reportTip(ctx context.Context, policies core.LandingPolicies) {
	type tipIn struct {
		r   *repository
		tip string
	}
	var tips []tipIn
	for _, r := range f.repos.ready() {
		if !r.policy(policies).CIGateHolds() || f.held(r, policies) != "" {
			continue
		}
		if tip, err := r.git.commit("refs/remotes/" + r.remote + "/" + f.repos.baseOf(r)); err == nil {
			tips = append(tips, tipIn{r, tip})
		}
	}
	if len(tips) == 0 {
		return
	}
	for _, tk := range f.landable(ctx, store.Accepted, store.InReview) {
		if f.halted(ctx) || f.paused() {
			return
		}
		for _, t := range tips {
			v, ok := viewIn(tk, t.r)
			switch {
			case !ok || v.Landed != nil || v.PushedCommit == nil:
			case v.PushedCommit.Commit == t.tip || !t.r.git.isAncestor(v.PushedCommit.Commit, t.tip):
			case tk.State == store.InReview && (v.Commit == "" || !f.repos.inBaseOf(t.r, v.Commit)):
			default:
				f.reportPushed(ctx, tk, policies, t.r, f.repos.baseOf(t.r), t.tip)
			}
		}
	}
}

// push pushes branch to remote unless its remote-tracking ref already
// holds commit: a push that succeeded moves that ref, so the Factory
// needs no state of its own. It never forces, so a remote that has moved
// ahead is left as it is. It returns whether it pushed, or why the push
// failed. It pushes from r's clone, to r's own remote of that name.
func (f *factory) push(ctx context.Context, r *repository, remote, branch, commit string) (pushed bool, why string) {
	if r.git.isAncestor(commit, "refs/remotes/"+remote+"/"+branch) {
		return false, ""
	}
	defer f.work.busy("pushing ", branch)()
	// A stop kills it, so a push that hangs doesn't hold up the Factory.
	if out, err := r.git.push(ctx, remote, branch); err != nil {
		return false, fmt.Sprintf("couldn't push %s to %s:\n\n```\n%s\n```", branch, remote, out)
	}
	return true, ""
}

// cantLandYet leaves a Ticket whose clean merge can't land where it is,
// to be retried next poll. Each distinct reason is commented once.
func (f *factory) cantLandYet(ctx context.Context, tk core.TicketView, why string) {
	f.commentOnce(ctx, tk, "can't land yet", "The Factory can't land this yet", why)
}

// cantLandIn is cantLandYet for a landing step in r. With several
// Repositories the comment names r, so Accepted but not landed can give
// it as why tk hasn't landed there.
func (f *factory) cantLandIn(ctx context.Context, tk core.TicketView, r *repository, why string) {
	if !f.repos.several() {
		f.cantLandYet(ctx, tk, why)
		return
	}
	// Once since tk entered its State, as that is how far back Accepted
	// but not landed reads it.
	var since time.Time
	if n := len(tk.Moves); n > 0 {
		since = tk.Moves[n-1].Time
	}
	f.postOnce(ctx, tk, "can't land yet in "+r.name, why, core.CantLandComment(r.name, why, tk.State), since)
}

// cantPushYet leaves an Accepted Ticket whose branch couldn't be pushed
// where it is, to be retried next poll. Each distinct reason is commented
// once.
func (f *factory) cantPushYet(ctx context.Context, tk core.TicketView, why string) {
	f.commentOnce(ctx, tk, "can't push yet", "The Factory can't push this Ticket's branch yet", why)
}

// cantPushIn is cantPushYet for a push in r, which the comment names
// unless it is the Home Repository.
func (f *factory) cantPushIn(ctx context.Context, tk core.TicketView, r *repository, why string) {
	if !r.home {
		why = fmt.Sprintf("in %s, %s", r.name, why)
	}
	f.cantPushYet(ctx, tk, why)
}

// commentOnce comments on tk that the Factory can't do something yet,
// and why, unless that comment is already there, and prints a line.
func (f *factory) commentOnce(ctx context.Context, tk core.TicketView, line, lead, why string) {
	f.postOnce(ctx, tk, line, why, fmt.Sprintf("%s: %s\n\nIt stays %s and the Factory tries again on every poll.", lead, why, tk.State), time.Time{})
}

// postOnce comments body on tk unless that comment is already there since
// since, and prints a line saying why.
func (f *factory) postOnce(ctx context.Context, tk core.TicketView, line, why, body string, since time.Time) {
	for _, c := range tk.Comments {
		if c.Body == body && !c.Time.Before(since) {
			return
		}
	}
	f.printf("%s: %s: %s", tk.ID, line, strings.TrimSuffix(firstLine(why), ":"))
	if _, err := f.client.Comment(ctx, f.project, core.CommentInput{Ticket: core.Ref(tk.ID), Body: body}); err != nil && ctx.Err() == nil {
		f.printf("%s: comment: %v", tk.ID, err)
	}
}

// firstLine is s up to its first newline.
func firstLine(s string) string {
	l, _, _ := strings.Cut(s, "\n")
	return l
}

// sendBack sends a Ticket whose work conflicts with the base back to
// Ready, so a later Claim resolves the conflict and the human reviews it
// again.
func (f *factory) sendBack(ctx context.Context, tk core.TicketView, head, into, tip string, conflicts []string) {
	var b strings.Builder
	fmt.Fprintf(&b, "The Factory couldn't land this: merging %s into %s at %s conflicts in these files:\n\n", short(head), into, short(tip))
	for _, c := range conflicts {
		fmt.Fprintf(&b, "- %s\n", c)
	}
	fmt.Fprintf(&b, "\nMerge %s into the branch, resolve the conflicts, and finish again.", into)
	if _, err := f.client.SendBack(ctx, f.project, core.SendBackInput{Ticket: core.Ref(tk.ID), Note: b.String()}); err != nil {
		if ctx.Err() == nil {
			f.printf("%s: send_back: %v", tk.ID, err)
		}
		return
	}
	f.printf("%s: conflicts with %s at %s in %s: sent back to Ready", tk.ID, into, short(tip), strings.Join(conflicts, ", "))
}

// landConflict is how merging a Ticket's work in r, head, into the
// branch into at tip conflicts: in files.
type landConflict struct {
	r               *repository
	head, into, tip string
	files           []string
}

// sendBackConflicts is sendBack for a Ticket whose work conflicts with
// its onto, its "base" or its "Spec Branch", in each of conflicts'
// Repositories: one note names each, with its files (ADR 0049). The
// Factory has merged it into none of them this time. With the Home
// Repository alone it is sendBack.
func (f *factory) sendBackConflicts(ctx context.Context, tk core.TicketView, onto string, conflicts []landConflict) {
	if !f.repos.several() {
		c := conflicts[0]
		f.sendBack(ctx, tk, c.head, c.into, c.tip, c.files)
		return
	}
	var b strings.Builder
	var names []string
	fmt.Fprintf(&b, "The Factory couldn't land this: merging it into the %s conflicts in these Repositories, so it merged it into none of them:\n\n", onto)
	for _, c := range conflicts {
		names = append(names, c.r.name)
		fmt.Fprintf(&b, "- %s: merging %s into %s at %s conflicts in these files:\n", c.r.name, short(c.head), c.into, short(c.tip))
		for _, file := range c.files {
			fmt.Fprintf(&b, "  - %s\n", file)
		}
	}
	fmt.Fprintf(&b, "\nIn each, merge its %s into the branch, resolve the conflicts, and finish again.", onto)
	if _, err := f.client.SendBack(ctx, f.project, core.SendBackInput{Ticket: core.Ref(tk.ID), Note: b.String()}); err != nil {
		if ctx.Err() == nil {
			f.printf("%s: send_back: %v", tk.ID, err)
		}
		return
	}
	f.printf("%s: conflicts with the %s in %s: sent back to Ready", tk.ID, onto, strings.Join(names, ", "))
}

// advanceBase fast-forwards r's base from old to merge. It returns why it
// couldn't, or "".
func (f *factory) advanceBase(r *repository, old, merge string) string {
	return f.setBaseTo(r, old, merge, r.git.fastForward)
}

// setBaseTo moves r's base from old to tip: with move in the worktree
// where it is checked out, or by ref with old as a guard. It returns why
// it couldn't, or "".
func (f *factory) setBaseTo(r *repository, old, tip string, move func(dir, commit string) (out string, err error)) string {
	base := f.repos.baseOf(r)
	list, err := r.git.worktrees()
	if err != nil {
		return err.Error()
	}
	for _, w := range list {
		if w.branch != base {
			continue
		}
		if moved := f.baseMoved(r, old); moved != "" {
			return moved
		}
		out, err := move(w.path, tip)
		if err == nil {
			return ""
		}
		if moved := f.baseMoved(r, old); moved != "" {
			return moved
		}
		return fmt.Sprintf("uncommitted changes in %s, where %s is checked out, are in the way:\n\n%s", w.path, base, out)
	}
	if err := r.git.updateRef("refs/heads/"+base, tip, old); err != nil {
		if moved := f.baseMoved(r, old); moved != "" {
			return moved
		}
		return err.Error()
	}
	return ""
}

// baseMoved says so when r's base is no longer at old.
func (f *factory) baseMoved(r *repository, old string) string {
	base := f.repos.baseOf(r)
	if tip, err := r.git.commit("refs/heads/" + base); err == nil && tip == old {
		return ""
	}
	return fmt.Sprintf("%s moved while the Factory was merging.", base)
}

// mergeOn is the base the Factory merged a Ticket into, and the merge
// commit it made there.
type mergeOn struct{ base, commit string }

// mergedOnOldBase is the merge the Factory made for the Ticket into a
// base of r's the Project no longer names, when that base still holds it.
// The Factory keeps it only in memory: a Ticket merged before a restart is
// merged again into the base the Project names then.
func (f *factory) mergedOnOldBase(r *repository, id string) (mergeOn, bool) {
	m, ok := f.stateOf(r).mergedOn[id]
	if !ok || m.base == f.repos.baseOf(r) || !r.git.isAncestor(m.commit, "refs/heads/"+m.base) {
		return mergeOn{}, false
	}
	return m, true
}

// landAt calls land with the commit that put tk's work into r's base,
// naming r unless it is the Home Repository.
func (f *factory) landAt(ctx context.Context, tk core.TicketView, r *repository, commit string) (landed bool) {
	in := core.LandInput{Ticket: core.Ref(tk.ID), Commit: commit}
	if !r.home {
		in.Repository = r.name
	}
	if _, err := f.client.Land(ctx, f.project, in); err != nil {
		if ctx.Err() == nil {
			f.printf("%s: land: %v", tk.ID, err)
		}
		return false
	}
	base, s := f.repos.baseOf(r), f.stateOf(r)
	if m, ok := s.mergedOn[tk.ID]; ok {
		base = m.base
		delete(s.mergedOn, tk.ID)
	}
	f.printf("%s: landed on %s at %s%s", tk.ID, base, short(commit), r.where())
	return true
}

func short(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}
