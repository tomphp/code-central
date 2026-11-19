package factory

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
)

// landing reads the Project, whose Landing Method, with its Push option, the
// human can change at any time, so it is read again on every poll, with
// whether CI Tracking is on. Its Landing Policy is what those make. ok is
// false when it can't be read, and then nothing lands this poll.
func (l *lander) landing(ctx context.Context) (p core.ProjectView, ok bool) {
	p, err := l.client.GetProject(ctx, l.project)
	if err != nil {
		if ctx.Err() == nil {
			l.printf("warning: can't read the Project's Landing Method, so nothing lands this poll: %v", err)
		}
		return core.ProjectView{}, false
	}
	l.setBase(p.Landing.Base)
	return p, true
}

// reportPushRemote reports the URL of the Home's own Push remote in p, or
// "" when its clone has none, making the Home local-only (Spec
// code-central-890): at start, when the name changes, and when the remote
// has gone while p has the Home pushed; one added later waits for the next
// start. It returns p as the report left it.
func (l *lander) reportPushRemote(ctx context.Context, p core.ProjectView) core.ProjectView {
	i := slices.IndexFunc(p.Repositories, func(v core.RepositoryView) bool { return v.Home })
	if i < 0 {
		return p
	}
	home := p.Repositories[i]
	url, err := l.repos.home.git.remoteURL(home.PushRemote)
	if err != nil {
		url = ""
	}
	if l.remoteReported && home.PushRemote == l.reportedRemote && (url != "" || home.LocalOnly) {
		return p
	}
	got, err := l.client.ReportPushRemote(ctx, l.project, core.ReportPushRemoteInput{URL: url, Full: true})
	if err != nil {
		if ctx.Err() == nil && !l.reportRemoteFailing {
			l.printf("warning: can't report the Push remote's URL to Code Central: %v", err)
		}
		l.reportRemoteFailing = true
		return p
	}
	l.reportRemoteFailing = false
	l.reportedRemote, l.remoteReported = home.PushRemote, true
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
func (l *lander) land(ctx context.Context, p core.ProjectView) {
	if l.stateOf(l.repos.home).missing && !l.repos.several() {
		return
	}
	accepted := l.landable(ctx, domain.Accepted)
	sort.SliceStable(accepted, func(i, j int) bool { return acceptedAt(accepted[i]).Before(acceptedAt(accepted[j])) })
	var held map[string]bool
	if !l.stateOf(l.repos.home).missing {
		held = l.landSpecsOff(ctx, accepted, p)
	}
	policies := p.LandingPolicies()
	// Each base its Landing Policy keeps level is brought up to the
	// remote's before anything lands: never a local-only one's, which a
	// Home that lost its remote under Pull Request is.
	for _, r := range l.repos.ready() {
		s, own := l.stateOf(r), r.policy(policies)
		if !s.missing && !r.localOnly && own.KeepsBaseLevel() {
			s.current = l.syncBase(ctx, r, r.remote, own.PushesBase(), accepted)
		}
	}
	// Under Merge + Push a base that isn't level lands nothing touching
	// it: not a Ticket, nor a merge into a Spec Branch cut from it.
	homeReady := l.held(l.repos.home, policies) == ""
	if !homeReady && !l.repos.several() {
		return
	}
	if policies.Any(domain.LandingPolicy.CIGateHolds) {
		l.reportTip(ctx, policies)
	}
	for _, tk := range accepted {
		if l.hooks.stopped(ctx) {
			return
		}
		if held[tk.ID] {
			continue
		}
		// The finish check again before the Ticket's first landing step,
		// which a restart or an Auto-accept may have come to before the
		// check after its turn.
		if tk.SpecMerge == nil && !tk.PartlyLanded {
			if passed, _ := checkFinish(ctx, l.repos, l.client, l.project, tk); !passed {
				continue
			}
		}
		ws, why := l.workToLand(tk, policies)
		if why != "" {
			if key := tk.ID + "@" + why; !l.saidHeld[key] {
				l.saidHeld[key] = true
				l.printf("%s: waits to land: %s", tk.ID, why)
			}
			continue
		}
		l.landEach(ctx, tk, p, ws)
	}
	// Ready Specs land in each Repository by its Landing, and hand merges
	// are detected, each held back only by the broken bases of the
	// Repositories it touches.
	l.landSpecs(ctx, p)
	l.detectHandMerges(ctx, p)
}

// landEach lands tk in each of ws, its Changed Repositories, by the route
// that Repository's own Landing Policy gives it (Spec code-central-890):
// by Merge the Factory merges it into the base there and, with Push on,
// pushes the base before landing it; by Spec Branch it merges it into its
// Spec Branch there; by Pull Request it pushes its branch there for its
// pull request, and lands it there once that is merged; by Branch it
// leaves it for the human to merge there, and lands it there once
// merged. Every merge the Factory makes, and every rebase of its branch
// before a push under Rebase, is worked out before tk lands in any
// Repository: one that conflicts, or can't be worked out this poll, moves
// nothing of tk anywhere (ADR 0049). Then it goes through them in the
// landing order, and stops at a merge it can't land yet, to go on from
// there next poll. tk is Partly landed until it has landed in each.
func (l *lander) landEach(ctx context.Context, tk core.TicketView, p core.ProjectView, ws []changedWork) {
	policies := p.LandingPolicies()
	onSpec := l.waitsOnSpecBranch(tk)
	routes := make([]domain.LandingRoute, len(ws))
	var merges, specs, pullRequests []changedWork
	for i, w := range ws {
		routes[i] = w.r.policy(policies).Route(onSpec, pullRequestMergedIn(tk, w.r))
		switch routes[i] {
		case domain.RouteMerge:
			merges = append(merges, w)
		case domain.RouteSpecBranch:
			specs = append(specs, w)
		case domain.RoutePullRequest:
			pullRequests = append(pullRequests, w)
		}
	}
	plans, conflicts, ok := l.planMerges(ctx, tk, policies, merges)
	if !ok {
		return
	}
	var rebases map[*repository]pushRebase
	if !onSpec {
		var more []landConflict
		if rebases, more, ok = l.planPushRebases(ctx, tk, policies, pullRequests); !ok {
			return
		}
		conflicts = append(conflicts, more...)
	}
	if len(conflicts) > 0 {
		l.sendBackConflicts(ctx, tk, "base", conflicts)
		return
	}
	if len(specs) > 0 && !l.mergeIntoSpecBranch(ctx, tk, policies, specs) {
		return
	}
	for i, w := range ws {
		switch routes[i] {
		case domain.RouteMerge:
			if pl, ok := plans[w.r]; ok && !l.mergeAndLand(ctx, tk, p, pl) {
				return
			}
		case domain.RoutePullRequest:
			if rb, ok := rebases[w.r]; ok && !l.rebaseForPush(ctx, tk, w.r, rb) {
				return
			}
			if changedElsewhere(l.repos.home.name, tk) {
				l.landPullRequestIn(ctx, tk, policies, w.r, onSpec)
			} else {
				// Its work is in the Home Repository alone, which lands
				// by the Ticket's own pull request.
				l.landPullRequest(ctx, tk, p)
			}
		case domain.RouteBranch:
			l.landMergedBranch(ctx, tk, policies, w)
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
func (l *lander) landMergedBranch(ctx context.Context, tk core.TicketView, policies core.LandingPolicies, w changedWork) {
	r := w.r
	head, what, err := l.workIn(r, tk)
	switch {
	case err == nil && w.landed != "" && r.git.isAncestor(head, w.landed):
		return
	case err == nil && l.repos.inBaseOf(r, head):
		l.printf("%s: merged into %s by hand%s", tk.ID, l.repos.baseOf(r), r.where())
		l.landAt(ctx, tk, r, head)
		return
	case err == nil && r.policy(policies).PushesAcceptedBranch():
		l.pushBranch(ctx, tk, policies, r, head)
	}
	key := tk.ID + "@" + head + r.where()
	if l.leftOnBranch[key] {
		return
	}
	l.leftOnBranch[key] = true
	if err != nil {
		l.printf("warning: %s is Accepted under the Branch Landing Method, but %s isn't in this repository", tk.ID, what)
		return
	}
	merge := branchIn(tk, r)
	if merge == "" {
		merge = short(head)
	}
	l.printf("%s: Accepted under the Branch Landing Method: merge %s into %s%s and the Factory lands it", tk.ID, merge, l.repos.baseOf(r), r.where())
}

// branchIn is tk's branch in r: the one its finish named there.
func branchIn(tk core.TicketView, r *repository) string {
	v, _ := viewIn(tk, r)
	return v.Branch
}

// pushBranch makes sure tk's branch in r, holding commit, is on r's Push
// remote, and comments when it can't be pushed yet, to be retried next
// poll.
func (l *lander) pushBranch(ctx context.Context, tk core.TicketView, policies core.LandingPolicies, r *repository, commit string) {
	branch := branchIn(tk, r)
	if branch == "" {
		l.cantPushYet(ctx, tk, "it was finished without a branch, so there is no branch to push.")
		return
	}
	pushed, why := l.push(ctx, r, r.remote, branch, commit, false)
	if why != "" {
		l.cantPushIn(ctx, tk, r, why)
		return
	}
	if pushed {
		l.printf("%s: pushed %s to %s%s", tk.ID, branch, r.remote, r.where())
		// Only a push made now: one made before CI Tracking was turned on
		// isn't tracked.
		l.reportPushed(ctx, tk, policies, r, branch, l.remoteTip(r, r.remote, branch, commit))
	}
}

// workOf is the commit of the work the human reviewed: the finish
// commit, or with none, the branch's tip. what names it, and err says
// it isn't in the repository.
func (l *lander) workOf(tk core.TicketView) (commit, what string, err error) {
	work, what := tk.Commit, "its commit "+tk.Commit
	if work == "" {
		work, what = "refs/heads/"+tk.Branch, "its branch "+tk.Branch
	}
	commit, err = l.repos.home.git.commit(work)
	return commit, what, err
}

// workIn is workOf in r: the commit of tk's work there, its finish commit
// for r, or with none, its branch's tip there; once the Factory has
// replayed it onto its Spec Branch or the base under Rebase, its branch's
// tip, which holds the replayed commits.
func (l *lander) workIn(r *repository, tk core.TicketView) (commit, what string, err error) {
	commit, _, what, err = l.replayedWorkIn(r, tk)
	return commit, what, err
}

// replayedWorkIn is workIn, and whether it is tk's work replayed onto its
// Spec Branch or the base.
func (l *lander) replayedWorkIn(r *repository, tk core.TicketView) (commit string, replayed bool, what string, err error) {
	commit, what, err = l.finishedIn(r, tk)
	if err != nil {
		return commit, false, what, err
	}
	if tip, ok := replayedTip(r, tk.ID, branchIn(tk, r)); ok {
		return tip, true, what, nil
	}
	return commit, false, what, nil
}

// finishedIn is the commit of tk's work in r as finished: its finish
// commit for r, or with none, its branch's tip there.
func (l *lander) finishedIn(r *repository, tk core.TicketView) (commit, what string, err error) {
	if r.home {
		return l.workOf(tk)
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
func (l *lander) detectHandMerges(ctx context.Context, p core.ProjectView) {
	policies := p.LandingPolicies()
	for _, tk := range l.landable(ctx, mergeableStates...) {
		if l.hooks.stopped(ctx) {
			return
		}
		l.detectMergedPullRequests(ctx, tk, policies)
		merged := l.mergedByHand(tk, policies)
		if len(merged) == 0 {
			continue
		}
		if tk.State == domain.InReview {
			if passed, _ := checkFinish(ctx, l.repos, l.client, l.project, tk); !passed {
				continue
			}
			for _, m := range merged {
				l.printf("%s: merged into %s by hand%s", tk.ID, l.repos.baseOf(m.r), m.r.where())
				if !l.landPushed(ctx, tk, p, m.r, l.repos.baseOf(m.r), m.commit) {
					break
				}
			}
			continue
		}
		m := merged[0]
		key := tk.ID + "@" + m.commit
		if !l.warnedMerged[key] {
			l.warnedMerged[key] = true
			l.printf("warning: %s is %s but its finish commit %s is already in %s%s: only an In Review or Accepted Ticket lands", tk.ID, tk.State, short(m.commit), l.repos.baseOf(m.r), m.r.where())
		}
	}
}

// mergedByHand is tk's Changed Repositories, in landing order, whose base
// holds tk's finish commit there and which it hasn't landed in, but for
// one whose base is broken (held) under policies, and one under Pull
// Request, where its pull request lands it.
func (l *lander) mergedByHand(tk core.TicketView, policies core.LandingPolicies) []changedWork {
	ws, why := l.changedWorkOf(tk)
	if why != "" {
		return nil
	}
	var out []changedWork
	for _, w := range ws {
		if w.r.policy(policies).LandsByPullRequest() {
			continue
		}
		if w.commit == "" || l.held(w.r, policies) != "" || !l.repos.inBaseOf(w.r, w.commit) || w.landed != "" && w.r.git.isAncestor(w.commit, w.landed) {
			continue
		}
		out = append(out, w)
	}
	return out
}

// mergeableStates are the States a Ticket with a finish commit can be in
// short of Accepted, Done and Cancelled: every State but those and Draft,
// which comes before any work.
var mergeableStates = slices.DeleteFunc(slices.Clone(domain.States), func(s domain.State) bool {
	return s == domain.Draft || s == domain.Accepted || s == domain.Done || s == domain.Cancelled
})

// inBase reports whether commit is reachable from the Home Repository's
// base.
func (l *lander) inBase(commit string) bool { return l.repos.inBaseOf(l.repos.home, commit) }

// landable is every Ticket in these States, read in full by one
// find_tickets call. If the call fails it lands nothing this poll.
func (l *lander) landable(ctx context.Context, states ...domain.State) []core.TicketView {
	out, err := l.client.FindTicketViews(ctx, l.project, core.FindTicketsInput{State: states})
	if err != nil {
		if ctx.Err() == nil {
			l.printf("warning: can't find %v Tickets: %v", states, err)
		}
		return nil
	}
	return out
}

// acceptedAt is when the Ticket last moved to Accepted.
func acceptedAt(tk core.TicketView) time.Time {
	for i := len(tk.Moves) - 1; i >= 0; i-- {
		if tk.Moves[i].To == domain.Accepted {
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
func (l *lander) changedWorkOf(tk core.TicketView) ([]changedWork, string) {
	if len(tk.Repositories) == 0 {
		return []changedWork{{r: l.repos.home, commit: tk.Commit}}, ""
	}
	var out []changedWork
	for _, v := range tk.Repositories {
		r := l.repos.named(v.Name)
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
func (l *lander) workToLand(tk core.TicketView, policies core.LandingPolicies) ([]changedWork, string) {
	ws, why := l.changedWorkOf(tk)
	if why != "" {
		return nil, why
	}
	for _, w := range ws {
		if why := l.held(w.r, policies); why != "" && w.landed == "" {
			return nil, why + w.r.where()
		}
	}
	return ws, ""
}

// mergePlan is how the Factory lands a Ticket's work in one Repository,
// worked out before it lands any: at old, a merge it made into a base the
// Project no longer names; at head, when the base holds it already; or by
// merging head into the base at baseTip, making tree, unless that
// conflicts. Under Rebase, head is the Ticket's work replayed onto
// baseTip, made by replay, which moves its branch there before the merge,
// or before it lands at baseTip itself when every commit was dropped as
// already there; replaying says the conflicts are the replay's.
type mergePlan struct {
	r                   *repository
	old                 *mergeOn
	baseTip, head, tree string
	replay              *replay
	inBase, replaying   bool
	conflicts           []string
}

// planMerges works out how an Accepted Ticket's work is merged into the
// base of each of ws, the Changed Repositories it lands in by Merge, that
// it hasn't landed in (ADR 0049), by Repository, and the merges that
// conflict, in the wording of each Repository's Landing Policy,
// policies', for the Ticket to be sent back with (sendBackConflicts). ok
// is false when a merge can't be worked out yet, with a comment or a
// line saying why.
func (l *lander) planMerges(ctx context.Context, tk core.TicketView, policies core.LandingPolicies, ws []changedWork) (plans map[*repository]mergePlan, conflicts []landConflict, ok bool) {
	plans = map[*repository]mergePlan{}
	for _, w := range ws {
		rebase := w.r.policy(policies).RebaseInForce()
		pl, landed, planned := l.planMerge(ctx, tk, w, rebase)
		switch {
		case !planned:
			return nil, nil, false
		case landed:
		case len(pl.conflicts) > 0:
			conflicts = append(conflicts, landConflict{r: pl.r, head: pl.head, into: l.repos.baseOf(pl.r), tip: pl.baseTip, files: pl.conflicts, rebase: rebase, replaying: pl.replaying})
		default:
			plans[w.r] = pl
		}
	}
	return plans, conflicts, true
}

// planMerge works out how tk lands in w's Repository, or that it has
// landed there already: its landed commit holds its work. With rebase,
// Rebase in force there, work that doesn't descend from the base's tip is
// replayed onto it first (planBaseReplay), and a Ticket whose work can't
// be replayed doesn't land rather than being merged. ok is false when it
// can't be worked out this poll, with a comment or a line saying why.
func (l *lander) planMerge(ctx context.Context, tk core.TicketView, w changedWork, rebase bool) (pl mergePlan, landed, ok bool) {
	r := w.r
	pl.r = r
	if w.landed != "" {
		if head, _, err := l.workIn(r, tk); err == nil && r.git.isAncestor(head, w.landed) {
			return pl, true, true
		}
	}
	if old, ok := l.mergedOnOldBase(r, tk.ID); ok {
		// Merged before the base changed and not landed yet, so waiting on
		// its push or CI: it finishes on the base it was merged into.
		pl.old = &old
		return pl, false, true
	}
	base := l.repos.baseOf(r)
	baseTip, err := r.git.commit("refs/heads/" + base)
	if err != nil {
		l.cantLandIn(ctx, tk, r, fmt.Sprintf("the base, %s, isn't a branch, so the Factory can't move it.", base))
		return pl, false, false
	}
	head, what, err := l.workIn(r, tk)
	if err != nil {
		l.cantLandIn(ctx, tk, r, what+" isn't in this repository.")
		return pl, false, false
	}
	pl.baseTip, pl.head = baseTip, head
	if l.repos.inBaseOf(r, head) {
		pl.inBase = true
		return pl, false, true
	}
	if rebase && !r.git.isAncestor(baseTip, head) {
		rp, files, why, err := l.planBaseReplay(r, branchIn(tk, r), head, baseTip)
		switch {
		case err != nil:
			l.printf("warning: %s: can't replay%s: %v", tk.ID, r.where(), err)
			return pl, false, false
		case why != "":
			l.cantLandIn(ctx, tk, r, why)
			return pl, false, false
		case len(files) > 0:
			pl.conflicts, pl.replaying = files, true
			return pl, false, true
		}
		pl.replay, pl.head = rp, rp.tip
		if pl.head == baseTip {
			// Every commit was dropped as already in the base: its branch
			// moves to the base's tip, and it lands there with no merge.
			pl.inBase = true
			return pl, false, true
		}
	}
	if pl.tree, pl.conflicts, err = r.git.mergeTree(baseTip, pl.head); err != nil {
		l.printf("warning: %s: can't merge%s: %v", tk.ID, r.where(), err)
		return pl, false, false
	}
	return pl, false, true
}

// mergeAndLand lands tk in pl's Repository as pl says, making the merge
// and advancing the base first when it needs one. Work replayed onto the
// base's tip under Rebase moves its branch there first, and is what is
// merged. It reports whether landing can go on to the next Repository: it
// can't when a step failed and is to be tried again next poll.
func (l *lander) mergeAndLand(ctx context.Context, tk core.TicketView, p core.ProjectView, pl mergePlan) bool {
	r, base := pl.r, l.repos.baseOf(pl.r)
	if pl.old != nil {
		return l.landPushed(ctx, tk, p, r, pl.old.base, pl.old.commit)
	}
	if pl.replay != nil {
		if err := pl.replay.apply(r, tk.ID, pl.baseTip); err != nil {
			l.cantLandIn(ctx, tk, r, err.Error()+".")
			return false
		}
		l.printf("%s: replayed onto %s at %s%s", tk.ID, base, short(pl.head), r.where())
	}
	if pl.inBase {
		return l.landPushed(ctx, tk, p, r, base, pl.head)
	}
	msg := fmt.Sprintf("Merge %s: %s", tk.ID, tk.Title)
	merge, err := r.git.commitTree(pl.tree, msg, pl.baseTip, pl.head)
	if err != nil {
		l.printf("warning: %s: can't make the merge commit%s: %v", tk.ID, r.where(), err)
		return false
	}
	if why := l.advanceBase(r, pl.baseTip, merge); why != "" {
		l.cantLandIn(ctx, tk, r, why)
		return false
	}
	l.stateOf(r).mergedOn[tk.ID] = mergeOn{base, merge}
	return l.landPushed(ctx, tk, p, r, base, merge)
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
func (l *lander) landPushed(ctx context.Context, tk core.TicketView, p core.ProjectView, r *repository, base, commit string) bool {
	policies := p.LandingPolicies()
	own := r.policy(policies)
	if !own.PushesBase() {
		return l.landAt(ctx, tk, r, commit)
	}
	v, _ := viewIn(tk, r)
	gated := own.CIGateHolds() && !v.CINotWatched
	if gated && l.pushedBase(r, v.PushedCommit, r.remote, base, commit) {
		l.landOnCI(ctx, tk, r, v)
		return true
	}
	pushed, why := l.push(ctx, r, r.remote, base, commit, false)
	if why != "" {
		l.cantLandIn(ctx, tk, r, why)
		return false
	}
	if pushed {
		l.printf("%s: pushed %s to %s%s", tk.ID, base, r.remote, r.where())
	}
	if !gated {
		return l.landAt(ctx, tk, r, commit)
	}
	// CI runs on what the push left the remote's base holding. A push
	// made on an earlier poll whose report failed is reported as what it
	// pushed then, not as a later Ticket's push the remote's base holds
	// since. Its CI result is read on a later poll.
	key := tk.ID + "@" + r.name
	switch {
	case pushed:
		commit = l.remoteTip(r, r.remote, base, commit)
	case l.unreported[key] != "":
		commit = l.unreported[key]
	}
	if l.reportPushed(ctx, tk, policies, r, base, commit) {
		delete(l.unreported, key)
	} else {
		l.unreported[key] = commit
	}
	return true
}

// pushedBase reports whether pushed, a Ticket's pushed commit in r, is
// r's base on remote holding commit: pushed and reported already, so what
// the remote's base holds now, maybe a later Ticket's push, mustn't
// replace it. A pushed commit of its branch, from Branch + Push, isn't
// one.
func (l *lander) pushedBase(r *repository, pushed *core.PushedCommitView, remote, base, commit string) bool {
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
func (l *lander) landOnCI(ctx context.Context, tk core.TicketView, r *repository, v core.ChangedRepositoryView) {
	pushed := v.PushedCommit.Commit
	if v.CIResult == core.CIPassed {
		l.landAt(ctx, tk, r, v.CICommit)
		return
	}
	if (v.CIResult == core.CIFailed || v.CIResult == core.CICancelled) && v.LandAnyway {
		l.printf("%s: landing anyway at %s%s, as you chose, though its CI %s", tk.ID, short(pushed), r.where(), v.CIResult)
		l.landAt(ctx, tk, r, pushed)
		return
	}
	key := tk.ID + "@" + pushed + "@" + string(v.CIResult)
	if l.saidOnCI[key] {
		return
	}
	l.saidOnCI[key] = true
	if v.CIResult == core.CIFailed {
		l.printf("%s: CI failed at %s%s, so it stays %s", tk.ID, short(pushed), r.where(), tk.State)
		return
	}
	if v.CIResult == core.CICancelled {
		l.printf("%s: CI cancelled at %s%s, so it stays %s", tk.ID, short(pushed), r.where(), tk.State)
		return
	}
	l.printf("%s: waiting on CI at %s%s before it lands", tk.ID, short(pushed), r.where())
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
func (l *lander) remoteTip(r *repository, remote, branch, pushed string) string {
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
func (l *lander) reportPushed(ctx context.Context, tk core.TicketView, policies core.LandingPolicies, r *repository, branch, commit string) (ok bool) {
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
	if _, err := l.client.ReportPushedCommit(ctx, l.project, in); err != nil {
		if ctx.Err() == nil {
			l.printf("%s: report_pushed_commit%s: %v", tk.ID, r.where(), err)
		}
		return false
	}
	if own.TracksCIResults() {
		l.printf("%s: CI tracks %s at %s%s", tk.ID, branch, short(commit), r.where())
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
func (l *lander) reportTip(ctx context.Context, policies core.LandingPolicies) {
	type tipIn struct {
		r   *repository
		tip string
	}
	var tips []tipIn
	for _, r := range l.repos.ready() {
		if !r.policy(policies).CIGateHolds() || l.held(r, policies) != "" {
			continue
		}
		if tip, err := r.git.commit("refs/remotes/" + r.remote + "/" + l.repos.baseOf(r)); err == nil {
			tips = append(tips, tipIn{r, tip})
		}
	}
	if len(tips) == 0 {
		return
	}
	for _, tk := range l.landable(ctx, domain.Accepted, domain.InReview) {
		if l.hooks.stopped(ctx) {
			return
		}
		for _, t := range tips {
			v, ok := viewIn(tk, t.r)
			switch {
			case !ok || v.Landed != nil || v.PushedCommit == nil:
			case v.PushedCommit.Commit == t.tip || !t.r.git.isAncestor(v.PushedCommit.Commit, t.tip):
			case tk.State == domain.InReview && (v.Commit == "" || !l.repos.inBaseOf(t.r, v.Commit)):
			default:
				l.reportPushed(ctx, tk, policies, t.r, l.repos.baseOf(t.r), t.tip)
			}
		}
	}
}

// push pushes branch to remote unless its remote-tracking ref already
// holds commit: a push that succeeded moves that ref, so the Factory
// needs no state of its own. Without force it never forces, so a remote
// that has moved ahead is left as it is. With force, which only the Spec
// Branch's and a Ticket's branch's pushes under Rebase set (ADR 0067), a
// remote branch that commit doesn't descend from, as after a rebase, is
// replaced, but only from the tip its remote-tracking ref holds
// (`--force-with-lease`), so a push the Factory hasn't fetched is never
// lost; force on r's base is refused. It returns whether it pushed, or
// why the push failed. It pushes from r's clone, to r's own remote of
// that name.
func (l *lander) push(ctx context.Context, r *repository, remote, branch, commit string, force bool) (pushed bool, why string) {
	tracking := "refs/remotes/" + remote + "/" + branch
	if r.git.isAncestor(commit, tracking) {
		return false, ""
	}
	if force && l.isBase(r, branch) {
		return false, fmt.Sprintf("the Factory never force-pushes a base, and %s is the base%s.", branch, r.where())
	}
	var lease string
	if theirs, err := r.git.commit(tracking); force && err == nil && !r.git.isAncestor(theirs, commit) {
		lease = theirs
	}
	defer l.work.busy("pushing ", branch)()
	// A stop kills it, so a push that hangs doesn't hold up the Factory.
	if out, err := r.git.push(ctx, remote, branch, lease); err != nil {
		return false, fmt.Sprintf("couldn't push %s to %s:\n\n```\n%s\n```", branch, remote, out)
	}
	return true, ""
}

// isBase reports whether branch is r's base, or for the Home Repository a
// base the Project named earlier in this run.
func (l *lander) isBase(r *repository, branch string) bool {
	return branch == l.repos.baseOf(r) || r.home && slices.Contains(l.oldBases, branch)
}

// cantLandYet leaves a Ticket whose clean merge can't land where it is,
// to be retried next poll. Each distinct reason is commented once.
func (l *lander) cantLandYet(ctx context.Context, tk core.TicketView, why string) {
	l.commentOnce(ctx, tk, "can't land yet", "The Factory can't land this yet", why)
}

// cantLandIn is cantLandYet for a landing step in r. With several
// Repositories the comment names r, so Accepted but not landed can give
// it as why tk hasn't landed there.
func (l *lander) cantLandIn(ctx context.Context, tk core.TicketView, r *repository, why string) {
	if !l.repos.several() {
		l.cantLandYet(ctx, tk, why)
		return
	}
	// Once since tk entered its State, as that is how far back Accepted
	// but not landed reads it.
	var since time.Time
	if n := len(tk.Moves); n > 0 {
		since = tk.Moves[n-1].Time
	}
	l.postOnce(ctx, tk, "can't land yet in "+r.name, why, core.CantLandComment(r.name, why, tk.State), since)
}

// cantPushYet leaves an Accepted Ticket whose branch couldn't be pushed
// where it is, to be retried next poll. Each distinct reason is commented
// once.
func (l *lander) cantPushYet(ctx context.Context, tk core.TicketView, why string) {
	l.commentOnce(ctx, tk, "can't push yet", "The Factory can't push this Ticket's branch yet", why)
}

// cantPushIn is cantPushYet for a push in r, which the comment names
// unless it is the Home Repository.
func (l *lander) cantPushIn(ctx context.Context, tk core.TicketView, r *repository, why string) {
	if !r.home {
		why = fmt.Sprintf("in %s, %s", r.name, why)
	}
	l.cantPushYet(ctx, tk, why)
}

// commentOnce comments on tk that the Factory can't do something yet,
// and why, unless that comment is already there, and prints a line.
func (l *lander) commentOnce(ctx context.Context, tk core.TicketView, line, lead, why string) {
	l.postOnce(ctx, tk, line, why, fmt.Sprintf("%s: %s\n\nIt stays %s and the Factory tries again on every poll.", lead, why, tk.State), time.Time{})
}

// postOnce comments body on tk unless that comment is already there since
// since, and prints a line saying why.
func (l *lander) postOnce(ctx context.Context, tk core.TicketView, line, why, body string, since time.Time) {
	for _, c := range tk.Comments {
		if c.Body == body && !c.Time.Before(since) {
			return
		}
	}
	l.printf("%s: %s: %s", tk.ID, line, strings.TrimSuffix(firstLine(why), ":"))
	if _, err := l.client.Comment(ctx, l.project, core.CommentInput{Ticket: core.Ref(tk.ID), Body: body}); err != nil && ctx.Err() == nil {
		l.printf("%s: comment: %v", tk.ID, err)
	}
}

// firstLine is s up to its first newline.
func firstLine(s string) string {
	l, _, _ := strings.Cut(s, "\n")
	return l
}

// sendBack sends a Ticket whose work conflicts with the base back to
// Ready, so a later Claim resolves the conflict and the human reviews it
// again: under Rebase by rebasing its branch onto c.into, and otherwise
// by merging c.into into its branch. A Spec merge Ticket whose Spec
// Branch went on while it was open rebases those commits onto its branch
// instead, since its branch already is the Spec Branch rebased, and the
// Factory then takes the branch as holding them. It reports whether the
// Ticket was sent back.
func (l *lander) sendBack(ctx context.Context, tk core.TicketView, c landConflict) bool {
	var b strings.Builder
	fmt.Fprintf(&b, "The Factory couldn't land this: %s at %s conflicts in these files:\n\n", c.what(), short(c.tip))
	for _, f := range c.files {
		fmt.Fprintf(&b, "- %s\n", f)
	}
	switch {
	case c.replacing:
		fmt.Fprintf(&b, "\nRebase %s's commits since %s onto the branch, resolve the conflicts, and finish again. The Factory then takes the branch as holding them, and moves %s to it.", c.into, short(c.fork), c.into)
	case c.rebase:
		fmt.Fprintf(&b, "\nRebase the branch onto %s, resolve the conflicts, and finish again.", c.into)
	default:
		fmt.Fprintf(&b, "\nMerge %s into the branch, resolve the conflicts, and finish again.", c.into)
	}
	if _, err := l.client.SendBack(ctx, l.project, core.SendBackInput{Ticket: core.Ref(tk.ID), Note: b.String()}); err != nil {
		if ctx.Err() == nil {
			l.printf("%s: send_back: %v", tk.ID, err)
		}
		return false
	}
	l.printf("%s: conflicts with %s at %s in %s: sent back to Ready", tk.ID, c.into, short(c.tip), strings.Join(c.files, ", "))
	return true
}

// landConflict is how merging a Ticket's work in r, head, into the
// branch into at tip conflicts, or replaying it onto into, or, for a
// Spec merge Ticket under Rebase, replaying into's commits since fork
// onto head: in files.
type landConflict struct {
	r                     *repository
	head, into, tip, fork string
	files                 []string
	// rebase is whether Rebase is in force, so the Ticket is asked to
	// rebase its branch, replaying whether the Factory replayed it, and
	// replacing whether it replayed into's later commits onto head, to
	// move into there.
	rebase, replaying, replacing bool
}

// what is what the Factory tried that conflicts: merging head into into,
// replaying it onto into, or replaying into's commits since fork onto it.
func (c landConflict) what() string {
	switch {
	case c.replacing:
		return fmt.Sprintf("replaying %s's commits since %s onto %s", c.into, short(c.fork), short(c.head))
	case c.replaying:
		return fmt.Sprintf("replaying %s onto %s", short(c.head), c.into)
	}
	return fmt.Sprintf("merging %s into %s", short(c.head), c.into)
}

// sendBackConflicts is sendBack for a Ticket whose work conflicts with
// its onto, its "base" or its "Spec Branch", in each of conflicts'
// Repositories: one note names each, with its files (ADR 0049), and says
// to rebase the branch in each when Rebase is in force in any of them,
// and to merge otherwise. The Factory has merged it into none of them
// this time. With the Home Repository alone it is sendBack. It reports
// whether the Ticket was sent back.
func (l *lander) sendBackConflicts(ctx context.Context, tk core.TicketView, onto string, conflicts []landConflict) bool {
	if !l.repos.several() {
		return l.sendBack(ctx, tk, conflicts[0])
	}
	var b strings.Builder
	var names []string
	rebase := slices.ContainsFunc(conflicts, func(c landConflict) bool { return c.rebase })
	replacing := slices.ContainsFunc(conflicts, func(c landConflict) bool { return c.replacing })
	switch {
	case replacing:
		fmt.Fprintf(&b, "The Factory couldn't land this: replaying the %s's later commits onto it conflicts in these Repositories, so it moved the %s in none of them:\n\n", onto, onto)
	case slices.ContainsFunc(conflicts, func(c landConflict) bool { return c.replaying }):
		fmt.Fprintf(&b, "The Factory couldn't land this: replaying it onto the %s conflicts in these Repositories, so it replayed it onto none of them:\n\n", onto)
	default:
		fmt.Fprintf(&b, "The Factory couldn't land this: merging it into the %s conflicts in these Repositories, so it merged it into none of them:\n\n", onto)
	}
	for _, c := range conflicts {
		names = append(names, c.r.name)
		fmt.Fprintf(&b, "- %s: %s at %s conflicts in these files:\n", c.r.name, c.what(), short(c.tip))
		for _, file := range c.files {
			fmt.Fprintf(&b, "  - %s\n", file)
		}
	}
	switch {
	case replacing:
		fmt.Fprintf(&b, "\nIn each, rebase its %s's commits since the fork point named above onto the branch, resolve the conflicts, and finish again. The Factory then takes the branch as holding them, and moves the %s to it.", onto, onto)
	case rebase:
		fmt.Fprintf(&b, "\nIn each, rebase the branch onto its %s, resolve the conflicts, and finish again.", onto)
	default:
		fmt.Fprintf(&b, "\nIn each, merge its %s into the branch, resolve the conflicts, and finish again.", onto)
	}
	if _, err := l.client.SendBack(ctx, l.project, core.SendBackInput{Ticket: core.Ref(tk.ID), Note: b.String()}); err != nil {
		if ctx.Err() == nil {
			l.printf("%s: send_back: %v", tk.ID, err)
		}
		return false
	}
	l.printf("%s: conflicts with the %s in %s: sent back to Ready", tk.ID, onto, strings.Join(names, ", "))
	return true
}

// advanceBase fast-forwards r's base from old to merge. It returns why it
// couldn't, or "".
func (l *lander) advanceBase(r *repository, old, merge string) string {
	return l.setBaseTo(r, old, merge, r.git.fastForward)
}

// setBaseTo moves r's base from old to tip: with move in the worktree
// where it is checked out, or by ref with old as a guard. It returns why
// it couldn't, or "".
func (l *lander) setBaseTo(r *repository, old, tip string, move func(dir, commit string) (out string, err error)) string {
	base := l.repos.baseOf(r)
	list, err := r.git.worktrees()
	if err != nil {
		return err.Error()
	}
	for _, w := range list {
		if w.branch != base {
			continue
		}
		if moved := l.baseMoved(r, old); moved != "" {
			return moved
		}
		out, err := move(w.path, tip)
		if err == nil {
			return ""
		}
		if moved := l.baseMoved(r, old); moved != "" {
			return moved
		}
		return fmt.Sprintf("uncommitted changes in %s, where %s is checked out, are in the way:\n\n%s", w.path, base, out)
	}
	if err := r.git.updateRef("refs/heads/"+base, tip, old); err != nil {
		if moved := l.baseMoved(r, old); moved != "" {
			return moved
		}
		return err.Error()
	}
	return ""
}

// baseMoved says so when r's base is no longer at old.
func (l *lander) baseMoved(r *repository, old string) string {
	base := l.repos.baseOf(r)
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
func (l *lander) mergedOnOldBase(r *repository, id string) (mergeOn, bool) {
	m, ok := l.stateOf(r).mergedOn[id]
	if !ok || m.base == l.repos.baseOf(r) || !r.git.isAncestor(m.commit, "refs/heads/"+m.base) {
		return mergeOn{}, false
	}
	return m, true
}

// landAt calls land with the commit that put tk's work into r's base,
// naming r unless it is the Home Repository.
func (l *lander) landAt(ctx context.Context, tk core.TicketView, r *repository, commit string) (landed bool) {
	in := core.LandInput{Ticket: core.Ref(tk.ID), Commit: commit}
	if !r.home {
		in.Repository = r.name
	}
	if _, err := l.client.Land(ctx, l.project, in); err != nil {
		if ctx.Err() == nil {
			l.printf("%s: land: %v", tk.ID, err)
		}
		return false
	}
	base, s := l.repos.baseOf(r), l.stateOf(r)
	if m, ok := s.mergedOn[tk.ID]; ok {
		base = m.base
		delete(s.mergedOn, tk.ID)
	}
	l.printf("%s: landed on %s at %s%s", tk.ID, base, short(commit), r.where())
	return true
}

func short(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}
