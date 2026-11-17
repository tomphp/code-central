package factory

import (
	"context"
	"fmt"
	"slices"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Pull requests across Repositories (Spec code-central-848, ADR 0050):
// under Pull Request a Ticket whose work is in Repositories beside the
// Home one has a pull request in each Changed Repository, into that
// Repository's own Base. The Factory pushes its branch in each, and lands
// each Repository at its pull request's merge commit once that
// Repository's fetched base holds it, in whatever order the human merges
// them; the Ticket is Done once every one has landed. A Ticket whose work
// is in the Home Repository alone lands as it always did.

// changedElsewhere reports whether tk's work is in a Repository beside
// the Home Repository, home.
func changedElsewhere(home string, tk core.TicketView) bool {
	return slices.ContainsFunc(tk.Repositories, func(r core.ChangedRepositoryView) bool { return r.Name != home })
}

// hasMerged reports whether pr is a pull request that has merged.
func hasMerged(pr *core.PullRequestView) bool { return pr != nil && pr.State == store.PRMerged }

// repositoryNamed is the Repository name with a clone, or nil.
func (f *factory) repositoryNamed(name string) *repository {
	for _, r := range f.readyRepositories() {
		if r.name == name {
			return r
		}
	}
	return nil
}

// baseHolds reports whether r's base, kept level with the remote's this
// poll, holds commit; for the Home Repository, a base it named earlier
// in this run may hold it instead.
func (f *factory) baseHolds(r *repository, commit string) bool {
	return f.stateOf(r).current && (f.inBaseOf(r, commit) || r.home && f.inOldBase(commit))
}

// landPullRequests sees to an Accepted Ticket whose work spans
// Repositories in ws, those of its Changed Repositories under Pull
// Request, in landing order: each not yet landed lands at its pull
// request's merge commit once its base holds it, or at once with nothing
// to merge; the rest have their branch pushed there for the Watcher to
// open a pull request on. A Ticket still waiting on its Spec Branch has
// that pushed instead, by pushSpecs, so only its merged pull requests are
// seen to here. Each is pushed to its own Push remote.
func (f *factory) landPullRequests(ctx context.Context, tk core.TicketView, p core.ProjectView, ws []changedWork) {
	onSpec := f.waitsOnSpecBranch(tk)
	for _, w := range ws {
		r := w.r
		rec, ok := viewIn(tk, r)
		if !ok || rec.Landed != nil {
			continue
		}
		if pr := rec.PullRequest; pr != nil && pr.State == store.PRMerged {
			f.landMergedIn(ctx, tk, r, pr)
			continue
		}
		remote := r.remote
		branch := rec.Branch
		if branch == "" || onSpec {
			continue
		}
		head, err := r.git.commit(rec.Commit)
		if err != nil {
			f.cantPushYet(ctx, tk, fmt.Sprintf("its work in %s, %s, isn't in that Repository.", r.name, rec.Commit))
			continue
		}
		if rec.PullRequest == nil && !f.pushedAlreadyIn(r, branch) && f.baseHolds(r, head) {
			f.printf("%s: nothing to merge into %s%s", tk.ID, f.baseOf(r), r.where())
			f.landIn(ctx, tk, r, head)
			continue
		}
		pushed, why := f.push(ctx, r, remote, branch, head)
		if why != "" {
			f.cantPushYet(ctx, tk, fmt.Sprintf("in %s, %s", r.name, why))
			continue
		}
		if pushed {
			f.printf("%s: pushed %s to %s%s", tk.ID, branch, remote, r.where())
		}
		f.reportPushed(ctx, tk, p.LandingPolicies(), r, branch, f.remoteTip(r, remote, branch, head))
	}
}

// landMergedIn lands tk in r, whose pull request there is reported
// merged, at the merge commit, once r's base holds it. Until then it
// waits for a later poll.
func (f *factory) landMergedIn(ctx context.Context, tk core.TicketView, r *repository, pr *core.PullRequestView) {
	if !f.baseHolds(r, pr.MergeCommit) {
		key := tk.ID + "@" + pr.MergeCommit
		if !f.leftOnBranch[key] {
			f.leftOnBranch[key] = true
			f.printf("%s: its pull request%s is merged at %s, which %s doesn't hold yet: waiting for it", tk.ID, r.where(), short(pr.MergeCommit), f.baseOf(r))
		}
		return
	}
	f.printf("%s: pull request %d merged%s", tk.ID, pr.Number, r.where())
	f.landIn(ctx, tk, r, pr.MergeCommit)
}

// landIn calls land for tk in r with the commit that put its work into
// r's base, naming r so the Ticket is Done only once every one of its
// Changed Repositories has landed.
func (f *factory) landIn(ctx context.Context, tk core.TicketView, r *repository, commit string) {
	in := core.LandInput{Ticket: core.Ref(tk.ID), Commit: commit, Repository: r.name}
	if _, err := f.client.Land(ctx, f.project, in); err != nil {
		if ctx.Err() == nil {
			f.printf("%s: land%s: %v", tk.ID, r.where(), err)
		}
		return
	}
	f.printf("%s: landed on %s%s at %s", tk.ID, f.baseOf(r), r.where(), short(commit))
}

// pushedAlreadyIn reports whether branch is on r's Push remote, as far
// as the last fetch or push knows.
func (f *factory) pushedAlreadyIn(r *repository, branch string) bool {
	_, err := r.git.commit("refs/remotes/" + r.remote + "/" + branch)
	return err == nil
}

// detectMergedPullRequestsIn lands an In Review Ticket whose work spans
// Repositories in each Repository under Pull Request, its base not
// broken (held) under policies, whose pull request is merged, as
// detectMergedPullRequests does for one in the Home Repository alone.
func (f *factory) detectMergedPullRequestsIn(ctx context.Context, tk core.TicketView, policies core.LandingPolicies) {
	for _, rec := range tk.Repositories {
		if rec.Landed != nil || !hasMerged(rec.PullRequest) {
			continue
		}
		if r := f.repositoryNamed(rec.Name); r != nil && r.policy(policies).LandsByPullRequest() && f.held(r, policies) == "" {
			f.landMergedIn(ctx, tk, r, rec.PullRequest)
		}
	}
}

// moveLandedBranch moves branch in r, where tk's work has landed, to r's
// Base tip at a Claim's start, before the session starts, so that only
// new work there opens a new pull request: as when tk was sent back after
// its pull request there merged (ADR 0050). It moves it only when its
// worktree, if any, is clean and the branch holds nothing the base
// doesn't besides the work that landed; otherwise it leaves it as it is.
func (f *factory) moveLandedBranch(r *repository, tk core.TicketView, branch, wt string) {
	i := slices.IndexFunc(tk.Repositories, func(v core.ChangedRepositoryView) bool { return v.Name == r.name })
	if i < 0 || tk.Repositories[i].Landed == nil {
		return
	}
	rec := tk.Repositories[i]
	head, err := r.git.commit("refs/heads/" + branch)
	if err != nil || rec.Commit != "" && head != rec.Commit {
		// Gone, or holding work beyond what landed: nothing to move.
		return
	}
	tip := f.baseTipOf(r)
	if tip == "" || head == tip {
		return
	}
	list, err := r.git.worktrees()
	if err != nil {
		return
	}
	if w, ok := find(list, wt); ok && w.branch == branch {
		if status, err := r.git.status(wt); err != nil || status != "" {
			return
		}
		if _, err := r.git.reset(wt, tip, head); err != nil {
			f.printf("warning: can't move %s%s to %s's tip: %v", branch, r.where(), f.baseOf(r), err)
			return
		}
	} else if slices.ContainsFunc(list, func(w gitWorktree) bool { return w.branch == branch }) {
		return
	} else if err := r.git.updateRef("refs/heads/"+branch, tip, head); err != nil {
		f.printf("warning: can't move %s%s to %s's tip: %v", branch, r.where(), f.baseOf(r), err)
		return
	}
	f.printf("%s: its work%s landed, so %s moved to %s's tip, %s", tk.ID, r.where(), branch, f.baseOf(r), short(tip))
}

// baseTipOf is r's Base tip: its Push remote's, as last fetched, when it
// holds the local base, else the local base's; "" when there is neither.
// A local-only Repository's is always the local base's.
func (f *factory) baseTipOf(r *repository) string {
	local, lerr := r.git.commit("refs/heads/" + f.baseOf(r))
	if r.remote != "" && !r.localOnly {
		if theirs, err := r.git.commit("refs/remotes/" + r.remote + "/" + f.baseOf(r)); err == nil && (lerr != nil || r.git.isAncestor(local, theirs)) {
			return theirs
		}
	}
	if lerr != nil {
		return ""
	}
	return local
}
