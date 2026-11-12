package factory

import (
	"context"
	"fmt"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Under the Pull Request Landing Method the Factory pushes each accepted
// Ticket's branch and keeps the local base level with the Push remote's,
// and the Pull Request Watcher opens and follows the pull requests. A
// Ticket lands at the merge commit the Code Host reported once that commit
// is in the fetched base, whatever the merge strategy; git ancestry plays
// no part, and there is no CI gate.

// syncBase fetches remote and fast-forwards the local base to the
// remote's, never merging, rebasing or forcing. It reports whether the
// base is level with the remote's. A failed fetch is a warning, retried
// next poll; a local base holding commits the remote's doesn't is
// diverged, and stops landing and new branches until a later poll finds it
// fixed.
func (f *factory) syncBase(ctx context.Context, remote string) bool {
	if out, err := f.repository.fetch(ctx, remote); err != nil {
		if ctx.Err() == nil {
			f.printf("warning: can't fetch %s, so nothing lands this poll: %s", remote, firstLine(out))
		}
		return false
	}
	local, err := f.repository.commit("refs/heads/" + f.baseBranch())
	if err != nil {
		f.printf("warning: the base, %s, isn't a branch, so the Factory can't keep it level with %s", f.baseBranch(), remote)
		return false
	}
	theirs, err := f.repository.commit("refs/remotes/" + remote + "/" + f.baseBranch())
	if err != nil {
		// The remote has no base yet: nothing to catch up with.
		f.baseDiverged = false
		return true
	}
	switch {
	case local == theirs:
		f.noteBase(ctx, false, local, theirs)
		return true
	case f.repository.isAncestor(local, theirs):
		f.noteBase(ctx, false, local, theirs)
		if why := f.advanceBase(local, theirs); why != "" {
			f.printf("warning: can't fast-forward %s to %s yet: %s", f.baseBranch(), remote, firstLine(why))
			return false
		}
		f.printf("%s: fast-forwarded to %s/%s at %s", f.baseBranch(), remote, f.baseBranch(), short(theirs))
		return true
	}
	f.noteBase(ctx, true, local, theirs)
	return false
}

// noteBase records whether the base has diverged, with a line when that
// changes, and reports it to Code Central. It reports on every poll that
// reaches a verdict, since Code Central drops the report with the
// Heartbeat: a failed fetch reaches none, so it leaves the last report.
func (f *factory) noteBase(ctx context.Context, diverged bool, local, remote string) {
	defer f.reportBase(ctx, diverged, local, remote)
	if diverged && !f.baseDiverged {
		f.printf("warning: the base, %s, has diverged from the remote's: it is at %s and the remote's at %s. Nothing lands and no branch is cut until you reconcile them by hand", f.baseBranch(), short(local), short(remote))
	}
	if !diverged && f.baseDiverged {
		f.printf("%s: no longer diverged from the remote's", f.baseBranch())
	}
	f.baseDiverged = diverged
}

// reportBase tells Code Central whether the base is current or diverged,
// with the local and remote commits. A failed report is a warning once
// until one goes through; the next poll reports again.
func (f *factory) reportBase(ctx context.Context, diverged bool, local, remote string) {
	state := core.BaseCurrent
	if diverged {
		state = core.BaseDiverged
	}
	ctx, cancel := context.WithTimeout(ctx, postTimeout)
	defer cancel()
	_, err := f.client.ReportBase(ctx, f.project, core.ReportBaseInput{State: state, Local: local, Remote: remote})
	if err != nil && ctx.Err() == nil && !f.reportBaseFailing {
		f.printf("warning: can't report the base to Code Central: %v", err)
	}
	f.reportBaseFailing = err != nil
}

// landPullRequest sees to an Accepted Ticket under Pull Request: one with
// nothing to merge lands at once, one whose pull request is merged lands
// at the merge commit, and the rest have their branch pushed for the
// Watcher to open a pull request on.
func (f *factory) landPullRequest(ctx context.Context, tk core.TicketView, landing core.LandingView, policy store.LandingPolicy) {
	if tk.PullRequest != nil && tk.PullRequest.State == store.PRMerged {
		f.landMerged(ctx, tk)
		return
	}
	head, _, err := f.workOf(tk)
	if err == nil && tk.PullRequest == nil && !f.pushedAlready(landing.Remote, tk.Branch) && f.baseCurrent && f.inBase(head) {
		f.printf("%s: nothing to merge into %s", tk.ID, f.baseBranch())
		f.landAt(ctx, tk, head)
		return
	}
	if err != nil {
		f.cantPushYet(ctx, tk, "its work isn't in this repository.")
		return
	}
	if tk.Branch == "" {
		return
	}
	pushed, why := f.push(ctx, landing.Remote, tk.Branch, head)
	if why != "" {
		f.cantPushYet(ctx, tk, why)
		return
	}
	if pushed {
		f.printf("%s: pushed %s to %s", tk.ID, tk.Branch, landing.Remote)
	}
	f.reportPushed(ctx, tk, policy, tk.Branch, f.remoteTip(landing.Remote, tk.Branch, head))
}

// landMerged lands tk, whose pull request is reported merged, at the merge
// commit, once the fetched base holds it. Until then it waits for a later
// poll.
func (f *factory) landMerged(ctx context.Context, tk core.TicketView) {
	merge := tk.PullRequest.MergeCommit
	if !f.baseCurrent || !f.inBase(merge) {
		key := tk.ID + "@" + merge
		if !f.leftOnBranch[key] {
			f.leftOnBranch[key] = true
			f.printf("%s: its pull request is merged at %s, which %s doesn't hold yet: waiting for it", tk.ID, short(merge), f.baseBranch())
		}
		return
	}
	f.printf("%s: pull request %d merged", tk.ID, tk.PullRequest.Number)
	f.landAt(ctx, tk, merge)
}

// detectMergedPullRequests lands In Review Tickets whose pull request is
// merged, as a branch merged by hand does under Branch. A Ticket in any
// other State whose pull request is merged gets a warning line and a
// comment, once.
func (f *factory) detectMergedPullRequests(ctx context.Context) {
	for _, tk := range f.landable(ctx, mergeableStates...) {
		if f.halted(ctx) || f.paused() {
			return
		}
		if tk.PullRequest == nil || tk.PullRequest.State != store.PRMerged {
			continue
		}
		if tk.State == store.InReview {
			f.landMerged(ctx, tk)
			continue
		}
		key := tk.ID + "@" + tk.PullRequest.MergeCommit
		if f.warnedMerged[key] {
			continue
		}
		f.warnedMerged[key] = true
		f.printf("warning: %s is %s but its pull request %d is merged: only an In Review or Accepted Ticket lands", tk.ID, tk.State, tk.PullRequest.Number)
		body := fmt.Sprintf("Its pull request %d was merged on the Code Host, but this Ticket is %s, so it doesn't land: only an In Review or Accepted Ticket lands.", tk.PullRequest.Number, tk.State)
		if _, err := f.client.Comment(ctx, f.project, core.CommentInput{Ticket: core.Ref(tk.ID), Body: body}); err != nil && ctx.Err() == nil {
			f.printf("%s: comment: %v", tk.ID, err)
		}
	}
}

// pushedAlready reports whether branch is on remote, as far as the last
// fetch or push knows.
func (f *factory) pushedAlready(remote, branch string) bool {
	_, err := f.repository.commit("refs/remotes/" + remote + "/" + branch)
	return err == nil
}
