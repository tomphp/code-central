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

// repositoryNamed is the Repository name with a clone, or nil.
func (f *factory) repositoryNamed(name string) *repository {
	for _, r := range f.readyRepositories() {
		if r.name == name {
			return r
		}
	}
	return nil
}

// fetchElsewhere fetches remote in every Repository with a clone but the
// Home one, which syncBase fetches, so their remote bases are current,
// and fast-forwards each local base that is behind its remote's. A
// failed fetch is a warning, and that Repository lands nothing this
// poll.
func (f *factory) fetchElsewhere(ctx context.Context, remote string) {
	f.fetchedElsewhere = map[string]bool{}
	for _, r := range f.readyRepositories() {
		if r.home {
			continue
		}
		if out, err := r.git.fetch(ctx, remote); err != nil {
			if ctx.Err() == nil {
				f.printf("warning: can't fetch %s%s, so nothing lands there this poll: %s", remote, r.where(), firstLine(out))
			}
			continue
		}
		f.fetchedElsewhere[r.name] = true
		f.fastForwardBase(r, remote)
	}
}

// fastForwardBase moves r's local base to its remote's when the remote's
// holds it, never merging or forcing: in the worktree where it is checked
// out, or by ref. One that can't move stays as it is, retried next poll.
func (f *factory) fastForwardBase(r *repository, remote string) {
	base := f.baseOf(r)
	local, lerr := r.git.commit("refs/heads/" + base)
	theirs, terr := r.git.commit("refs/remotes/" + remote + "/" + base)
	if lerr != nil || terr != nil || local == theirs || !r.git.isAncestor(local, theirs) {
		return
	}
	list, err := r.git.worktrees()
	if err != nil {
		return
	}
	if i := slices.IndexFunc(list, func(w gitWorktree) bool { return w.branch == base }); i >= 0 {
		_, err = r.git.fastForward(list[i].path, theirs)
	} else {
		err = r.git.updateRef("refs/heads/"+base, theirs, local)
	}
	if err != nil {
		return
	}
	f.printf("%s%s: fast-forwarded to %s/%s at %s", base, r.where(), remote, base, short(theirs))
}

// baseHolds reports whether r's base holds commit: for the Home
// Repository, its base kept level with the remote's; for another, its
// local base or the remote's as this poll fetched it.
func (f *factory) baseHolds(r *repository, remote, commit string) bool {
	if r.home {
		return f.baseCurrent && (f.inBase(commit) || f.inOldBase(commit, remote))
	}
	return f.inBaseOf(r, commit) || f.fetchedElsewhere[r.name] && r.git.isAncestor(commit, "refs/remotes/"+remote+"/"+f.baseOf(r))
}

// landPullRequests sees to an Accepted Ticket under Pull Request whose
// work spans Repositories, in landing order: each Changed Repository not
// yet landed lands at its pull request's merge commit once its base
// holds it, or at once with nothing to merge; the rest have their branch
// pushed there for the Watcher to open a pull request on.
func (f *factory) landPullRequests(ctx context.Context, tk core.TicketView, p core.ProjectView) {
	remote := p.Landing.Remote
	for _, rec := range tk.Repositories {
		if rec.Landed != nil {
			continue
		}
		r := f.repositoryNamed(rec.Name)
		if r == nil {
			// Repository missing says why.
			continue
		}
		if pr := rec.PullRequest; pr != nil && pr.State == store.PRMerged {
			f.landMergedIn(ctx, tk, r, pr, remote)
			continue
		}
		branch := rec.Branch
		if branch == "" {
			continue
		}
		head, err := r.git.commit(rec.Commit)
		if err != nil {
			f.cantPushYet(ctx, tk, fmt.Sprintf("its work in %s, %s, isn't in that Repository.", r.name, rec.Commit))
			continue
		}
		if rec.PullRequest == nil && !f.pushedAlreadyIn(r, remote, branch) && f.baseHolds(r, remote, head) {
			f.printf("%s: nothing to merge into %s%s", tk.ID, f.baseOf(r), r.where())
			f.landIn(ctx, tk, r, head)
			continue
		}
		pushed, why := f.pushIn(ctx, r, remote, branch, head)
		if why != "" {
			f.cantPushYet(ctx, tk, fmt.Sprintf("in %s, %s", r.name, why))
			continue
		}
		if pushed {
			f.printf("%s: pushed %s to %s%s", tk.ID, branch, remote, r.where())
		}
		f.reportPushedIn(ctx, tk, p.LandingPolicy(), r, rec, branch, f.remoteTipIn(r, remote, branch, head))
	}
}

// landMergedIn lands tk in r, whose pull request there is reported
// merged, at the merge commit, once r's base holds it. Until then it
// waits for a later poll.
func (f *factory) landMergedIn(ctx context.Context, tk core.TicketView, r *repository, pr *core.PullRequestView, remote string) {
	if !f.baseHolds(r, remote, pr.MergeCommit) {
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

// pushedAlreadyIn reports whether branch is on remote in r, as far as
// the last fetch or push knows.
func (f *factory) pushedAlreadyIn(r *repository, remote, branch string) bool {
	_, err := r.git.commit("refs/remotes/" + remote + "/" + branch)
	return err == nil
}

// reportPushedIn is reportPushed for tk's work in r, rec: reported once,
// when it isn't the commit last reported pushed there.
func (f *factory) reportPushedIn(ctx context.Context, tk core.TicketView, policy core.LandingPolicy, r *repository, rec core.ChangedRepositoryView, branch, commit string) {
	if !policy.RecordsPushedCommit() || rec.PushedCommit != nil && rec.PushedCommit.Commit == commit {
		return
	}
	in := core.ReportPushedCommitInput{Ticket: core.Ref(tk.ID), Commit: commit, Branch: branch}
	if !r.home {
		in.Repository = r.name
	}
	if _, err := f.client.ReportPushedCommit(ctx, f.project, in); err != nil && ctx.Err() == nil {
		f.printf("%s: report_pushed_commit%s: %v", tk.ID, r.where(), err)
	}
}

// detectMergedPullRequestsIn lands an In Review Ticket whose work spans
// Repositories in each Repository whose pull request is merged, as
// detectMergedPullRequests does for one in the Home Repository alone.
func (f *factory) detectMergedPullRequestsIn(ctx context.Context, tk core.TicketView, remote string) {
	for _, rec := range tk.Repositories {
		if rec.Landed != nil || rec.PullRequest == nil || rec.PullRequest.State != store.PRMerged {
			continue
		}
		if r := f.repositoryNamed(rec.Name); r != nil {
			f.landMergedIn(ctx, tk, r, rec.PullRequest, remote)
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

// baseTipOf is r's Base tip: the remote's, as last fetched, when it holds
// the local base, else the local base's; "" when there is neither.
func (f *factory) baseTipOf(r *repository) string {
	local, lerr := r.git.commit("refs/heads/" + f.baseOf(r))
	if f.landingRemote != "" {
		if theirs, err := r.git.commit("refs/remotes/" + f.landingRemote + "/" + f.baseOf(r)); err == nil && (lerr != nil || r.git.isAncestor(local, theirs)) {
			return theirs
		}
	}
	if lerr != nil {
		return ""
	}
	return local
}
