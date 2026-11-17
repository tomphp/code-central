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
// Repositories, makes sure each Repository has a clone, lands what it can
// unless the Factory is paused, halted or missing the Push remote, and
// sweeps the worktrees the Factory is done with.
func (f *factory) landAndSweep(ctx context.Context) {
	p, ok := f.landing(ctx)
	if ok {
		f.syncRepositories(ctx, p)
		f.locateBases(ctx, p)
	}
	if ok && !f.paused() && !f.halted(ctx) && f.remoteReady(p) {
		f.land(ctx, p)
	}
	policy := p.LandingPolicy()
	f.sweep(ctx, policy)
	f.sweepSpecBranches(ctx, policy)
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
	if !p.LandingPolicy().KeepsBaseLevel() {
		for _, s := range f.bases {
			s.diverged = false
		}
	}
	if !f.modelsReported {
		f.modelsReported = f.reportModels(ctx)
	}
	return p, true
}

// missingRemote says why landing can't go on when Push names a remote
// this repository doesn't have, or is "".
func (f *factory) missingRemote(remote string, policy core.LandingPolicy) string {
	if !policy.NeedsPushRemote() {
		return ""
	}
	if f.home.git.hasRemote(remote) {
		return ""
	}
	return fmt.Sprintf("Push names the remote %q, which this repository doesn't have: add it with `git remote add`, or change it on the Project's Project Options page in Code Central", remote)
}

// reportPushRemote reports the URL of the Push remote, when the
// repository has it, so the human's CI settings can offer its GitHub
// repository. It is reported even with Push off: the remote is the one
// Push would use.
func (f *factory) reportPushRemote(remote string) {
	url, err := f.home.git.remoteURL(remote)
	if err != nil {
		return
	}
	if _, err := f.client.ReportPushRemote(context.Background(), f.project, core.ReportPushRemoteInput{URL: url}); err != nil {
		f.printf("warning: can't report the Push remote's URL to Code Central: %v", err)
	}
}

// remoteReady reports whether landing can go on this poll: it can't when
// Push names a missing remote, and a warning says so once per remote.
func (f *factory) remoteReady(p core.ProjectView) bool {
	why := f.missingRemote(p.Landing.Remote, p.LandingPolicy())
	if why == "" {
		// Back again: should it go missing later, that is warned of too.
		delete(f.warnedRemote, p.Landing.Remote)
		return true
	}
	if !f.warnedRemote[p.Landing.Remote] {
		f.warnedRemote[p.Landing.Remote] = true
		f.printf("warning: nothing lands until this is fixed: %s", why)
	}
	return false
}

// land lands Accepted Tickets one at a time, oldest accept first (ADR
// 0006), each by the route the Landing Policy gives it, and detects merges
// made by hand. By Merge the Factory merges it into the base and, with
// Push on, pushes the base before landing it; by Branch it leaves it for
// the human to merge, and lands it once merged; by Pull Request it lands
// it through its pull request; by Spec Branch it merges it into
// its Spec Branch. The Factory never accepts: only the human moves a
// Ticket to Accepted.
//
// Each Repository's base is synced apart, and a broken one (missing, or
// under Merge + Push not level with the remote's) holds back only the
// Tickets landing in it; with the Home Repository alone, it holds back
// everything, as it always has (Spec code-central-848).
func (f *factory) land(ctx context.Context, p core.ProjectView) {
	if f.stateOf(f.home).missing && !f.several() {
		return
	}
	accepted := f.landable(ctx, store.Accepted)
	sort.SliceStable(accepted, func(i, j int) bool { return acceptedAt(accepted[i]).Before(acceptedAt(accepted[j])) })
	var held map[string]bool
	if !f.stateOf(f.home).missing {
		held = f.landSpecsOff(ctx, accepted, p)
	}
	policy := p.LandingPolicy()
	if policy.KeepsBaseLevel() {
		// Each base is brought up to the remote's before anything lands.
		for _, r := range f.readyRepositories() {
			if s := f.stateOf(r); !s.missing {
				s.current = f.syncBase(ctx, r, p.Landing.Remote, policy.PushesBase(), accepted)
			}
		}
	}
	// Under Merge + Push a base that isn't level lands nothing touching
	// it: not a Ticket, nor a merge into a Spec Branch cut from it.
	homeReady := f.held(f.home, policy) == ""
	if !homeReady && !f.several() {
		return
	}
	if homeReady && policy.CIGateHolds() {
		f.reportTip(ctx, p.Landing.Remote, policy)
	}
	f.landingRemote = p.Landing.Remote
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
		prMerged := tk.PullRequest != nil && tk.PullRequest.State == store.PRMerged
		route := policy.Route(f.waitsOnSpecBranch(tk), prMerged)
		ws, why := f.workToLand(tk, route, policy)
		if why != "" {
			if key := tk.ID + "@" + why; !f.saidHeld[key] {
				f.saidHeld[key] = true
				f.printf("%s: waits to land: %s", tk.ID, why)
			}
			continue
		}
		switch route {
		case core.RouteBranch:
			f.landMergedBranch(ctx, tk, p)
		case core.RoutePullRequest:
			if changedElsewhere(f.home.name, tk) {
				f.landPullRequests(ctx, tk, p)
			} else {
				f.landPullRequest(ctx, tk, p)
			}
		case core.RouteSpecBranch:
			if tk.SpecMerge == nil || !tk.SpecMerge.Counts {
				f.mergeIntoSpecBranch(ctx, tk)
			}
		case core.RouteMerge:
			f.landAccepted(ctx, tk, p, ws)
		}
	}
	// Spec Branches are the Home Repository's; a hand merge is held per
	// Repository.
	switch {
	case !homeReady:
	case policy.LandsByPullRequest():
		// A Spec Branch goes to the base through its own pull request,
		// not by a merge here: it is pushed instead.
		f.pushSpecs(ctx, accepted, p)
	default:
		f.landSpecs(ctx, p)
	}
	f.detectHandMerges(ctx, p)
}

// landMergedBranch lands an Accepted Ticket under the Branch Landing
// Method once its work is in the base: the human merged it. Until then it
// is left as it is, with a line saying so once: never merged, never sent
// back. With Push on, its branch is pushed to the remote meanwhile.
func (f *factory) landMergedBranch(ctx context.Context, tk core.TicketView, p core.ProjectView) {
	head, what, err := f.workOf(tk)
	if err == nil && f.inBase(head) {
		f.printf("%s: merged into %s by hand", tk.ID, f.baseBranch())
		f.landAt(ctx, tk, f.home, head)
		return
	}
	if policy := p.LandingPolicy(); err == nil && policy.PushesAcceptedBranch() {
		f.pushBranch(ctx, tk, policy, p.Landing.Remote, head)
	}
	key := tk.ID + "@" + head
	if f.leftOnBranch[key] {
		return
	}
	f.leftOnBranch[key] = true
	if err != nil {
		f.printf("warning: %s is Accepted under the Branch Landing Method, but %s isn't in this repository", tk.ID, what)
		return
	}
	merge := tk.Branch
	if merge == "" {
		merge = short(head)
	}
	f.printf("%s: Accepted under the Branch Landing Method: merge %s into %s and the Factory lands it", tk.ID, merge, f.baseBranch())
}

// pushBranch makes sure tk's branch, holding commit, is on remote, and
// comments when it can't be pushed yet, to be retried next poll.
func (f *factory) pushBranch(ctx context.Context, tk core.TicketView, policy core.LandingPolicy, remote, commit string) {
	if tk.Branch == "" {
		f.cantPushYet(ctx, tk, "it was finished without a branch, so there is no branch to push.")
		return
	}
	pushed, why := f.push(ctx, f.home, remote, tk.Branch, commit)
	if why != "" {
		f.cantPushYet(ctx, tk, why)
		return
	}
	if pushed {
		f.printf("%s: pushed %s to %s", tk.ID, tk.Branch, remote)
		// Only a push made now: one made before CI Tracking was turned on
		// isn't tracked.
		f.reportPushed(ctx, tk, policy, tk.Branch, f.remoteTip(remote, tk.Branch, commit))
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
	commit, err = f.home.git.commit(work)
	return commit, what, err
}

// workIn is workOf in r: the commit of tk's work there, its finish commit
// for r, or with none, its branch's tip there.
func (f *factory) workIn(r *repository, tk core.TicketView) (commit, what string, err error) {
	if r.home {
		return f.workOf(tk)
	}
	i := slices.IndexFunc(tk.Repositories, func(v core.ChangedRepositoryView) bool { return v.Name == r.name })
	if i < 0 {
		return "", "its work in Repository " + r.name, fmt.Errorf("%s holds no work in Repository %s", tk.ID, r.name)
	}
	v := tk.Repositories[i]
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
// and the Ticket is Done once it has landed in all of them.
func (f *factory) detectHandMerges(ctx context.Context, p core.ProjectView) {
	policy := p.LandingPolicy()
	if policy.LandsByPullRequest() {
		if f.held(f.home, policy) == "" {
			f.detectMergedPullRequests(ctx, p.Landing.Remote)
		}
		return
	}
	for _, tk := range f.landable(ctx, mergeableStates...) {
		if f.halted(ctx) || f.paused() {
			return
		}
		merged := f.mergedByHand(tk, policy)
		if len(merged) == 0 {
			continue
		}
		if tk.State == store.InReview {
			if passed, _ := f.checkFinish(ctx, tk); !passed {
				continue
			}
			for _, m := range merged {
				f.printf("%s: merged into %s by hand%s", tk.ID, f.baseOf(m.r), m.r.where())
				if !f.landPushed(ctx, tk, p, m.r, f.baseOf(m.r), m.commit) {
					break
				}
			}
			continue
		}
		m := merged[0]
		key := tk.ID + "@" + m.commit
		if !f.warnedMerged[key] {
			f.warnedMerged[key] = true
			f.printf("warning: %s is %s but its finish commit %s is already in %s%s: only an In Review or Accepted Ticket lands", tk.ID, tk.State, short(m.commit), f.baseOf(m.r), m.r.where())
		}
	}
}

// mergedByHand is tk's Changed Repositories, in landing order, whose base
// holds tk's finish commit there and which it hasn't landed in, but for
// one whose base is broken (held) under policy.
func (f *factory) mergedByHand(tk core.TicketView, policy core.LandingPolicy) []changedWork {
	ws, why := f.changedWorkOf(tk)
	if why != "" {
		return nil
	}
	var out []changedWork
	for _, w := range ws {
		if w.commit == "" || f.held(w.r, policy) != "" || !f.inBaseOf(w.r, w.commit) || w.landed != "" && w.r.git.isAncestor(w.commit, w.landed) {
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
func (f *factory) inBase(commit string) bool { return f.inBaseOf(f.home, commit) }

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
		return []changedWork{{r: f.home, commit: tk.Commit}}, ""
	}
	var out []changedWork
	for _, v := range tk.Repositories {
		r := f.repositories[v.Name]
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

// workToLand is tk's Changed Repositories to land by route, or says why
// tk doesn't land this poll: one of them has no clone, or a broken base
// (held), though one it has landed in doesn't hold it back. A Ticket
// landing other than by Merge lands in the Home Repository alone, so only
// its base holds that Ticket back.
func (f *factory) workToLand(tk core.TicketView, route core.LandingRoute, policy core.LandingPolicy) ([]changedWork, string) {
	if route != core.RouteMerge {
		return nil, f.held(f.home, policy)
	}
	ws, why := f.changedWorkOf(tk)
	if why != "" {
		return nil, why
	}
	for _, w := range ws {
		if why := f.held(w.r, policy); why != "" && w.landed == "" {
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

// landAccepted merges an Accepted Ticket's work into the base of each of
// ws, its Changed Repositories, it hasn't landed in, and lands it there
// (ADR 0049). It works out every merge first: a conflict in any lands
// none, and sends the Ticket back. Then, in landing order, it merges,
// advances the base, pushes and lands in each Repository in turn, and
// stops at one it can't land in yet, to go on from there next poll.
func (f *factory) landAccepted(ctx context.Context, tk core.TicketView, p core.ProjectView, ws []changedWork) {
	var plans, conflicts []mergePlan
	for _, w := range ws {
		pl, landed, ok := f.planMerge(ctx, tk, w)
		switch {
		case !ok:
			return
		case landed:
		case len(pl.conflicts) > 0:
			conflicts = append(conflicts, pl)
		default:
			plans = append(plans, pl)
		}
	}
	if len(conflicts) > 0 {
		f.sendBackConflicts(ctx, tk, conflicts)
		return
	}
	for _, pl := range plans {
		if !f.mergeAndLand(ctx, tk, p, pl) {
			return
		}
	}
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
	base := f.baseOf(r)
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
	if f.inBaseOf(r, head) {
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
	r, base := pl.r, f.baseOf(pl.r)
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
// Under Merge with Push on, it first makes sure the remote's base holds
// commit. With CI Tracking on as well, the CI gate holds in the Home
// Repository: tk lands there only once its CI result is passed, at the
// pushed commit that passed (ADR 0019, ADR 0034), and the next Repository
// goes on once the push is made, as the gate holds only the land. Any
// other Repository lands once pushed, as one Not watched does, until CI
// Tracking covers it.
func (f *factory) landPushed(ctx context.Context, tk core.TicketView, p core.ProjectView, r *repository, base, commit string) bool {
	policy := p.LandingPolicy()
	if !policy.PushesBase() {
		return f.landAt(ctx, tk, r, commit)
	}
	gated := r.home && policy.CIGateHolds()
	if gated && f.pushedBase(tk, p.Landing.Remote, base, commit) {
		f.landOnCI(ctx, tk)
		return true
	}
	pushed, why := f.push(ctx, r, p.Landing.Remote, base, commit)
	if why != "" {
		f.cantLandIn(ctx, tk, r, why)
		return false
	}
	if pushed {
		f.printf("%s: pushed %s to %s%s", tk.ID, base, p.Landing.Remote, r.where())
	}
	if !gated {
		return f.landAt(ctx, tk, r, commit)
	}
	// CI runs on what the push left the remote's base holding. A push
	// made on an earlier poll whose report failed is reported as what it
	// pushed then, not as a later Ticket's push the remote's base holds
	// since. Its CI result is read on a later poll.
	switch {
	case pushed:
		commit = f.remoteTip(p.Landing.Remote, base, commit)
	case f.unreported[tk.ID] != "":
		commit = f.unreported[tk.ID]
	}
	if f.reportPushed(ctx, tk, policy, base, commit) {
		delete(f.unreported, tk.ID)
	} else {
		f.unreported[tk.ID] = commit
	}
	return true
}

// pushedBase reports whether tk's pushed commit is the base on remote
// holding commit: pushed and reported already, so what the remote's base
// holds now, maybe a later Ticket's push, mustn't replace it. A pushed
// commit of its branch, from Branch + Push, isn't one.
func (f *factory) pushedBase(tk core.TicketView, remote, base, commit string) bool {
	if tk.PushedCommit == nil {
		return false
	}
	pushed := tk.PushedCommit.Commit
	return f.home.git.isAncestor(commit, pushed) && f.home.git.isAncestor(pushed, "refs/remotes/"+remote+"/"+base)
}

// landOnCI lands a Ticket the CI gate holds once its CI result in the
// Home Repository is passed, at the pushed commit that passed, or once the
// human chose Land anyway on a failed one there, at its latest: each
// Repository lands on its own CI result, so another's red build doesn't
// hold it back. Until then it stays where it is, with a line for each CI
// result it waits on.
func (f *factory) landOnCI(ctx context.Context, tk core.TicketView) {
	pushed := tk.PushedCommit.Commit
	result, commit, anyway := f.homeCI(tk)
	if result == core.CIPassed {
		f.landAt(ctx, tk, f.home, commit)
		return
	}
	if (result == core.CIFailed || result == core.CICancelled) && anyway {
		f.printf("%s: landing anyway at %s, as you chose, though its CI %s", tk.ID, short(pushed), result)
		f.landAt(ctx, tk, f.home, pushed)
		return
	}
	key := tk.ID + "@" + pushed + "@" + string(result)
	if f.saidOnCI[key] {
		return
	}
	f.saidOnCI[key] = true
	if result == core.CIFailed {
		f.printf("%s: CI failed at %s, so it stays %s", tk.ID, short(pushed), tk.State)
		return
	}
	if result == core.CICancelled {
		f.printf("%s: CI cancelled at %s, so it stays %s", tk.ID, short(pushed), tk.State)
		return
	}
	f.printf("%s: waiting on CI at %s before it lands", tk.ID, short(pushed))
}

// homeCI is tk's CI in the Home Repository: its CI result there, the
// pushed commit that result comes from, and whether the human chose Land
// anyway on it. With the Home Repository alone, the Ticket's own fields
// are the Home's; with several, its Changed Repository's are, as its CI
// result combines every Repository's, and one that didn't change the Home
// Repository has no CI result there.
func (f *factory) homeCI(tk core.TicketView) (result core.CIResult, commit string, landAnyway bool) {
	if len(tk.Repositories) == 0 {
		return tk.CIResult, tk.CICommit, tk.LandAnyway
	}
	for _, r := range tk.Repositories {
		if r.Name == f.project {
			return r.CIResult, r.CICommit, r.LandAnyway
		}
	}
	return "", "", false
}

// remoteTip is the commit branch's remote-tracking ref on remote holds,
// or, without one to read, pushed.
func (f *factory) remoteTip(remote, branch, pushed string) string {
	return f.remoteTipIn(f.home, remote, branch, pushed)
}

// remoteTipIn is remoteTip in r.
func (f *factory) remoteTipIn(r *repository, remote, branch, pushed string) string {
	commit, err := r.git.commit("refs/remotes/" + remote + "/" + branch)
	if err != nil {
		return pushed
	}
	return commit
}

// reportPushed reports, when the Landing Policy records pushed commits, commit, pushed as
// branch, as tk's pushed commit: the one CI runs on. It is reported once,
// when it isn't tk's pushed commit already. ok is false when the report
// failed.
func (f *factory) reportPushed(ctx context.Context, tk core.TicketView, policy core.LandingPolicy, branch, commit string) (ok bool) {
	if !policy.RecordsPushedCommit() {
		return true
	}
	if tk.PushedCommit != nil && tk.PushedCommit.Commit == commit {
		return true
	}
	if _, err := f.client.ReportPushedCommit(ctx, f.project, core.ReportPushedCommitInput{Ticket: core.Ref(tk.ID), Commit: commit, Branch: branch}); err != nil {
		if ctx.Err() == nil {
			f.printf("%s: report_pushed_commit: %v", tk.ID, err)
		}
		return false
	}
	if policy.TracksCIResults() {
		f.printf("%s: CI tracks %s at %s", tk.ID, branch, short(commit))
	}
	return true
}

// reportTip reports the remote's base tip, once the base is synced, as the
// pushed commit of each Ticket waiting on CI (Accepted, or In Review and
// merged by hand, with a pushed commit) whose latest pushed commit isn't
// the tip but is an ancestor of it: the tip contains the Ticket's work,
// and if it passes, the Ticket lands (ADR 0034). A tip that doesn't
// contain the work, as after a force push, is reported to no Ticket.
func (f *factory) reportTip(ctx context.Context, remote string, policy core.LandingPolicy) {
	base := f.baseBranch()
	tip, err := f.home.git.commit("refs/remotes/" + remote + "/" + base)
	if err != nil {
		return
	}
	for _, tk := range f.landable(ctx, store.Accepted, store.InReview) {
		if f.halted(ctx) || f.paused() {
			return
		}
		if tk.PushedCommit == nil || tk.PushedCommit.Commit == tip || !f.home.git.isAncestor(tk.PushedCommit.Commit, tip) {
			continue
		}
		if tk.State == store.InReview && (tk.Commit == "" || !f.inBase(tk.Commit)) {
			continue
		}
		f.reportPushed(ctx, tk, policy, base, tip)
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
	if !f.several() {
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

// sendBackConflicts is sendBack for a Ticket whose work conflicts with
// the base in each of conflicts' Repositories: one note names each, with
// its files (ADR 0049). The Factory has merged it into none of them this
// time. With the Home Repository alone it is sendBack.
func (f *factory) sendBackConflicts(ctx context.Context, tk core.TicketView, conflicts []mergePlan) {
	if !f.several() {
		pl := conflicts[0]
		f.sendBack(ctx, tk, pl.head, f.baseOf(pl.r), pl.baseTip, pl.conflicts)
		return
	}
	var b strings.Builder
	var names []string
	b.WriteString("The Factory couldn't land this: merging it into the base conflicts in these Repositories, so it merged it into none of them:\n\n")
	for _, pl := range conflicts {
		names = append(names, pl.r.name)
		fmt.Fprintf(&b, "- %s: merging %s into %s at %s conflicts in these files:\n", pl.r.name, short(pl.head), f.baseOf(pl.r), short(pl.baseTip))
		for _, c := range pl.conflicts {
			fmt.Fprintf(&b, "  - %s\n", c)
		}
	}
	b.WriteString("\nIn each, merge its base into the branch, resolve the conflicts, and finish again.")
	if _, err := f.client.SendBack(ctx, f.project, core.SendBackInput{Ticket: core.Ref(tk.ID), Note: b.String()}); err != nil {
		if ctx.Err() == nil {
			f.printf("%s: send_back: %v", tk.ID, err)
		}
		return
	}
	f.printf("%s: conflicts with the base in %s: sent back to Ready", tk.ID, strings.Join(names, ", "))
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
	base := f.baseOf(r)
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
	base := f.baseOf(r)
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
	if !ok || m.base == f.baseOf(r) || !r.git.isAncestor(m.commit, "refs/heads/"+m.base) {
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
	base, s := f.baseOf(r), f.stateOf(r)
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
