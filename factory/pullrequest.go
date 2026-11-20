package factory

import (
	"context"
	"fmt"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

// Under the Pull Request Landing Method the Factory pushes each accepted
// Ticket's branch and keeps the local base level with the Push remote's,
// and the Pull Request Watcher opens and follows the pull requests. A
// Ticket lands at the merge commit the Code Host reported once that commit
// is in the fetched base, whatever the merge strategy; git ancestry plays
// no part, and there is no CI gate.

// syncBase fetches remote and fast-forwards the local base to the
// remote's, never merging or forcing. It reports whether the base is
// level with the remote's, or with pushesBase (Merge + Push, where the
// Factory merges locally and then pushes the base), level or ahead of
// it: a local base holding the remote's whole history is current, and one
// that holds commits the remote's doesn't while the remote holds commits
// it doesn't is moved onto the remote's tip (moveBase), with merged the
// Accepted Tickets whose merges it rebuilds. A failed fetch is a warning,
// retried next poll; a local base that can't be brought level (under Pull
// Request, any holding commits the remote's doesn't) is diverged, and
// stops landing and new branches until a later poll finds it fixed. Each
// Repository's base is synced apart, in r, with remote the name of its
// own remote.
func (l *lander) syncBase(ctx context.Context, r *repository, remote string, pushesBase bool, merged []api.TicketView) bool {
	base := l.repos.baseOf(r)
	if out, err := r.git.Fetch(ctx, remote); err != nil {
		if ctx.Err() == nil {
			if r.home {
				l.printf("warning: can't fetch %s, so nothing lands this poll: %s", remote, firstLine(out))
			} else {
				l.printf("warning: can't fetch %s in Repository %s, so nothing touching it lands this poll: %s", remote, r.name, firstLine(out))
			}
		}
		return false
	}
	local, err := r.git.Commit("refs/heads/" + base)
	if err != nil {
		l.printf("warning: the base, %s, isn't a branch%s, so the Factory can't keep it level with %s", base, r.where(), remote)
		return false
	}
	theirs, err := r.git.Commit("refs/remotes/" + remote + "/" + base)
	if err != nil {
		// The remote has no base yet: nothing to catch up with.
		l.stateOf(r).diverged = false
		return true
	}
	switch {
	case local == theirs, pushesBase && r.git.IsAncestor(theirs, local):
		l.noteBase(ctx, r, false, local, theirs)
		return true
	case r.git.IsAncestor(local, theirs):
		l.noteBase(ctx, r, false, local, theirs)
		if why := l.advanceBase(r, local, theirs); why != "" {
			l.printf("warning: can't fast-forward %s%s to %s yet: %s", base, r.where(), remote, firstLine(why))
			return false
		}
		l.printf("%s: fast-forwarded to %s/%s at %s%s", base, remote, base, short(theirs), r.where())
		return true
	case pushesBase:
		return l.moveBase(ctx, r, remote, local, theirs, merged)
	}
	l.noteBase(ctx, r, true, local, theirs)
	return false
}

// noteBase records whether r's base has diverged, with a line when that
// changes, and reports it to Code Central. It reports on every poll that
// reaches a verdict, since Code Central drops the report with the
// Heartbeat: a failed fetch reaches none, so it leaves the last report.
func (l *lander) noteBase(ctx context.Context, r *repository, diverged bool, local, remote string) {
	defer l.reportBase(ctx, r, diverged, local, remote)
	s, base := l.stateOf(r), l.repos.baseOf(r)
	if diverged && !s.diverged {
		if r.home {
			l.printf("warning: the base, %s, has diverged from the remote's: it is at %s and the remote's at %s. Nothing lands and no branch is cut until you reconcile them by hand", base, short(local), short(remote))
		} else {
			l.printf("warning: the base, %s, in Repository %s has diverged from the remote's: it is at %s and the remote's at %s. Nothing touching it lands and no branch is cut until you reconcile them by hand", base, r.name, short(local), short(remote))
		}
	}
	if !diverged && s.diverged {
		l.printf("%s: no longer diverged from the remote's%s", base, r.where())
	}
	s.diverged = diverged
}

// reportBase tells Code Central whether r's base is current or diverged,
// with the local and remote commits. A failed report is a warning once
// until one goes through; the next poll reports again.
func (l *lander) reportBase(ctx context.Context, r *repository, diverged bool, local, remote string) {
	state := api.BaseCurrent
	if diverged {
		state = api.BaseDiverged
	}
	l.reportBaseState(ctx, r, api.ReportBaseInput{State: state, Local: local, Remote: remote})
}

// reportBaseState sends one report_base, of r's base: the Home
// Repository's names none, as a Factory from before Repositories did.
func (l *lander) reportBaseState(ctx context.Context, r *repository, in api.ReportBaseInput) {
	if !r.home {
		in.Repository = r.name
	}
	ctx, cancel := context.WithTimeout(ctx, postTimeout)
	defer cancel()
	_, err := l.client.ReportBase(ctx, l.project, in)
	if err != nil && ctx.Err() == nil && !l.reportBaseFailing {
		l.printf("warning: can't report the base to Code Central: %v", err)
	}
	l.reportBaseFailing = err != nil
}

// landPullRequest sees to an Accepted Ticket whose work is in the Home
// Repository alone, under Pull Request there: one with nothing to merge
// lands at once, one whose pull request is merged lands at the merge
// commit, and the rest have their branch pushed for the Watcher to open a
// pull request on. Under Rebase its branch holds its work rebased onto
// the base, when it was (rebaseForPush), which is pushed with lease over
// an older push.
func (l *lander) landPullRequest(ctx context.Context, tk api.TicketView, p api.ProjectView) {
	if hasMerged(tk.PullRequest) {
		l.landMerged(ctx, tk)
		return
	}
	remote := l.repos.home.remote
	head, _, err := l.workIn(l.repos.home, tk)
	if err == nil && tk.PullRequest == nil && !l.pushedAlreadyIn(l.repos.home, tk.Branch) && l.stateOf(l.repos.home).current && l.inBase(head) {
		l.printf("%s: nothing to merge into %s", tk.ID, l.repos.baseBranch())
		l.landAt(ctx, tk, l.repos.home, head)
		return
	}
	if err != nil {
		l.cantPushYet(ctx, tk, "its work isn't in this repository.")
		return
	}
	if tk.Branch == "" {
		return
	}
	policies := p.LandingPolicies()
	pushed, why := l.push(ctx, l.repos.home, remote, tk.Branch, head, l.repos.home.policy(policies).RebaseInForce())
	if why != "" {
		l.cantPushYet(ctx, tk, why)
		return
	}
	if pushed {
		l.printf("%s: pushed %s to %s", tk.ID, tk.Branch, remote)
	}
	l.reportPushed(ctx, tk, policies, l.repos.home, tk.Branch, l.remoteTip(l.repos.home, remote, tk.Branch, head))
}

// inOldBase reports whether the Home's fetched Push remote holds commit
// on a base the Project named earlier in this run: an open pull request
// keeps its target when the base changes, so it merges there.
func (l *lander) inOldBase(commit string) bool {
	for _, b := range l.oldBases {
		if l.repos.home.git.IsAncestor(commit, "refs/remotes/"+l.repos.home.remote+"/"+b) {
			return true
		}
	}
	return false
}

// landMerged lands tk, whose pull request is reported merged, at the merge
// commit, once the fetched base holds it. Until then it waits for a later
// poll.
func (l *lander) landMerged(ctx context.Context, tk api.TicketView) {
	merge := tk.PullRequest.MergeCommit
	if !l.stateOf(l.repos.home).current || !l.inBase(merge) && !l.inOldBase(merge) {
		key := tk.ID + "@" + merge
		if !l.leftOnBranch[key] {
			l.leftOnBranch[key] = true
			l.printf("%s: its pull request is merged at %s, which %s doesn't hold yet: waiting for it", tk.ID, short(merge), l.repos.baseBranch())
		}
		return
	}
	l.printf("%s: pull request %d merged", tk.ID, tk.PullRequest.Number)
	l.landAt(ctx, tk, l.repos.home, merge)
}

// detectMergedPullRequests lands tk, In Review, in each of its Changed
// Repositories under Pull Request whose pull request there is merged, as
// a branch merged by hand does under Branch; each is held back only by
// its own broken base. A Ticket in any other State whose pull request in
// the Home Repository is merged gets a warning line and a comment, once.
func (l *lander) detectMergedPullRequests(ctx context.Context, tk api.TicketView, policies api.LandingPolicies) {
	if tk.State == domain.InReview && changedElsewhere(l.repos.home.name, tk) {
		l.detectMergedPullRequestsIn(ctx, tk, policies)
		return
	}
	if !l.repos.home.policy(policies).LandsByPullRequest() || l.held(l.repos.home, policies) != "" || !hasMerged(tk.PullRequest) {
		return
	}
	if tk.State == domain.InReview {
		l.landMerged(ctx, tk)
		return
	}
	key := tk.ID + "@" + tk.PullRequest.MergeCommit
	if l.warnedMerged[key] {
		return
	}
	l.warnedMerged[key] = true
	l.printf("warning: %s is %s but its pull request %d is merged: only an In Review or Accepted Ticket lands", tk.ID, tk.State, tk.PullRequest.Number)
	body := fmt.Sprintf("Its pull request %d was merged on the Code Host, but this Ticket is %s, so it doesn't land: only an In Review or Accepted Ticket lands.", tk.PullRequest.Number, tk.State)
	if _, err := l.client.Comment(ctx, l.project, api.CommentInput{Ticket: api.Ref(tk.ID), Body: body}); err != nil && ctx.Err() == nil {
		l.printf("%s: comment: %v", tk.ID, err)
	}
}
