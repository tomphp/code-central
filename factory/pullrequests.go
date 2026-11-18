package factory

import (
	"context"
	"fmt"
	"slices"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Pull requests across Repositories (Spec code-central-848, ADR 0050,
// Spec code-central-890): a Ticket whose work is in Repositories beside
// the Home one has a pull request in each of its Changed Repositories
// under Pull Request, and in no other, into that Repository's own Base.
// The Factory pushes its branch in each, and lands each Repository at its
// pull request's merge commit once that Repository's fetched base holds
// it, in whatever order the human merges them; the Ticket is Done once
// every one of its Changed Repositories has landed, each by its own
// Landing. A Ticket whose work is in the Home Repository alone lands as
// it always did.

// changedElsewhere reports whether tk's work is in a Repository beside
// the Home Repository, home.
func changedElsewhere(home string, tk core.TicketView) bool {
	return slices.ContainsFunc(tk.Repositories, func(r core.ChangedRepositoryView) bool { return r.Name != home })
}

// hasMerged reports whether pr is a pull request that has merged.
func hasMerged(pr *core.PullRequestView) bool { return pr != nil && pr.State == store.PRMerged }

// repositoryNamed is the Repository name with a clone, or nil.
func (l *lander) repositoryNamed(name string) *repository {
	for _, r := range l.repos.ready() {
		if r.name == name {
			return r
		}
	}
	return nil
}

// baseHolds reports whether r's base, kept level with the remote's this
// poll, holds commit; for the Home Repository, a base it named earlier
// in this run may hold it instead.
func (l *lander) baseHolds(r *repository, commit string) bool {
	return l.stateOf(r).current && (l.repos.inBaseOf(r, commit) || r.home && l.inOldBase(commit))
}

// landPullRequestIn sees to an Accepted Ticket whose work spans
// Repositories in r, one of its Changed Repositories under Pull Request:
// not yet landed there, it lands at its pull request's merge commit once
// r's base holds it, or at once with nothing to merge; otherwise its
// branch is pushed to r's Push remote for the Watcher to open a pull
// request on. A Ticket still waiting on its Spec Branch, onSpec, has that
// pushed instead, by landSpecs, so only its merged pull request is seen
// to here.
func (l *lander) landPullRequestIn(ctx context.Context, tk core.TicketView, policies core.LandingPolicies, r *repository, onSpec bool) {
	rec, ok := viewIn(tk, r)
	if !ok || rec.Landed != nil {
		return
	}
	if hasMerged(rec.PullRequest) {
		l.landMergedIn(ctx, tk, r, rec.PullRequest)
		return
	}
	remote := r.remote
	branch := rec.Branch
	if branch == "" || onSpec {
		return
	}
	head, err := r.git.commit(rec.Commit)
	if err != nil {
		l.cantPushYet(ctx, tk, fmt.Sprintf("its work in %s, %s, isn't in that Repository.", r.name, rec.Commit))
		return
	}
	if rec.PullRequest == nil && !l.pushedAlreadyIn(r, branch) && l.baseHolds(r, head) {
		l.printf("%s: nothing to merge into %s%s", tk.ID, l.repos.baseOf(r), r.where())
		l.landIn(ctx, tk, r, head)
		return
	}
	pushed, why := l.push(ctx, r, remote, branch, head)
	if why != "" {
		l.cantPushYet(ctx, tk, fmt.Sprintf("in %s, %s", r.name, why))
		return
	}
	if pushed {
		l.printf("%s: pushed %s to %s%s", tk.ID, branch, remote, r.where())
	}
	l.reportPushed(ctx, tk, policies, r, branch, l.remoteTip(r, remote, branch, head))
}

// landMergedIn lands tk in r, whose pull request there is reported
// merged, at the merge commit, once r's base holds it. Until then it
// waits for a later poll.
func (l *lander) landMergedIn(ctx context.Context, tk core.TicketView, r *repository, pr *core.PullRequestView) {
	if !l.baseHolds(r, pr.MergeCommit) {
		key := tk.ID + "@" + pr.MergeCommit
		if !l.leftOnBranch[key] {
			l.leftOnBranch[key] = true
			l.printf("%s: its pull request%s is merged at %s, which %s doesn't hold yet: waiting for it", tk.ID, r.where(), short(pr.MergeCommit), l.repos.baseOf(r))
		}
		return
	}
	l.printf("%s: pull request %d merged%s", tk.ID, pr.Number, r.where())
	l.landIn(ctx, tk, r, pr.MergeCommit)
}

// landIn calls land for tk in r with the commit that put its work into
// r's base, naming r so the Ticket is Done only once every one of its
// Changed Repositories has landed.
func (l *lander) landIn(ctx context.Context, tk core.TicketView, r *repository, commit string) {
	in := core.LandInput{Ticket: core.Ref(tk.ID), Commit: commit, Repository: r.name}
	if _, err := l.client.Land(ctx, l.project, in); err != nil {
		if ctx.Err() == nil {
			l.printf("%s: land%s: %v", tk.ID, r.where(), err)
		}
		return
	}
	l.printf("%s: landed on %s%s at %s", tk.ID, l.repos.baseOf(r), r.where(), short(commit))
}

// pushedAlreadyIn reports whether branch is on r's Push remote, as far
// as the last fetch or push knows.
func (l *lander) pushedAlreadyIn(r *repository, branch string) bool {
	_, err := r.git.commit("refs/remotes/" + r.remote + "/" + branch)
	return err == nil
}

// detectMergedPullRequestsIn lands an In Review Ticket whose work spans
// Repositories in each Repository under Pull Request, its base not
// broken (held) under policies, whose pull request is merged, as
// detectMergedPullRequests does for one in the Home Repository alone.
func (l *lander) detectMergedPullRequestsIn(ctx context.Context, tk core.TicketView, policies core.LandingPolicies) {
	for _, rec := range tk.Repositories {
		if rec.Landed != nil || !hasMerged(rec.PullRequest) {
			continue
		}
		if r := l.repositoryNamed(rec.Name); r != nil && r.policy(policies).LandsByPullRequest() && l.held(r, policies) == "" {
			l.landMergedIn(ctx, tk, r, rec.PullRequest)
		}
	}
}
