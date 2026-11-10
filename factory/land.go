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
	landing, ok := f.landing(ctx)
	if ok && !f.paused() && !f.halted(ctx) && f.remoteReady(landing) {
		f.land(ctx, landing)
	}
	f.sweep(ctx, landing.Method)
}

// landing reads the Project's Landing Method, with its Push option, which the human can
// change at any time, so it is read again on every poll, with whether CI
// Tracking is on. ok is false when
// it can't be read, and then nothing lands this poll.
func (f *factory) landing(ctx context.Context) (landing core.LandingView, ok bool) {
	p, err := f.client.GetProject(ctx, f.project)
	if err != nil {
		if ctx.Err() == nil {
			f.printf("warning: can't read the Project's Landing Method, so nothing lands this poll: %v", err)
		}
		return core.LandingView{}, false
	}
	f.tracksCI = p.CI.Tracking
	return p.Landing, true
}

// missingRemote says why landing can't go on when Push names a remote
// this repository doesn't have, or is "".
func (f *factory) missingRemote(landing core.LandingView) string {
	if !landing.Push {
		return ""
	}
	if f.repository.hasRemote(landing.Remote) {
		return ""
	}
	return fmt.Sprintf("Push names the remote %q, which this repository doesn't have: add it with `git remote add`, or change it on the Project's Project Options page in Code Central", landing.Remote)
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
func (f *factory) remoteReady(landing core.LandingView) bool {
	why := f.missingRemote(landing)
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
func (f *factory) land(ctx context.Context, landing core.LandingView) {
	accepted := f.landable(ctx, store.Accepted)
	sort.SliceStable(accepted, func(i, j int) bool { return acceptedAt(accepted[i]).Before(acceptedAt(accepted[j])) })
	for _, tk := range accepted {
		if f.halted(ctx) || f.paused() {
			return
		}
		if landing.Method == store.LandingBranch {
			f.landMergedBranch(ctx, tk, landing)
		} else {
			f.landAccepted(ctx, tk, landing)
		}
	}
	f.detectHandMerges(ctx, landing)
}

// landMergedBranch lands an Accepted Ticket under the Branch Landing
// Method once its work is in the base: the human merged it. Until then it
// is left as it is, with a line saying so once: never merged, never sent
// back. With Push on, its branch is pushed to the remote meanwhile.
func (f *factory) landMergedBranch(ctx context.Context, tk core.TicketView, landing core.LandingView) {
	head, what, err := f.workOf(tk)
	if err == nil && f.inBase(head) {
		f.printf("%s: merged into %s by hand", tk.ID, f.base)
		f.landAt(ctx, tk, head)
		return
	}
	if err == nil && landing.Push {
		f.pushBranch(ctx, tk, landing.Remote, head)
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
	f.printf("%s: Accepted under the Branch Landing Method: merge %s into %s and the Factory lands it", tk.ID, merge, f.base)
}

// pushBranch makes sure tk's branch, holding commit, is on remote, and
// comments when it can't be pushed yet, to be retried next poll.
func (f *factory) pushBranch(ctx context.Context, tk core.TicketView, remote, commit string) {
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
		f.reportPushed(ctx, tk, remote, tk.Branch, commit)
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
func (f *factory) detectHandMerges(ctx context.Context, landing core.LandingView) {
	for _, tk := range f.landable(ctx, mergeableStates...) {
		if f.halted(ctx) || f.paused() {
			return
		}
		if tk.Commit == "" || !f.inBase(tk.Commit) {
			continue
		}
		if tk.State == store.InReview {
			f.printf("%s: merged into %s by hand", tk.ID, f.base)
			f.landPushed(ctx, tk, landing, tk.Commit)
			continue
		}
		key := tk.ID + "@" + tk.Commit
		if !f.warnedMerged[key] {
			f.warnedMerged[key] = true
			f.printf("warning: %s is %s but its finish commit %s is already in %s: only an In Review or Accepted Ticket lands", tk.ID, tk.State, short(tk.Commit), f.base)
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
	return f.repository.isAncestor(commit, "refs/heads/"+f.base)
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
func (f *factory) landAccepted(ctx context.Context, tk core.TicketView, landing core.LandingView) {
	baseTip, err := f.repository.commit("refs/heads/" + f.base)
	if err != nil {
		f.cantLandYet(ctx, tk, fmt.Sprintf("the base, %s, isn't a branch, so the Factory can't move it.", f.base))
		return
	}
	head, what, err := f.workOf(tk)
	if err != nil {
		f.cantLandYet(ctx, tk, what+" isn't in this repository.")
		return
	}
	if f.inBase(head) {
		f.landPushed(ctx, tk, landing, head)
		return
	}
	tree, conflicts, err := f.repository.mergeTree(baseTip, head)
	if err != nil {
		f.printf("warning: %s: can't merge: %v", tk.ID, err)
		return
	}
	if len(conflicts) > 0 {
		f.sendBack(ctx, tk, head, baseTip, conflicts)
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
	f.landPushed(ctx, tk, landing, merge)
}

// landPushed lands tk at commit, which is in the base. Under Merge with
// Push on, it first makes sure the remote's base holds commit, and leaves
// tk where it is, commented, when the push fails. With CI Tracking on as
// well, the gate holds: tk lands only at its pushed commit once that
// commit's CI result is passed (ADR 0019).
func (f *factory) landPushed(ctx context.Context, tk core.TicketView, landing core.LandingView, commit string) {
	if landing.Method == store.LandingMerge && landing.Push {
		if f.tracksCI && tk.PushedCommit != nil && f.repository.isAncestor(commit, tk.PushedCommit.Commit) {
			// Pushed already: what the remote's base holds now may be a
			// later Ticket's push, which mustn't replace tk's own.
			f.landOnCI(ctx, tk)
			return
		}
		pushed, why := f.pushBase(ctx, landing.Remote, commit)
		if why != "" {
			f.cantLandYet(ctx, tk, why)
			return
		}
		if pushed {
			f.printf("%s: pushed %s to %s", tk.ID, f.base, landing.Remote)
		}
		if f.reportPushed(ctx, tk, landing.Remote, f.base, commit) {
			// Its CI result is read on a later poll.
			return
		}
	}
	f.landAt(ctx, tk, commit)
}

// landOnCI lands a Ticket the gate holds at its pushed commit once its
// CI result is passed. Until then it stays where it is, with a line for
// each CI result it waits on.
func (f *factory) landOnCI(ctx context.Context, tk core.TicketView) {
	pushed := tk.PushedCommit.Commit
	if tk.CIResult == core.CIPassed {
		f.landAt(ctx, tk, pushed)
		return
	}
	key := tk.ID + "@" + pushed + "@" + string(tk.CIResult)
	if f.waitingOnCI[key] {
		return
	}
	f.waitingOnCI[key] = true
	if tk.CIResult == core.CIFailed {
		f.printf("%s: CI failed at %s, so it stays %s", tk.ID, short(pushed), tk.State)
		return
	}
	f.printf("%s: waiting on CI at %s before it lands", tk.ID, short(pushed))
}

// reportPushed reports, while CI Tracking is on, the commit the remote's
// branch now holds as tk's pushed commit: the one CI runs on. Without a
// remote-tracking ref to read, it is pushed, the commit just pushed. It
// is reported once, when it isn't tk's pushed commit already. It returns
// whether CI tracks tk, so the gate holds it, even when the report
// failed: it is retried next poll.
func (f *factory) reportPushed(ctx context.Context, tk core.TicketView, remote, branch, pushed string) (tracked bool) {
	if !f.tracksCI {
		return false
	}
	commit, err := f.repository.commit("refs/remotes/" + remote + "/" + branch)
	if err != nil {
		commit = pushed
	}
	if tk.PushedCommit != nil && tk.PushedCommit.Commit == commit {
		return true
	}
	if _, err := f.client.ReportPushedCommit(ctx, f.project, core.ReportPushedCommitInput{Ticket: core.Ref(tk.ID), Commit: commit}); err != nil {
		if ctx.Err() == nil {
			f.printf("%s: report_pushed_commit: %v", tk.ID, err)
		}
		return true
	}
	f.printf("%s: CI tracks %s at %s", tk.ID, branch, short(commit))
	return true
}

// pushBase pushes the base to remote unless the remote's base already
// holds commit.
func (f *factory) pushBase(ctx context.Context, remote, commit string) (pushed bool, why string) {
	return f.push(ctx, remote, f.base, commit)
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
func (f *factory) sendBack(ctx context.Context, tk core.TicketView, head, baseTip string, conflicts []string) {
	var b strings.Builder
	fmt.Fprintf(&b, "The Factory couldn't land this: merging %s into %s at %s conflicts in these files:\n\n", short(head), f.base, short(baseTip))
	for _, c := range conflicts {
		fmt.Fprintf(&b, "- %s\n", c)
	}
	fmt.Fprintf(&b, "\nMerge %s into the branch, resolve the conflicts, and finish again.", f.base)
	if _, err := f.client.SendBack(ctx, f.project, core.SendBackInput{Ticket: core.Ref(tk.ID), Note: b.String()}); err != nil {
		if ctx.Err() == nil {
			f.printf("%s: send_back: %v", tk.ID, err)
		}
		return
	}
	f.printf("%s: conflicts with %s at %s in %s: sent back to Ready", tk.ID, f.base, short(baseTip), strings.Join(conflicts, ", "))
}

// advanceBase fast-forwards the base from old to merge: in the checkout
// where it is checked out, or by ref with old as a guard. It returns why
// it couldn't, or "".
func (f *factory) advanceBase(old, merge string) string {
	list, err := f.repository.worktrees()
	if err != nil {
		return err.Error()
	}
	for _, w := range list {
		if w.branch != f.base {
			continue
		}
		if moved := f.baseMoved(old); moved != "" {
			return moved
		}
		out, err := f.repository.fastForward(w.path, merge)
		if err == nil {
			return ""
		}
		if moved := f.baseMoved(old); moved != "" {
			return moved
		}
		return fmt.Sprintf("uncommitted changes in %s, where %s is checked out, are in the way:\n\n%s", w.path, f.base, out)
	}
	if err := f.repository.updateRef("refs/heads/"+f.base, merge, old); err != nil {
		if moved := f.baseMoved(old); moved != "" {
			return moved
		}
		return err.Error()
	}
	return ""
}

// baseMoved says so when the base is no longer at old.
func (f *factory) baseMoved(old string) string {
	if tip, err := f.repository.commit("refs/heads/" + f.base); err == nil && tip == old {
		return ""
	}
	return fmt.Sprintf("%s moved while the Factory was merging.", f.base)
}

// landAt calls land with the commit that put tk's work into the base.
func (f *factory) landAt(ctx context.Context, tk core.TicketView, commit string) {
	if _, err := f.client.Land(ctx, f.project, core.LandInput{Ticket: core.Ref(tk.ID), Commit: commit}); err != nil {
		if ctx.Err() == nil {
			f.printf("%s: land: %v", tk.ID, err)
		}
		return
	}
	f.printf("%s: landed on %s at %s", tk.ID, f.base, short(commit))
}

func short(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}
