package factory

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// worktree is ticket's Home worktree, where its sessions run.
func (f *factory) worktree(ticket string) string { return filepath.Join(f.home.worktrees, ticket) }

// repositoryWorktree is a Claim's or a Conversation's worktree in one
// Repository (ADR 0048): its path, beside the Repository's clone, and the
// Base and branch it is on, where an empty branch is detached at the
// Base's tip.
type repositoryWorktree struct {
	r      *repository
	path   string
	base   string
	branch string
}

// label names w's Repository in a list of them, marking the Home
// Repository.
func (w repositoryWorktree) label() string {
	if w.r.home {
		return w.r.name + ", the Home Repository"
	}
	return w.r.name
}

// worktreeSet is a Claim's or a Conversation's worktrees, one in every
// Repository with a clone, in landing order, the Home worktree among
// them.
type worktreeSet []repositoryWorktree

// worktreesNamed is the worktree folder named name (a Ticket ID, or the
// Chat worktree's) on branch in every Repository with a clone.
func (f *factory) worktreesNamed(name, branch string) worktreeSet {
	var ws worktreeSet
	for _, r := range f.readyRepositories() {
		ws = append(ws, repositoryWorktree{r: r, path: filepath.Join(r.worktrees, name), base: f.baseOf(r), branch: branch})
	}
	return ws
}

// home is the Home worktree among ws, where the session starts.
func (ws worktreeSet) home() repositoryWorktree {
	for _, w := range ws {
		if w.r.home {
			return w
		}
	}
	panic("factory: no Home worktree")
}

// addDirs is the paths of ws but the Home worktree's: the folders the
// session reaches with --add-dir.
func (ws worktreeSet) addDirs() []string {
	var dirs []string
	for _, w := range ws {
		if !w.r.home {
			dirs = append(dirs, w.path)
		}
	}
	return dirs
}

// several reports whether ws spans more than one Repository, which the
// session is then told of.
func (ws worktreeSet) several() bool { return len(ws) > 1 }

// about is what, a sentence about w, one of ws, as said to the human:
// opening with w's Repository when there are several.
func (ws worktreeSet) about(w repositoryWorktree, what string) string {
	if !ws.several() || what == "" {
		return what
	}
	return "In Repository " + w.r.name + ", " + strings.ToLower(what[:1]) + what[1:]
}

// setUpWorktrees gives the claimed Ticket tk its worktree in every
// Repository ws names: on its work branch, or detached at the Base's tip
// for a Stage. A branch in a Repository where tk's work has landed moves
// to the Base's tip first. The first that fails is the Claim's failure,
// naming its Repository when there are several; those set up before it
// stay.
func (f *factory) setUpWorktrees(tk core.TicketView, ws worktreeSet) *failure {
	for _, w := range ws {
		var err error
		if w.branch != "" {
			f.moveLandedBranch(w.r, tk, w.branch, w.path)
			err = f.cutBranchWorktree(w.r, tk, w.branch, w.path)
		} else {
			err = f.stageWorktree(w.r, w.path)
		}
		if err == nil {
			continue
		}
		fl := (*failure)(nil)
		if !errors.As(err, &fl) {
			fl = f.setUpFailed(w.path, err)
		}
		fl.report.WhatBroke = ws.about(w, fl.report.WhatBroke)
		return fl
	}
	return nil
}

// gitWorktree is one entry of `git worktree list`.
type gitWorktree struct {
	path   string
	branch string // "" when detached
}

// find is the worktree at path.
func find(list []gitWorktree, path string) (gitWorktree, bool) {
	for _, w := range list {
		if w.path == path {
			return w, true
		}
	}
	return gitWorktree{}, false
}

// branchWorktree gives a Ticket its worktree at wt in r on branch, its
// workBranch: reused when it is there, else added on the existing
// branch, else on a new branch from from: the base's tip, or a Spec
// Branch's. Anything else at wt,
// or branch checked out elsewhere, is a failure, and nothing is
// touched.
func (f *factory) branchWorktree(r *repository, branch, wt, from string) error {
	list, err := r.git.worktrees()
	if err != nil {
		return err
	}
	if w, ok := find(list, wt); ok {
		if w.branch == branch {
			return nil
		}
		return f.wrongWorktree(wt, w, "on branch "+branch)
	}
	if _, err := os.Stat(wt); err == nil {
		return f.strayDirectory(r, wt)
	}
	if _, err := r.git.commit("refs/heads/" + branch); err == nil {
		for _, w := range list {
			if w.branch == branch {
				return f.branchElsewhere(branch, wt, w.path)
			}
		}
		_, err := f.gitOut(r.dir, "worktree", "add", "-q", wt, branch)
		return err
	}
	_, err = f.gitOut(r.dir, "worktree", "add", "-q", "-b", branch, wt, from)
	return err
}

// stageWorktree gives a Stage Ticket its worktree at wt in r: detached at
// the base's tip, added when it isn't there and moved to the current tip
// when it is. A dirty one is left as it is, as a failure: the Stage wrote
// files it shouldn't have. Anything at wt but a detached worktree is a
// failure too, and nothing is touched.
func (f *factory) stageWorktree(r *repository, wt string) error {
	left, err := f.detachAtBase(r, wt)
	switch {
	case err != nil || left == nil:
		return err
	case left.stray:
		return f.strayDirectory(r, wt)
	case left.branch != "":
		return f.wrongWorktree(wt, gitWorktree{wt, left.branch}, "detached")
	}
	head, _ := f.gitOut(wt, "rev-parse", "--short", "HEAD")
	return &failure{core.ReportFailureInput{
		WhatBroke:       fmt.Sprintf("The Stage's worktree is dirty, so the Factory can't start the Stage. Changed files:\n\n%s", left.dirty),
		Why:             fmt.Sprintf("A Stage writes nothing to the repository, but %s has uncommitted changes, most likely written by an earlier Claim's session. The Factory moves a Stage's worktree to %s's tip at each new Claim only when it is clean.", wt, f.baseOf(r)),
		WhatToFix:       fmt.Sprintf("Look at the changes in %s and keep anything worth keeping elsewhere (a Stage's output belongs in Code Central). Then clean the worktree and send the Ticket back.", wt),
		WhereWorkStands: fmt.Sprintf("No session ran under this Claim. The worktree %s is detached at %s, with the changes above left as they were.", wt, head),
	}}
}

// leftAsItWas is why detachAtBase left what is at a worktree path as it
// was: something that isn't a worktree is there (stray), the worktree is
// on a branch, or it is dirty, with its changed files.
type leftAsItWas struct {
	stray  bool
	branch string
	dirty  string
}

// detachAtBase makes wt a worktree of r detached at its base's tip: added
// when it isn't there, and moved to the current tip when it is a clean
// detached one. Anything else is left as it was, and returned saying why.
func (f *factory) detachAtBase(r *repository, wt string) (*leftAsItWas, error) {
	list, err := r.git.worktrees()
	if err != nil {
		return nil, err
	}
	w, ok := find(list, wt)
	if !ok {
		if _, err := os.Stat(wt); err == nil {
			return &leftAsItWas{stray: true}, nil
		}
		_, err := f.gitOut(r.dir, "worktree", "add", "-q", "--detach", wt, f.baseOf(r))
		return nil, err
	}
	if w.branch != "" {
		return &leftAsItWas{branch: w.branch}, nil
	}
	status, err := r.git.status(wt)
	if err != nil {
		return nil, err
	}
	if status != "" {
		return &leftAsItWas{dirty: status}, nil
	}
	_, err = f.gitOut(wt, "checkout", "-q", "--detach", f.baseOf(r))
	return nil, err
}

// prune runs `git worktree prune` in r, dropping worktrees whose folders
// are gone.
func (f *factory) prune(r *repository) {
	if _, err := f.gitOut(r.dir, "worktree", "prune"); err != nil {
		f.printf("warning: %v", err)
	}
}

// sweep looks, in every Repository with a clone, at each worktree `git
// worktree list` reports in its worktree folder. One whose Ticket is Done
// or Cancelled, or Accepted where the Landing Policy doesn't keep it
// (under Branch, so the human can check its branch out), or Accepted with
// its Spec merge recorded under Land per Spec, is removed with `git
// worktree remove` when clean, and left with a warning line when dirty.
// With several Repositories, the Ticket's branch goes with its worktree
// when the Ticket never used it (sweepsBranch, sweepUnusedBranch); with
// the Home Repository alone, branches are never deleted. Anything else is
// kept, the Chat worktrees among it.
func (f *factory) sweep(ctx context.Context, policies core.LandingPolicies) {
	tickets := map[string]*core.TicketView{}
	for _, r := range f.readyRepositories() {
		if ctx.Err() != nil {
			return
		}
		f.sweepIn(ctx, r, policies, tickets)
	}
}

// sweepIn is sweep's work in r. tickets holds each Ticket the sweep has
// read, by ID, nil for one it couldn't, so each is read once.
func (f *factory) sweepIn(ctx context.Context, r *repository, policies core.LandingPolicies, tickets map[string]*core.TicketView) {
	policy := r.policy(policies)
	list, err := r.git.worktrees()
	if err != nil {
		f.printf("warning: can't list the worktrees%s: %v", r.where(), err)
		return
	}
	for _, w := range list {
		if ctx.Err() != nil {
			return
		}
		if filepath.Dir(w.path) != r.worktrees {
			continue
		}
		id := filepath.Base(w.path)
		// The Chat worktree is no Ticket's, and stays so that a copied
		// Resume Command keeps working.
		if id == chatWorktreeName || f.turnRunningOn(id) {
			continue
		}
		tk, ok := tickets[id]
		if !ok {
			tk = f.sweptTicket(ctx, id)
			tickets[id] = tk
		}
		if tk == nil || !doneWithWorktree(*tk, policy) {
			continue
		}
		status, err := r.git.status(w.path)
		if err != nil {
			f.printf("warning: %s: can't read the worktree %s: %v", id, w.path, err)
			continue
		}
		if status != "" {
			if !f.warned[w.path] {
				f.warned[w.path] = true
				f.printf("warning: %s is %s but its worktree %s has uncommitted changes: left in place", id, tk.State, w.path)
			}
			continue
		}
		if err := r.git.removeWorktree(w.path); err != nil {
			f.printf("warning: %s: can't remove the worktree %s: %v", id, w.path, err)
			continue
		}
		delete(f.warned, w.path)
		f.printf("%s: %s: removed its worktree %s", id, tk.State, w.path)
		if f.sweepsBranch(r, *tk, w.branch) {
			f.sweepUnusedBranch(r, *tk, w.branch)
		}
	}
}

// sweepsBranch reports whether the sweep may delete tk's branch in r,
// once its worktree there is gone, if tk never used it. In a Repository
// other than the Home Repository it may: nothing reads a branch that
// holds no work. In the Home Repository it may only with several
// Repositories, as a Project with the Home Repository alone never has,
// and only once tk is Done or Cancelled or names another branch, since
// landing reads the branch a Ticket names.
func (f *factory) sweepsBranch(r *repository, tk core.TicketView, branch string) bool {
	switch {
	case !r.home:
		return true
	case !f.several():
		return false
	}
	return tk.State == store.Done || tk.State == store.Cancelled || tk.Branch != branch
}

// sweptTicket is the Ticket id for the sweep, or nil when it can't be
// read: a worktree named like no Ticket is no Ticket's.
func (f *factory) sweptTicket(ctx context.Context, id string) *core.TicketView {
	tk, err := f.client.GetTicket(ctx, f.project, core.Ref(id))
	if err != nil {
		var ce *core.Error
		if ctx.Err() == nil && !(errors.As(err, &ce) && ce.Code == core.NotFound) {
			f.printf("%s: get_ticket: %v", id, err)
		}
		return nil
	}
	return &tk
}

// sweepUnusedBranch deletes branch, tk's in r, when tk never used it: its
// tip is still on the first-parent line of r's Base, or of the Spec
// Branch of tk's Spec, where it was cut, rather than merged into either or
// beyond them. A branch with work of its own is kept.
func (f *factory) sweepUnusedBranch(r *repository, tk core.TicketView, branch string) {
	if branch == "" {
		return
	}
	ref := "refs/heads/" + branch
	tip, err := r.git.commit(ref)
	if err != nil {
		return
	}
	if !onFirstParentLine(r.git, tip, "refs/heads/"+f.baseOf(r)) && !onFirstParentLine(r.git, tip, "refs/heads/"+specBranch(tk.Spec.ID)) {
		return
	}
	if err := r.git.deleteRef(ref, tip); err != nil {
		f.printf("warning: %s: can't delete its unused branch %s%s: %v", tk.ID, branch, r.where(), err)
		return
	}
	f.printf("%s: deleted its unused branch %s%s", tk.ID, branch, r.where())
}

// onFirstParentLine reports whether commit is rev's tip or one of the
// commits its first parents lead back through in g: rev's own line, not
// a branch merged into it.
func onFirstParentLine(g repo, commit, rev string) bool {
	tip, err := g.commit(rev)
	if err != nil {
		return false
	}
	if tip == commit {
		return true
	}
	if !g.isAncestor(commit, tip) {
		return false
	}
	after, err := g.firstParents(tip, commit)
	if err != nil || len(after) == 0 {
		return false
	}
	parents, err := g.parentsOf(after[0])
	return err == nil && len(parents) > 0 && parents[0] == commit
}

// doneWithWorktree reports whether the Factory is done with the worktree
// of tk, under the Landing Policy of the worktree's Repository.
func doneWithWorktree(tk core.TicketView, policy core.LandingPolicy) bool {
	switch tk.State {
	case store.Done, store.Cancelled:
		return true
	case store.Accepted:
		// Under Land per Spec its work is safe in the Spec Branch once
		// the merge is recorded.
		return !policy.KeepsAcceptedWorktree() || tk.SpecLanded && tk.SpecMerge != nil && tk.SpecMerge.Counts
	}
	return false
}

// turnRunningOn reports whether a turn is running on ticket.
func (f *factory) turnRunningOn(ticket string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.turns {
		if t.claim.Ticket == ticket {
			return true
		}
	}
	return false
}

// setUpFailed is the failure for an error setting up the worktree at wt
// that isn't already one, such as a git command failing.
func (f *factory) setUpFailed(wt string, err error) *failure {
	return &failure{core.ReportFailureInput{
		WhatBroke:       fmt.Sprintf("The Factory couldn't set up this Ticket's worktree at %s: %v", wt, err),
		Why:             "A git command the Factory runs to set up the worktree failed, so no session could start.",
		WhatToFix:       "Read the git error above, fix what it names in the repository (a stale lock file, a bad base, a full disk), then send the Ticket back.",
		WhereWorkStands: fmt.Sprintf("No session ran under this Claim. The Factory stopped at the failed git command and touched nothing after it; check `git worktree list` for %s.", wt),
	}}
}

func (f *factory) strayDirectory(r *repository, wt string) error {
	return &failure{core.ReportFailureInput{
		WhatBroke:       fmt.Sprintf("Something that isn't a git worktree is at this Ticket's worktree path, %s, so the Factory can't set up the worktree.", wt),
		Why:             fmt.Sprintf("Each Ticket's worktree is at %s/<ticket-id>. %s exists but `git worktree list` doesn't list it, and the Factory never deletes or overwrites what it doesn't recognise.", r.worktrees, wt),
		WhatToFix:       fmt.Sprintf("Look at %s, keep anything worth keeping elsewhere, and remove or move it. Then send the Ticket back.", wt),
		WhereWorkStands: fmt.Sprintf("No session ran under this Claim. %s was left as it was.", wt),
	}}
}

func (f *factory) wrongWorktree(wt string, w gitWorktree, want string) error {
	on := "detached"
	if w.branch != "" {
		on = "on branch " + w.branch
	}
	return &failure{core.ReportFailureInput{
		WhatBroke:       fmt.Sprintf("The worktree at %s is %s, not %s, so the Factory can't use it.", wt, on, want),
		Why:             fmt.Sprintf("This Ticket's worktree must be %s. The Factory never switches or removes a worktree it doesn't recognise.", want),
		WhatToFix:       fmt.Sprintf("Look at %s, keep anything worth keeping, and remove it with `git worktree remove` (or switch it %s). Then send the Ticket back.", wt, want),
		WhereWorkStands: fmt.Sprintf("No session ran under this Claim. The worktree %s was left %s, as it was.", wt, on),
	}}
}

func (f *factory) branchElsewhere(branch, wt, elsewhere string) error {
	return &failure{core.ReportFailureInput{
		WhatBroke:       fmt.Sprintf("Branch %s is checked out at %s, so the Factory can't add this Ticket's worktree at %s on it.", branch, elsewhere, wt),
		Why:             "git checks a branch out in one worktree at a time, and the Factory never switches a worktree it doesn't own.",
		WhatToFix:       fmt.Sprintf("Switch %s off branch %s (or remove that worktree), then send the Ticket back.", elsewhere, branch),
		WhereWorkStands: fmt.Sprintf("No session ran under this Claim. Branch %s and %s were left as they were.", branch, elsewhere),
	}}
}
