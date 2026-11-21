// Typed git helpers for tests that look at where a worktree is, which serve
// both gits like the ones in git.go.

package factorytest

// HeadIn is the commit checked out in the worktree at dir, which the
// repo's worktree list holds.
func (h *Harness) HeadIn(dir string) string {
	h.t.Helper()
	return h.head(dir)
}

// BranchIn is the branch checked out in the worktree at dir, "" when it is
// detached.
func (h *Harness) BranchIn(dir string) string {
	h.t.Helper()
	if h.fake != nil {
		trees, err := h.fake.Open(h.Repo).Worktrees()
		must(h.t, err)
		for _, w := range trees {
			if w.Path == dir {
				return w.Branch
			}
		}
		h.t.Fatalf("factorytest: the repo has no worktree at %s", dir)
	}
	return h.git(dir, "branch", "--show-current")
}

// AddDetachedWorktree adds a worktree for ticket at the Factory's place for
// it, Worktree(ticket), detached at the tip of branch, as a Stage's is.
func (h *Harness) AddDetachedWorktree(ticket, branch string) {
	h.t.Helper()
	path := h.Worktree(ticket)
	if h.fake != nil {
		must(h.t, h.fake.Open(h.Repo).AddDetachedWorktree(path, h.tip(h.Repo, branch)))
		return
	}
	h.git(h.Repo, "worktree", "add", "-q", "--detach", path, branch)
}
