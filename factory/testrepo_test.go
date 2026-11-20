package factory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/tomoram/code-central/factory/git"
)

// testRepo is a repository in a git.Fake, with the set-up a landing or
// sweep test wants made short: commits on a branch, a merge by hand, a
// remote with its branches, a dirty worktree. It is the git.Repo the
// Factory is given, counting the pushes that went through. The tests
// through factorytest run git.Real against real git.
type testRepo struct {
	git.Repo
	t       testing.TB
	f       *git.Fake
	dir     string
	remotes map[string]string // remote name -> its bare repository's folder
	next    atomic.Int32
	pushes  atomic.Int32
	mu      sync.Mutex
	last    map[string]string // branch -> the file commitOn last wrote on it
}

// newTestRepo is a repository in f at dir, whose checkout is on main, at
// one commit.
func newTestRepo(t testing.TB, f *git.Fake, dir string) *testRepo {
	t.Helper()
	f.Init(dir, map[string]string{"README": "Code Central\n"}, "Set up Code Central")
	return &testRepo{Repo: f.Open(dir), t: t, f: f, dir: dir, remotes: map[string]string{}, last: map[string]string{}}
}

// scratchDir is a new folder, with its symlinks resolved as the Fake
// does.
func scratchDir(t testing.TB) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// Push counts the pushes that succeed.
func (r *testRepo) Push(ctx context.Context, remote, branch, lease string) (string, error) {
	out, err := r.Repo.Push(ctx, remote, branch, lease)
	if err == nil {
		r.pushes.Add(1)
	}
	return out, err
}

// commitOn commits on branch, made from main when it doesn't exist, in a
// file of its own, and returns the commit.
func (r *testRepo) commitOn(branch, msg string) string {
	r.t.Helper()
	file := fmt.Sprintf("work/%d.txt", r.next.Add(1))
	r.mu.Lock()
	r.last[branch] = file
	r.mu.Unlock()
	return r.commitFileOn(branch, msg, file, msg+"\n")
}

// fileOf is the file commitOn last wrote on branch: leaving it dirty in
// the checkout of the base holds back a merge of the branch.
func (r *testRepo) fileOf(branch string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.last[branch]
}

// commitFileOn is commitOn writing content, one line, to file: a branch
// and main that each write one to the same file conflict.
func (r *testRepo) commitFileOn(branch, msg, file, content string) string {
	r.t.Helper()
	parent := r.tip("refs/heads/" + branch)
	if parent == "" {
		parent = r.tip("refs/heads/main")
	}
	c := r.f.MakeCommit(r.dir, git.Change{Parents: []string{parent}, Files: map[string]string{file: content}, Message: msg})
	r.moveBranch(branch, c)
	return c
}

// mergeByHand merges branch into main with a merge commit, as the human
// would, and returns main's new tip.
func (r *testRepo) mergeByHand(branch string) string {
	r.t.Helper()
	main := r.tip("refs/heads/main")
	tree, conflicts, err := r.MergeTree(main, r.tip("refs/heads/"+branch))
	if err != nil || len(conflicts) > 0 {
		r.t.Fatalf("merging %s by hand: %v, conflicts %v", branch, err, conflicts)
	}
	c, err := r.CommitTree(tree, "My own merge", main, r.tip("refs/heads/"+branch))
	if err != nil {
		r.t.Fatal(err)
	}
	r.moveBranch("main", c)
	return c
}

// moveBranch moves branch to commit, which descends from where it is,
// through the worktree it is checked out in, if any.
func (r *testRepo) moveBranch(branch, commit string) {
	r.t.Helper()
	list, err := r.Worktrees()
	if err != nil {
		r.t.Fatal(err)
	}
	for _, w := range list {
		if w.Branch == branch {
			if out, err := r.FastForward(w.Path, commit); err != nil {
				r.t.Fatalf("moving %s: %v\n%s", branch, err, out)
			}
			return
		}
	}
	r.setBranch(branch, commit)
}

// setBranch points branch at commit, a branch nothing has checked out.
func (r *testRepo) setBranch(branch, commit string) {
	r.t.Helper()
	ref := "refs/heads/" + branch
	if err := r.UpdateRef(ref, commit, r.tip(ref)); err != nil {
		r.t.Fatal(err)
	}
}

// tip is the commit ref points at, "" when there is none.
func (r *testRepo) tip(ref string) string {
	c, err := r.Commit(ref)
	if err != nil {
		return ""
	}
	return c
}

// messageOf is commit's message.
func (r *testRepo) messageOf(commit string) string { return r.f.MessageOf(r.dir, commit) }

// addRemote adds the remote name, an empty bare repository.
func (r *testRepo) addRemote(name string) {
	r.t.Helper()
	bare := filepath.Join(scratchDir(r.t), name+".git")
	r.f.InitBare(bare)
	r.f.AddRemote(r.dir, name, bare)
	r.remotes[name] = bare
}

// removeRemote removes the remote name from the clone.
func (r *testRepo) removeRemote(name string) { r.f.RemoveRemote(r.dir, name) }

// remoteTip is the commit the remote name's branch is at, "" when it has
// none.
func (r *testRepo) remoteTip(name, branch string) string {
	c, err := r.f.Open(r.remotes[name]).Commit("refs/heads/" + branch)
	if err != nil {
		return ""
	}
	return c
}

// setRemoteBranch puts the remote name's branch at commit, as a push from
// elsewhere would, and fetches it.
func (r *testRepo) setRemoteBranch(name, branch, commit string) {
	r.t.Helper()
	r.f.SetRemoteBranch(r.dir, name, branch, commit)
	if out, err := r.Fetch(context.Background(), name); err != nil {
		r.t.Fatalf("fetching %s: %v\n%s", name, err, out)
	}
}

// deleteRemoteBranch deletes the remote name's branch, leaving the clone's
// remote-tracking ref of it.
func (r *testRepo) deleteRemoteBranch(name, branch string) {
	r.t.Helper()
	if err := r.f.Open(r.remotes[name]).DeleteRef("refs/heads/"+branch, r.remoteTip(name, branch)); err != nil {
		r.t.Fatal(err)
	}
}

// setBroken breaks the remote name, or mends it.
func (r *testRepo) setBroken(name string, broken bool) {
	if broken {
		r.f.BreakRemote(r.remotes[name])
	} else {
		r.f.RestoreRemote(r.remotes[name])
	}
}

// setRefusing makes the remote name refuse pushes, still serving fetches,
// or accept them again.
func (r *testRepo) setRefusing(name string, refusing bool) {
	if refusing {
		r.f.RefusePushes(r.remotes[name])
	} else {
		r.f.AcceptPushes(r.remotes[name])
	}
}

// addWorktree checks branch out at path, making it from main when it
// doesn't exist.
func (r *testRepo) addWorktree(path, branch string) {
	r.t.Helper()
	if r.tip("refs/heads/"+branch) == "" {
		r.setBranch(branch, r.tip("refs/heads/main"))
	}
	if err := r.AddWorktree(path, branch); err != nil {
		r.t.Fatal(err)
	}
}

// hasWorktree reports whether a worktree is at path.
func (r *testRepo) hasWorktree(path string) bool {
	r.t.Helper()
	list, err := r.Worktrees()
	if err != nil {
		r.t.Fatal(err)
	}
	_, ok := find(list, path)
	return ok
}

// makeDirty leaves file, untracked, in the worktree at path.
func (r *testRepo) makeDirty(path, file string) {
	r.t.Helper()
	full := filepath.Join(path, file)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("not committed\n"), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

// makeClean removes file from the worktree at path.
func (r *testRepo) makeClean(path, file string) {
	r.t.Helper()
	if err := os.Remove(filepath.Join(path, file)); err != nil {
		r.t.Fatal(err)
	}
}

// detach checks main's tip out detached in the repository's own folder,
// so that no checkout is on main.
func (r *testRepo) detach() {
	r.t.Helper()
	if err := r.CheckoutDetached(r.dir, r.tip("refs/heads/main")); err != nil {
		r.t.Fatal(err)
	}
}
