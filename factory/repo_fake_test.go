package factory

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
)

// fakeRepo is an in-memory repo for testing landing and the sweep
// without git: commits are names with parents, refs point at them, and
// what only real git can tell (a merge's conflicts, a dirty worktree, a
// broken remote) is set by the test. The tests through factorytest
// run gitRepo against real git.
type fakeRepo struct {
	mu      sync.Mutex
	parents map[string][]string // commit -> its parents
	message map[string]string   // commit -> its message
	refs    map[string]string   // full ref name -> commit
	remotes map[string]*fakeRemote
	// conflicts is the files merging a head commit into any base
	// conflicts in.
	conflicts map[string][]string
	list      []gitWorktree     // the repo's own checkout first
	dirty     map[string]string // worktree path -> its status
	pushes    int
	next      int
}

// fakeRemote is a remote's branches; a broken one refuses every push.
type fakeRemote struct {
	refs   map[string]string // branch -> commit
	broken bool
}

// newFakeRepo is a repo whose checkout at dir is on main, at one commit.
func newFakeRepo(dir string) *fakeRepo {
	r := &fakeRepo{parents: map[string][]string{}, message: map[string]string{}, refs: map[string]string{}, remotes: map[string]*fakeRemote{}, conflicts: map[string][]string{}, dirty: map[string]string{}}
	r.refs["refs/heads/main"] = r.newCommit("Set up Code Central")
	r.list = []gitWorktree{{path: dir, branch: "main"}}
	return r
}

func (r *fakeRepo) newCommit(msg string, parents ...string) string {
	r.next++
	c := fmt.Sprintf("%040x", r.next)
	r.parents[c] = parents
	r.message[c] = msg
	return c
}

// commitOn commits on branch, made from main when it doesn't exist, and
// returns the commit.
func (r *fakeRepo) commitOn(branch, msg string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	ref := "refs/heads/" + branch
	parent, ok := r.refs[ref]
	if !ok {
		parent = r.refs["refs/heads/main"]
	}
	c := r.newCommit(msg, parent)
	r.refs[ref] = c
	return c
}

// mergeByHand merges branch into main with a merge commit, as the human
// would, and returns main's new tip.
func (r *fakeRepo) mergeByHand(branch string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.newCommit("My own merge", r.refs["refs/heads/main"], r.refs["refs/heads/"+branch])
	r.refs["refs/heads/main"] = c
	return c
}

// tip is the commit ref points at, "" when there is none.
func (r *fakeRepo) tip(ref string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.refs[ref]
}

// messageOf is commit's message.
func (r *fakeRepo) messageOf(commit string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.message[commit]
}

// addRemote adds the remote name, with no branches.
func (r *fakeRepo) addRemote(name string) *fakeRemote {
	r.mu.Lock()
	defer r.mu.Unlock()
	rm := &fakeRemote{refs: map[string]string{}}
	r.remotes[name] = rm
	return rm
}

// removeRemote removes the remote name.
func (r *fakeRepo) removeRemote(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.remotes, name)
}

// remoteTip is the commit the remote name's branch is at.
func (r *fakeRepo) remoteTip(name, branch string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.remotes[name].refs[branch]
}

// setBroken breaks the remote name's pushes, or mends them.
func (r *fakeRepo) setBroken(name string, broken bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.remotes[name].broken = broken
}

// addWorktree checks branch out at path.
func (r *fakeRepo) addWorktree(path, branch string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.list = append(r.list, gitWorktree{path: path, branch: branch})
}

// hasWorktree reports whether a worktree is at path.
func (r *fakeRepo) hasWorktree(path string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := find(r.list, path)
	return ok
}

// setDirty gives the worktree at path uncommitted changes, or none.
func (r *fakeRepo) setDirty(path, status string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dirty[path] = status
}

func (r *fakeRepo) resolve(rev string) (string, bool) {
	if c, ok := r.refs[rev]; ok {
		return c, true
	}
	_, ok := r.parents[rev]
	return rev, ok
}

func (r *fakeRepo) reachable(commit, from string) bool {
	seen := map[string]bool{}
	todo := []string{from}
	for len(todo) > 0 {
		c := todo[len(todo)-1]
		todo = todo[:len(todo)-1]
		if c == commit {
			return true
		}
		if !seen[c] {
			seen[c] = true
			todo = append(todo, r.parents[c]...)
		}
	}
	return false
}

func (r *fakeRepo) commit(rev string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.resolve(rev); ok {
		return c, nil
	}
	return "", fmt.Errorf("git rev-parse: %s isn't a commit", rev)
}

func (r *fakeRepo) isAncestor(commit, rev string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	tip, ok := r.resolve(rev)
	return ok && r.reachable(commit, tip)
}

func (r *fakeRepo) hasRemote(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.remotes[name] != nil
}

func (r *fakeRepo) mergeTree(base, head string) (string, []string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if files := r.conflicts[head]; len(files) > 0 {
		return "tree-with-conflicts", files, nil
	}
	return "tree-of-" + head, nil, nil
}

func (r *fakeRepo) commitTree(tree, msg string, parents ...string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.newCommit(msg, parents...), nil
}

func (r *fakeRepo) fastForward(dir, commit string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.dirty[dir] != "" {
		return "error: Your local changes to the following files would be overwritten by merge:\n\t" + r.dirty[dir], errors.New("exit status 1")
	}
	w, _ := find(r.list, dir)
	ref := "refs/heads/" + w.branch
	if !r.reachable(r.refs[ref], commit) {
		return "fatal: Not possible to fast-forward, aborting.", errors.New("exit status 128")
	}
	r.refs[ref] = commit
	return "", nil
}

func (r *fakeRepo) updateRef(ref, new, old string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.refs[ref] != old {
		return fmt.Errorf("git update-ref: %s isn't at %s", ref, old)
	}
	r.refs[ref] = new
	return nil
}

func (r *fakeRepo) specBranches() ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for ref := range r.refs {
		if b, ok := strings.CutPrefix(ref, "refs/heads/"); ok && strings.HasPrefix(b, "spec/") {
			out = append(out, b)
		}
	}
	slices.Sort(out)
	return out, nil
}

func (r *fakeRepo) deleteRef(ref, old string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.refs[ref] != old {
		return fmt.Errorf("git update-ref: %s isn't at %s", ref, old)
	}
	delete(r.refs, ref)
	return nil
}

func (r *fakeRepo) push(ctx context.Context, remote, branch string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rm := r.remotes[remote]
	if rm == nil || rm.broken {
		return fmt.Sprintf("fatal: '%s' does not appear to be a git repository", remote), errors.New("exit status 128")
	}
	local := r.refs["refs/heads/"+branch]
	if theirs := rm.refs[branch]; theirs != "" && !r.reachable(theirs, local) {
		return fmt.Sprintf(" ! [rejected]        %s -> %s (fetch first)", branch, branch), errors.New("exit status 1")
	}
	r.pushes++
	rm.refs[branch] = local
	r.refs["refs/remotes/"+remote+"/"+branch] = local
	return "", nil
}

func (r *fakeRepo) fetch(ctx context.Context, remote string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rm := r.remotes[remote]
	if rm == nil || rm.broken {
		return fmt.Sprintf("fatal: '%s' does not appear to be a git repository", remote), errors.New("exit status 128")
	}
	for branch, commit := range rm.refs {
		r.refs["refs/remotes/"+remote+"/"+branch] = commit
	}
	return "", nil
}

func (r *fakeRepo) worktrees() ([]gitWorktree, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.list), nil
}

func (r *fakeRepo) status(dir string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dirty[dir], nil
}

func (r *fakeRepo) removeWorktree(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.dirty[path] != "" {
		return fmt.Errorf("git worktree remove: %s contains modified or untracked files", path)
	}
	r.list = slices.DeleteFunc(r.list, func(w gitWorktree) bool { return w.path == path })
	return nil
}
