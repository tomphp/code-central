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

// landAndSweep is a poll's landing: it reads the Landing Method, lands
// what it can unless the Factory is paused, halted or missing the Push
// remote, and sweeps the worktrees the Factory is done with.
func (f *factory) landAndSweep(ctx context.Context) {
	landing, policy, ok := f.landing(ctx)
	if ok {
		ok = f.locateBase(ctx, landing, policy)
	}
	if ok && !f.paused() && !f.halted(ctx) && f.remoteReady(landing, policy) {
		f.land(ctx, landing, policy)
	}
	f.sweep(ctx, landing.Method)
	f.sweepSpecBranches(ctx, landing.Method == store.LandingPullRequest)
}

// landing reads the Project's Landing Method, with its Push option, which the human can
// change at any time, so it is read again on every poll, with whether CI
// Tracking is on. It returns the Landing Policy those make. ok is false when
// it can't be read, and then nothing lands this poll. A poll that reads it
// also makes the report of what claude accepts, until one is taken.
func (f *factory) landing(ctx context.Context) (landing core.LandingView, policy store.LandingPolicy, ok bool) {
	p, err := f.client.GetProject(ctx, f.project)
	if err != nil {
		if ctx.Err() == nil {
			f.printf("warning: can't read the Project's Landing Method, so nothing lands this poll: %v", err)
		}
		return core.LandingView{}, store.LandingPolicy{}, false
	}
	f.setBase(p.Landing.Base)
	policy = landingPolicy(p)
	if !policy.KeepsBaseLevel() {
		f.baseDiverged = false
	}
	if !f.modelsReported {
		f.modelsReported = f.reportModels(ctx)
	}
	return p.Landing, policy, true
}

// missingRemote says why landing can't go on when Push names a remote
// this repository doesn't have, or is "".
func (f *factory) missingRemote(remote string, policy store.LandingPolicy) string {
	if !policy.NeedsPushRemote() {
		return ""
	}
	if f.repository.hasRemote(remote) {
		return ""
	}
	return fmt.Sprintf("Push names the remote %q, which this repository doesn't have: add it with `git remote add`, or change it on the Project's Project Options page in Code Central", remote)
}

// reportPushRemote reports the URL of the Push remote, when the
// repository has it, so the human's CI settings can offer its GitHub
// repository. It is reported even with Push off: the remote is the one
// Push would use.
func (f *factory) reportPushRemote(remote string) {
	url, err := f.gitOut(f.repo, "remote", "get-url", remote)
	if err != nil {
		return
	}
	if _, err := f.client.ReportPushRemote(context.Background(), f.project, core.ReportPushRemoteInput{URL: url}); err != nil {
		f.printf("warning: can't report the Push remote's URL to Code Central: %v", err)
	}
}

// remoteReady reports whether landing can go on this poll: it can't when
// Push names a missing remote, and a warning says so once per remote.
func (f *factory) remoteReady(landing core.LandingView, policy store.LandingPolicy) bool {
	why := f.missingRemote(landing.Remote, policy)
	if why == "" {
		// Back again: should it go missing later, that is warned of too.
		delete(f.warnedRemote, landing.Remote)
		return true
	}
	if !f.warnedRemote[landing.Remote] {
		f.warnedRemote[landing.Remote] = true
		f.printf("warning: nothing lands until this is fixed: %s", why)
	}
	return false
}

// land lands Accepted Tickets one at a time, oldest accept first (ADR
// 0006), by the Project's Landing Method, and detects merges made by
// hand. Under Merge the Factory merges each into the base and, with Push
// on, pushes the base before landing it; under Branch it leaves each for the human to
// merge, and lands it once merged. The Factory never accepts: only the
// human moves a Ticket to Accepted.
func (f *factory) land(ctx context.Context, landing core.LandingView, policy store.LandingPolicy) {
	accepted := f.landable(ctx, store.Accepted)
	sort.SliceStable(accepted, func(i, j int) bool { return acceptedAt(accepted[i]).Before(acceptedAt(accepted[j])) })
	held := f.landSpecsOff(ctx, accepted, landing, policy)
	pullRequest := landing.Method == store.LandingPullRequest
	if policy.KeepsBaseLevel() {
		// The base is brought up to the remote's before anything lands.
		f.baseCurrent = f.syncBase(ctx, landing.Remote, policy.PushesBase(), accepted)
		if !f.baseCurrent && policy.PushesBase() {
			// Under Merge + Push a base that isn't level lands nothing: not
			// a Ticket, nor a merge into a Spec Branch cut from it.
			return
		}
	}
	if policy.CIGateHolds() {
		f.reportTip(ctx, landing.Remote, policy)
	}
	for _, tk := range accepted {
		if f.halted(ctx) || f.paused() {
			return
		}
		if held[tk.ID] {
			continue
		}
		if landing.Method == store.LandingBranch {
			f.landMergedBranch(ctx, tk, landing, policy)
		} else if pullRequest && (tk.PullRequest != nil && tk.PullRequest.State == store.PRMerged || !f.waitsOnSpecBranch(tk, landing, policy)) {
			f.landPullRequest(ctx, tk, landing, policy)
		} else if f.waitsOnSpecBranch(tk, landing, policy) {
			if tk.SpecMerge == nil || !tk.SpecMerge.Counts {
				f.mergeIntoSpecBranch(ctx, tk)
			}
		} else {
			f.landAccepted(ctx, tk, landing, policy)
		}
	}
	if pullRequest {
		// A Spec Branch goes to the base through its own pull request,
		// not by a merge here: it is pushed instead.
		f.pushSpecs(ctx, accepted, landing, policy)
	} else {
		f.landSpecs(ctx, landing, policy)
	}
	f.detectHandMerges(ctx, landing, policy)
}

// landMergedBranch lands an Accepted Ticket under the Branch Landing
// Method once its work is in the base: the human merged it. Until then it
// is left as it is, with a line saying so once: never merged, never sent
// back. With Push on, its branch is pushed to the remote meanwhile.
func (f *factory) landMergedBranch(ctx context.Context, tk core.TicketView, landing core.LandingView, policy store.LandingPolicy) {
	head, what, err := f.workOf(tk)
	if err == nil && f.inBase(head) {
		f.printf("%s: merged into %s by hand", tk.ID, f.baseBranch())
		f.landAt(ctx, tk, head)
		return
	}
	if err == nil && landing.Push {
		f.pushBranch(ctx, tk, policy, landing.Remote, head)
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
func (f *factory) pushBranch(ctx context.Context, tk core.TicketView, policy store.LandingPolicy, remote, commit string) {
	if tk.Branch == "" {
		f.cantPushYet(ctx, tk, "it was finished without a branch, so there is no branch to push.")
		return
	}
	pushed, why := f.push(ctx, remote, tk.Branch, commit)
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
	commit, err = f.repository.commit(work)
	return commit, what, err
}

// detectHandMerges lands In Review Tickets whose finish commit is in the
// base: the human merged them. Under Merge + Push it lands each only once
// the base is pushed. (land sees to Accepted ones.) A Ticket merged while in any other State
// gets a warning line, once.
func (f *factory) detectHandMerges(ctx context.Context, landing core.LandingView, policy store.LandingPolicy) {
	if landing.Method == store.LandingPullRequest {
		f.detectMergedPullRequests(ctx, landing.Remote)
		return
	}
	for _, tk := range f.landable(ctx, mergeableStates...) {
		if f.halted(ctx) || f.paused() {
			return
		}
		if tk.Commit == "" || !f.inBase(tk.Commit) {
			continue
		}
		if tk.State == store.InReview {
			f.printf("%s: merged into %s by hand", tk.ID, f.baseBranch())
			f.landPushed(ctx, tk, landing, policy, f.baseBranch(), tk.Commit)
			continue
		}
		key := tk.ID + "@" + tk.Commit
		if !f.warnedMerged[key] {
			f.warnedMerged[key] = true
			f.printf("warning: %s is %s but its finish commit %s is already in %s: only an In Review or Accepted Ticket lands", tk.ID, tk.State, short(tk.Commit), f.baseBranch())
		}
	}
}

// mergeableStates are the States a Ticket with a finish commit can be in
// short of Accepted, Done and Cancelled: every State but those and Draft,
// which comes before any work.
var mergeableStates = slices.DeleteFunc(slices.Clone(store.States), func(s store.State) bool {
	return s == store.Draft || s == store.Accepted || s == store.Done || s == store.Cancelled
})

// inBase reports whether commit is reachable from the base.
func (f *factory) inBase(commit string) bool {
	return f.repository.isAncestor(commit, "refs/heads/"+f.baseBranch())
}

// landable is every Ticket in these States, read in full.
func (f *factory) landable(ctx context.Context, states ...store.State) []core.TicketView {
	rows, err := f.client.FindTickets(ctx, f.project, core.FindTicketsInput{State: states})
	if err != nil {
		if ctx.Err() == nil {
			f.printf("warning: can't find %v Tickets: %v", states, err)
		}
		return nil
	}
	var out []core.TicketView
	for _, r := range rows {
		tk, err := f.client.GetTicket(ctx, f.project, core.Ref(r.ID))
		if err != nil {
			if ctx.Err() == nil {
				f.printf("%s: get_ticket: %v", r.ID, err)
			}
			continue
		}
		out = append(out, tk)
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

// landAccepted merges an Accepted Ticket's work into the base and lands
// it.
func (f *factory) landAccepted(ctx context.Context, tk core.TicketView, landing core.LandingView, policy store.LandingPolicy) {
	if old, ok := f.mergedOnOldBase(tk.ID); ok {
		// Merged before the base changed and not landed yet, so waiting on
		// its push or CI: it finishes on the base it was merged into.
		f.landPushed(ctx, tk, landing, policy, old.base, old.commit)
		return
	}
	baseTip, err := f.repository.commit("refs/heads/" + f.baseBranch())
	if err != nil {
		f.cantLandYet(ctx, tk, fmt.Sprintf("the base, %s, isn't a branch, so the Factory can't move it.", f.baseBranch()))
		return
	}
	head, what, err := f.workOf(tk)
	if err != nil {
		f.cantLandYet(ctx, tk, what+" isn't in this repository.")
		return
	}
	if f.inBase(head) {
		f.landPushed(ctx, tk, landing, policy, f.baseBranch(), head)
		return
	}
	tree, conflicts, err := f.repository.mergeTree(baseTip, head)
	if err != nil {
		f.printf("warning: %s: can't merge: %v", tk.ID, err)
		return
	}
	if len(conflicts) > 0 {
		f.sendBack(ctx, tk, head, f.baseBranch(), baseTip, conflicts)
		return
	}
	msg := fmt.Sprintf("Merge %s: %s", tk.ID, tk.Title)
	merge, err := f.repository.commitTree(tree, msg, baseTip, head)
	if err != nil {
		f.printf("warning: %s: can't make the merge commit: %v", tk.ID, err)
		return
	}
	if why := f.advanceBase(baseTip, merge); why != "" {
		f.cantLandYet(ctx, tk, why)
		return
	}
	f.mergedOn[tk.ID] = mergeOn{f.baseBranch(), merge}
	f.landPushed(ctx, tk, landing, policy, f.baseBranch(), merge)
}

// landPushed lands tk at commit, which is in the base. Under Merge with
// Push on, it first makes sure the remote's base holds commit, and leaves
// tk where it is, commented, when the push fails. With CI Tracking on as
// well, the CI gate holds: tk lands only once its CI result is passed,
// at the pushed commit that passed (ADR 0019, ADR 0034).
func (f *factory) landPushed(ctx context.Context, tk core.TicketView, landing core.LandingView, policy store.LandingPolicy, base, commit string) {
	if !policy.PushesBase() {
		f.landAt(ctx, tk, commit)
		return
	}
	if policy.CIGateHolds() && f.pushedBase(tk, landing.Remote, base, commit) {
		f.landOnCI(ctx, tk)
		return
	}
	pushed, why := f.push(ctx, landing.Remote, base, commit)
	if why != "" {
		f.cantLandYet(ctx, tk, why)
		return
	}
	if pushed {
		f.printf("%s: pushed %s to %s", tk.ID, base, landing.Remote)
	}
	if !policy.CIGateHolds() {
		f.landAt(ctx, tk, commit)
		return
	}
	// CI runs on what the push left the remote's base holding. A push
	// made on an earlier poll whose report failed is reported as what it
	// pushed then, not as a later Ticket's push the remote's base holds
	// since. Its CI result is read on a later poll.
	switch {
	case pushed:
		commit = f.remoteTip(landing.Remote, base, commit)
	case f.unreported[tk.ID] != "":
		commit = f.unreported[tk.ID]
	}
	if f.reportPushed(ctx, tk, policy, base, commit) {
		delete(f.unreported, tk.ID)
	} else {
		f.unreported[tk.ID] = commit
	}
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
	return f.repository.isAncestor(commit, pushed) && f.repository.isAncestor(pushed, "refs/remotes/"+remote+"/"+base)
}

// landOnCI lands a Ticket the CI gate holds once its CI result is
// passed, at the pushed commit that passed, or once the human chose Land
// anyway on a failed one, at its latest. Until then it stays where it
// is, with a line for each CI result it waits on.
func (f *factory) landOnCI(ctx context.Context, tk core.TicketView) {
	pushed := tk.PushedCommit.Commit
	if tk.CIResult == core.CIPassed {
		f.landAt(ctx, tk, tk.CICommit)
		return
	}
	if (tk.CIResult == core.CIFailed || tk.CIResult == core.CICancelled) && tk.LandAnyway {
		f.printf("%s: landing anyway at %s, as you chose, though its CI %s", tk.ID, short(pushed), tk.CIResult)
		f.landAt(ctx, tk, pushed)
		return
	}
	key := tk.ID + "@" + pushed + "@" + string(tk.CIResult)
	if f.saidOnCI[key] {
		return
	}
	f.saidOnCI[key] = true
	if tk.CIResult == core.CIFailed {
		f.printf("%s: CI failed at %s, so it stays %s", tk.ID, short(pushed), tk.State)
		return
	}
	if tk.CIResult == core.CICancelled {
		f.printf("%s: CI cancelled at %s, so it stays %s", tk.ID, short(pushed), tk.State)
		return
	}
	f.printf("%s: waiting on CI at %s before it lands", tk.ID, short(pushed))
}

// remoteTip is the commit branch's remote-tracking ref on remote holds,
// or, without one to read, pushed.
func (f *factory) remoteTip(remote, branch, pushed string) string {
	commit, err := f.repository.commit("refs/remotes/" + remote + "/" + branch)
	if err != nil {
		return pushed
	}
	return commit
}

// reportPushed reports, when the Landing Policy records pushed commits, commit, pushed as
// branch, as tk's pushed commit: the one CI runs on. It is reported once,
// when it isn't tk's pushed commit already. ok is false when the report
// failed.
func (f *factory) reportPushed(ctx context.Context, tk core.TicketView, policy store.LandingPolicy, branch, commit string) (ok bool) {
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
func (f *factory) reportTip(ctx context.Context, remote string, policy store.LandingPolicy) {
	base := f.baseBranch()
	tip, err := f.repository.commit("refs/remotes/" + remote + "/" + base)
	if err != nil {
		return
	}
	for _, tk := range f.landable(ctx, store.Accepted, store.InReview) {
		if f.halted(ctx) || f.paused() {
			return
		}
		if tk.PushedCommit == nil || tk.PushedCommit.Commit == tip || !f.repository.isAncestor(tk.PushedCommit.Commit, tip) {
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
// failed.
func (f *factory) push(ctx context.Context, remote, branch, commit string) (pushed bool, why string) {
	if f.repository.isAncestor(commit, "refs/remotes/"+remote+"/"+branch) {
		return false, ""
	}
	// A stop kills it, so a push that hangs doesn't hold up the Factory.
	if out, err := f.repository.push(ctx, remote, branch); err != nil {
		return false, fmt.Sprintf("couldn't push %s to %s:\n\n```\n%s\n```", branch, remote, out)
	}
	return true, ""
}

// cantLandYet leaves a Ticket whose clean merge can't land where it is,
// to be retried next poll. Each distinct reason is commented once.
func (f *factory) cantLandYet(ctx context.Context, tk core.TicketView, why string) {
	f.commentOnce(ctx, tk, "can't land yet", "The Factory can't land this yet", why)
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
	body := fmt.Sprintf("%s: %s\n\nIt stays %s and the Factory tries again on every poll.", lead, why, tk.State)
	for _, c := range tk.Comments {
		if c.Body == body {
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

// advanceBase fast-forwards the base from old to merge. It returns why it
// couldn't, or "".
func (f *factory) advanceBase(old, merge string) string {
	return f.setBaseTo(old, merge, f.repository.fastForward)
}

// setBaseTo moves the base from old to tip: with move in the worktree
// where it is checked out, or by ref with old as a guard. It returns why
// it couldn't, or "".
func (f *factory) setBaseTo(old, tip string, move func(dir, commit string) (out string, err error)) string {
	list, err := f.repository.worktrees()
	if err != nil {
		return err.Error()
	}
	for _, w := range list {
		if w.branch != f.baseBranch() {
			continue
		}
		if moved := f.baseMoved(old); moved != "" {
			return moved
		}
		out, err := move(w.path, tip)
		if err == nil {
			return ""
		}
		if moved := f.baseMoved(old); moved != "" {
			return moved
		}
		return fmt.Sprintf("uncommitted changes in %s, where %s is checked out, are in the way:\n\n%s", w.path, f.baseBranch(), out)
	}
	if err := f.repository.updateRef("refs/heads/"+f.baseBranch(), tip, old); err != nil {
		if moved := f.baseMoved(old); moved != "" {
			return moved
		}
		return err.Error()
	}
	return ""
}

// baseMoved says so when the base is no longer at old.
func (f *factory) baseMoved(old string) string {
	if tip, err := f.repository.commit("refs/heads/" + f.baseBranch()); err == nil && tip == old {
		return ""
	}
	return fmt.Sprintf("%s moved while the Factory was merging.", f.baseBranch())
}

// mergeOn is the base the Factory merged a Ticket into, and the merge
// commit it made there.
type mergeOn struct{ base, commit string }

// mergedOnOldBase is the merge the Factory made for the Ticket into a
// base the Project no longer names, when that base still holds it. The
// Factory keeps it only in memory: a Ticket merged before a restart is
// merged again into the base the Project names then.
func (f *factory) mergedOnOldBase(id string) (mergeOn, bool) {
	m, ok := f.mergedOn[id]
	if !ok || m.base == f.baseBranch() || !f.repository.isAncestor(m.commit, "refs/heads/"+m.base) {
		return mergeOn{}, false
	}
	return m, true
}

// landAt calls land with the commit that put tk's work into the base.
func (f *factory) landAt(ctx context.Context, tk core.TicketView, commit string) {
	if _, err := f.client.Land(ctx, f.project, core.LandInput{Ticket: core.Ref(tk.ID), Commit: commit}); err != nil {
		if ctx.Err() == nil {
			f.printf("%s: land: %v", tk.ID, err)
		}
		return
	}
	base := f.baseBranch()
	if m, ok := f.mergedOn[tk.ID]; ok {
		base = m.base
		delete(f.mergedOn, tk.ID)
	}
	f.printf("%s: landed on %s at %s", tk.ID, base, short(commit))
}

func short(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}

// landingPolicy is the Landing Policy of p's Landing Method, Push, Land per
// Spec and CI Tracking.
func landingPolicy(p core.ProjectView) store.LandingPolicy {
	return store.NewLandingPolicy(p.Landing.Method, p.Landing.Push, p.Landing.LandPerSpec, p.CI.Tracking)
}
