package factory

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

func (f *factory) worktree(ticket string) string { return filepath.Join(f.worktrees, ticket) }

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

// implementationWorktree gives implementation Ticket id its worktree at
// wt on branch id: reused when it is there, else added on the existing
// branch, else on a new branch from the base's tip. Anything else at wt,
// or branch id checked out elsewhere, is a failure, and nothing is
// touched.
func (f *factory) implementationWorktree(id, wt string) error {
	list, err := f.repository.worktrees()
	if err != nil {
		return err
	}
	if w, ok := find(list, wt); ok {
		if w.branch == id {
			return nil
		}
		return f.wrongWorktree(wt, w, "on branch "+id)
	}
	if _, err := os.Stat(wt); err == nil {
		return f.strayDirectory(wt)
	}
	if _, err := f.gitOut(f.repo, "rev-parse", "--verify", "-q", "refs/heads/"+id); err == nil {
		for _, w := range list {
			if w.branch == id {
				return f.branchElsewhere(id, wt, w.path)
			}
		}
		_, err := f.gitOut(f.repo, "worktree", "add", "-q", wt, id)
		return err
	}
	_, err = f.gitOut(f.repo, "worktree", "add", "-q", "-b", id, wt, f.base)
	return err
}

// stageWorktree gives Stage Ticket id its worktree at wt: detached at the
// base's tip, added when it isn't there and moved to the current tip when
// it is. A dirty one is left as it is, as a failure: the Stage wrote files
// it shouldn't have. Anything at wt but a detached worktree is a failure
// too, and nothing is touched.
func (f *factory) stageWorktree(id, wt string) error {
	list, err := f.repository.worktrees()
	if err != nil {
		return err
	}
	w, ok := find(list, wt)
	if !ok {
		if _, err := os.Stat(wt); err == nil {
			return f.strayDirectory(wt)
		}
		_, err := f.gitOut(f.repo, "worktree", "add", "-q", "--detach", wt, f.base)
		return err
	}
	if w.branch != "" {
		return f.wrongWorktree(wt, w, "detached")
	}
	status, err := f.repository.status(wt)
	if err != nil {
		return err
	}
	if status != "" {
		head, _ := f.gitOut(wt, "rev-parse", "--short", "HEAD")
		return &failure{core.ReportFailureInput{
			WhatBroke:       fmt.Sprintf("The Stage's worktree is dirty, so the Factory can't start the Stage. Changed files:\n\n%s", status),
			Why:             fmt.Sprintf("A Stage writes nothing to the repository, but %s has uncommitted changes, most likely written by an earlier Claim's session. The Factory moves a Stage's worktree to %s's tip at each new Claim only when it is clean.", wt, f.base),
			WhatToFix:       fmt.Sprintf("Look at the changes in %s and keep anything worth keeping elsewhere (a Stage's output belongs in Code Central). Then clean the worktree and send the Ticket back.", wt),
			WhereWorkStands: fmt.Sprintf("No session ran under this Claim. The worktree %s is detached at %s, with the changes above left as they were.", wt, head),
		}}
	}
	_, err = f.gitOut(wt, "checkout", "-q", "--detach", f.base)
	return err
}

// prune runs `git worktree prune`, dropping worktrees whose folders are
// gone.
func (f *factory) prune() {
	if _, err := f.gitOut(f.repo, "worktree", "prune"); err != nil {
		f.printf("warning: %v", err)
	}
}

// sweep looks at every worktree `git worktree list` reports in the
// Factory's folder. One whose Ticket is Done or Cancelled, or Accepted
// under the Branch Landing Method, so the human can check its branch
// out, is removed with `git worktree remove` when clean, and left with a
// warning line when dirty. Branches are never deleted, and anything else
// is kept.
func (f *factory) sweep(ctx context.Context, method store.LandingMethod) {
	list, err := f.repository.worktrees()
	if err != nil {
		f.printf("warning: can't list the worktrees: %v", err)
		return
	}
	for _, w := range list {
		if ctx.Err() != nil {
			return
		}
		if filepath.Dir(w.path) != f.worktrees {
			continue
		}
		id := filepath.Base(w.path)
		if f.turnRunningOn(id) {
			continue
		}
		tk, err := f.client.GetTicket(ctx, f.project, core.Ref(id))
		if err != nil {
			var ce *core.Error
			if ctx.Err() == nil && !(errors.As(err, &ce) && ce.Code == core.NotFound) {
				f.printf("%s: get_ticket: %v", id, err)
			}
			continue
		}
		if !doneWithWorktree(tk.State, method) {
			continue
		}
		status, err := f.repository.status(w.path)
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
		if err := f.repository.removeWorktree(w.path); err != nil {
			f.printf("warning: %s: can't remove the worktree %s: %v", id, w.path, err)
			continue
		}
		delete(f.warned, w.path)
		f.printf("%s: %s: removed its worktree %s", id, tk.State, w.path)
	}
}

// doneWithWorktree reports whether the Factory is done with the worktree
// of a Ticket in state, under the Landing Method.
func doneWithWorktree(state store.State, method store.LandingMethod) bool {
	return state == store.Done || state == store.Cancelled || state == store.Accepted && method == store.LandingBranch
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

func (f *factory) strayDirectory(wt string) error {
	return &failure{core.ReportFailureInput{
		WhatBroke:       fmt.Sprintf("Something that isn't a git worktree is at this Ticket's worktree path, %s, so the Factory can't set up the worktree.", wt),
		Why:             fmt.Sprintf("Each Ticket's worktree is at %s/<ticket-id>. %s exists but `git worktree list` doesn't list it, and the Factory never deletes or overwrites what it doesn't recognise.", f.worktrees, wt),
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
