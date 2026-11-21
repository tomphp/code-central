package git

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// fakeVersion is the version a Fake gives until SetVersion.
const fakeVersion = "git version 2.47.0"

// InitBare makes an empty bare repository in the folder path, with main
// its HEAD, for repositories in the Fake to have as their remote. It fails
// the test when path is, is in or holds a worktree already.
func (f *Fake) InitBare(path string) {
	f.tb.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := os.MkdirAll(path, 0o755); err != nil {
		f.tb.Fatal(err)
	}
	path = realPath(path)
	if w := f.overlapping(path); w != nil {
		f.tb.Fatalf("fake git: %s is, is in or holds the worktree %s already", path, w.dir)
	}
	r := newFakeRepository(path, "refs/heads/main")
	r.bare = true
	f.repos[path] = r
}

// AddRemote gives the repository at dir the remote name at url, as git
// remote add does. It fails the test when dir holds no repository, or
// one with a remote name already.
func (f *Fake) AddRemote(dir, name, url string) {
	f.tb.Helper()
	f.withRepository(dir, func(r *fakeRepository) {
		if _, ok := r.remotes[name]; ok {
			f.tb.Fatalf("fake git: %s has a remote %s already", dir, name)
		}
		if r.remotes == nil {
			r.remotes = map[string]string{}
		}
		r.remotes[name] = url
	})
}

// RemoveRemote removes the remote name from the repository at dir, as git
// remote remove does, with its remote-tracking branches. It fails the test
// when dir holds no repository, or one without the remote.
func (f *Fake) RemoveRemote(dir, name string) {
	f.tb.Helper()
	f.withRepository(dir, func(r *fakeRepository) {
		if _, ok := r.remotes[name]; !ok {
			f.tb.Fatalf("fake git: %s has no remote %s", dir, name)
		}
		delete(r.remotes, name)
		for ref := range r.refs {
			if strings.HasPrefix(ref, "refs/remotes/"+name+"/") {
				delete(r.refs, ref)
			}
		}
	})
}

// SetRemoteBranch puts the remote's branch of the repository at dir at
// commit, a commit in that repository, with its history, as a push from
// elsewhere would: the repository's own refs, remote-tracking ones too,
// are left for a fetch. It fails the test when the remote isn't one the
// Fake models.
func (f *Fake) SetRemoteBranch(dir, remote, branch, commit string) {
	f.tb.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	g := fakeRepo{f: f, dir: filepath.Clean(dir)}
	r, err := g.repository()
	if err != nil {
		f.tb.Fatal(err)
	}
	url, ok := r.remotes[remote]
	if !ok {
		f.tb.Fatalf("fake git: %s has no remote %s", dir, remote)
	}
	to, _, err := f.reach(url)
	if err != nil {
		f.tb.Fatal(err)
	}
	id, err := g.resolve(r, commit)
	if err != nil {
		f.tb.Fatal(err)
	}
	copyObjects(r, to, id)
	to.refs["refs/heads/"+branch] = id
}

// MessageOf is the message of commit in the repository at dir, without
// its closing newline. It fails the test when there is no such commit.
func (f *Fake) MessageOf(dir, commit string) string {
	f.tb.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	g := fakeRepo{f: f, dir: filepath.Clean(dir)}
	r, err := g.repository()
	if err != nil {
		f.tb.Fatal(err)
	}
	c, ok := r.commits[commit]
	if !ok {
		f.tb.Fatalf("fake git: %s has no commit %s", dir, commit)
	}
	return strings.TrimSuffix(c.message, "\n")
}

// BreakRemote makes the repository at path unreachable as a remote, as
// though it were gone: pushes, fetches, ls-remote and clones of it fail
// until RestoreRemote.
func (f *Fake) BreakRemote(path string) {
	f.tb.Helper()
	f.withRepository(path, func(r *fakeRepository) { r.broken = true })
}

// RestoreRemote undoes BreakRemote.
func (f *Fake) RestoreRemote(path string) {
	f.tb.Helper()
	f.withRepository(path, func(r *fakeRepository) { r.broken = false })
}

// RefusePushes makes the repository at path refuse every push, as a
// pre-receive hook would, while still serving fetches.
func (f *Fake) RefusePushes(path string) {
	f.tb.Helper()
	f.withRepository(path, func(r *fakeRepository) { r.refusing = true })
}

// AcceptPushes undoes RefusePushes.
func (f *Fake) AcceptPushes(path string) {
	f.tb.Helper()
	f.withRepository(path, func(r *fakeRepository) { r.refusing = false })
}

// withRepository calls do with the repository at path, under the Fake's
// lock, failing the test when there is none.
func (f *Fake) withRepository(path string, do func(r *fakeRepository)) {
	f.tb.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	r := f.repos[realPath(path)]
	if r == nil {
		f.tb.Fatalf("fake git: %s holds no repository", path)
	}
	do(r)
}

// SetVersion makes v what Version gives, as git version would print it.
func (f *Fake) SetVersion(v string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.version = v
}

func (f *Fake) Version() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.version == "" {
		return fakeVersion, nil
	}
	return f.version, nil
}

func (f *Fake) TopLevel(dir string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.topLevel(dir)
}

// topLevel is the folder, resolved, of the worktree holding dir: a
// repository's own folder or one of its linked worktrees'.
func (f *Fake) topLevel(dir string) (string, error) {
	if _, err := os.Stat(dir); err != nil {
		return "", err
	}
	r, w := f.holding(realPath(dir))
	switch {
	case r == nil:
		return gitFailed("fatal: not a git repository (or any of the parent directories): .git")
	case r.bare:
		return "", f.notModelled("the top level of the bare repository %s", w.dir)
	}
	return w.dir, nil
}

// Clone clones the repository at url into path as git clone does: the
// clone has url's branches as remote-tracking branches under remote, and
// url's HEAD branch checked out in path. A path in or around a worktree
// isn't modelled.
func (f *Fake) Clone(ctx context.Context, remote, url, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	path = filepath.Clean(path)
	dir := realPath(path)
	if entries, err := os.ReadDir(path); err == nil && len(entries) > 0 || f.repos[dir] != nil {
		return gitFailed("fatal: destination path '%s' already exists and is not an empty directory.", path)
	}
	if w := f.overlapping(dir); w != nil {
		return "", f.notModelled("a clone at %s, with the worktree %s inside or around it", path, w.dir)
	}
	from, out, err := f.reach(url)
	if err != nil {
		return out, err
	}
	head, ok := from.refs[from.main().head]
	if !ok {
		return "", f.notModelled("a clone of %s, whose HEAD is no branch it has", url)
	}
	r := newFakeRepository(dir, from.main().head)
	r.remotes = map[string]string{remote: url}
	if err := f.fetchInto(r, from, remote); err != nil {
		return "", err
	}
	r.refs[r.main().head] = head
	if err := os.MkdirAll(path, 0o755); err != nil {
		return "", err
	}
	if err := checkOut(dir, fakeTree{}, r.treeOf(head)); err != nil {
		return "", err
	}
	f.repos[dir] = r
	return "", nil
}

// Rename renames the folder from to to as os.Rename does, and moves the
// repository there, with its worktrees inside it, along with it: git
// finds a repository wherever its folder is, the Fake only where it
// recorded it. A repository's linked worktrees elsewhere aren't moved.
func (f *Fake) Rename(from, to string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	from = realPath(from)
	if err := os.Rename(from, to); err != nil {
		return err
	}
	to = realPath(to)
	moved := map[string]*fakeRepository{}
	for dir, r := range f.repos {
		if !within(dir, from) {
			continue
		}
		for _, w := range r.worktrees {
			if within(w.dir, from) {
				w.dir = to + strings.TrimPrefix(w.dir, from)
			}
		}
		delete(f.repos, dir)
		moved[to+strings.TrimPrefix(dir, from)] = r
	}
	for dir, r := range moved {
		f.repos[dir] = r
	}
	return nil
}

// reach is the repository at url, as a remote, or git's output and error
// when it can't be reached. Only a folder holding a repository in the
// Fake is modelled.
func (f *Fake) reach(url string) (*fakeRepository, string, error) {
	if !filepath.IsAbs(url) {
		return nil, "", f.notModelled("the remote URL %q", url)
	}
	to := f.repos[realPath(url)]
	switch {
	case to == nil:
		return nil, "", f.notModelled("a remote outside the Fake, %s", url)
	case to.broken:
		out, err := unreachable(url)
		return nil, out, err
	}
	return to, "", nil
}

// unreachable is git's failure to reach the remote remote.
func unreachable(remote string) (string, error) {
	return gitFailed("fatal: '%s' does not appear to be a git repository\nfatal: Could not read from remote repository.\n\nPlease make sure you have the correct access rights\nand the repository exists.", remote)
}

// fetchInto copies the branches of from, with their commits, to r as its
// remote-tracking branches for remote, as a fetch with the refspec
// git remote add and git clone configure does. It doesn't prune. Tags
// aren't modelled.
func (f *Fake) fetchInto(r, from *fakeRepository, remote string) error {
	for _, ref := range slices.Sorted(maps.Keys(from.refs)) {
		if strings.HasPrefix(ref, "refs/tags/") {
			return f.notModelled("fetching the tag %s", ref)
		}
		if branch, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
			copyObjects(from, r, from.refs[ref])
			r.refs["refs/remotes/"+remote+"/"+branch] = from.refs[ref]
		}
	}
	return nil
}

// copyObjects copies commit, and its ancestors and their trees, from from
// to to.
func copyObjects(from, to *fakeRepository, commit string) {
	for c := range from.ancestors(commit) {
		fc := from.commits[c]
		to.commits[c] = fc
		to.trees[fc.tree] = from.trees[fc.tree]
	}
}

func (g fakeRepo) HasRemote(name string) bool {
	_, err := g.RemoteURL(name)
	return err == nil
}

func (g fakeRepo) RemoteURL(name string) (string, error) {
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return "", err
	}
	url, ok := r.remotes[name]
	if !ok {
		return "", fmt.Errorf("error: No such remote '%s'", name)
	}
	return url, nil
}

func (g fakeRepo) TopLevel() (string, error) {
	g.f.mu.Lock()
	defer g.f.mu.Unlock()
	return g.f.topLevel(g.dir)
}

// resolveRemote is the repository remote names, as one of r's remotes or
// as a URL, its URL and whether it is one of r's remotes; or git's output
// and error when it can't be reached.
func (g fakeRepo) resolveRemote(r *fakeRepository, remote string) (to *fakeRepository, url string, named bool, out string, err error) {
	url, named = r.remotes[remote]
	if !named {
		if !filepath.IsAbs(remote) && !strings.ContainsAny(remote, `:/\`) {
			out, err := unreachable(remote)
			return nil, "", false, out, err
		}
		url = remote
	}
	to, out, err = g.f.reach(url)
	return to, url, named, out, err
}

// Push pushes branch to remote as git push does, with lease as
// --force-with-lease, rejecting a push that isn't a fast-forward or
// misses its lease with git's output, and moving remote's
// remote-tracking branch when remote is one of the repository's remotes.
func (g fakeRepo) Push(ctx context.Context, remote, branch, lease string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return "", err
	}
	if branch == "" || branch == "HEAD" || strings.HasPrefix(branch, "refs/") || strings.ContainsAny(branch, ":+^~ *?[\\") {
		return "", g.f.notModelled("the push of %q", branch)
	}
	to, url, named, out, err := g.resolveRemote(r, remote)
	if err != nil {
		return out, err
	}
	if !to.bare {
		return "", g.f.notModelled("a push to %s, which isn't bare", url)
	}
	ref := "refs/heads/" + branch
	ours, ok := r.refs[ref]
	if !ok {
		return gitFailed("error: src refspec %s does not match any\nerror: failed to push some refs to '%s'", branch, url)
	}
	rejected := func(why string) (string, error) {
		return gitFailed("To %s\n ! [rejected]        %s -> %s (%s)\nerror: failed to push some refs to '%s'", url, branch, branch, why, url)
	}
	theirs, exists := to.refs[ref]
	track := func() {
		if named {
			r.refs["refs/remotes/"+remote+"/"+branch] = ours
		}
	}
	if exists && theirs == ours {
		track()
		return "Everything up-to-date", nil
	}
	forced := false
	switch {
	case lease != "":
		want, err := g.resolve(r, lease)
		if err != nil {
			want = lease
		}
		if !exists || theirs != want {
			return rejected("stale info")
		}
		forced = true
	case exists && theirs != ours && !r.ancestors(ours)[theirs]:
		if _, ok := r.commits[theirs]; !ok {
			return rejected("fetch first")
		}
		return rejected("non-fast-forward")
	}
	if to.refusing {
		return gitFailed("remote: pushes are refused\nTo %s\n ! [remote rejected] %s -> %s (pre-receive hook declined)\nerror: failed to push some refs to '%s'", url, branch, branch, url)
	}
	copyObjects(r, to, ours)
	to.refs[ref] = ours
	track()
	switch {
	case !exists:
		return fmt.Sprintf("To %s\n * [new branch]      %s -> %s", url, branch, branch), nil
	case forced && !r.ancestors(ours)[theirs]:
		return fmt.Sprintf("To %s\n + %s...%s %s -> %s (forced update)", url, theirs[:7], ours[:7], branch, branch), nil
	}
	return fmt.Sprintf("To %s\n   %s..%s  %s -> %s", url, theirs[:7], ours[:7], branch, branch), nil
}

func (g fakeRepo) Fetch(ctx context.Context, remote string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return "", err
	}
	from, url, named, out, err := g.resolveRemote(r, remote)
	if err != nil {
		return out, err
	}
	if !named {
		return "", g.f.notModelled("a fetch from %s, which isn't a remote of %s", url, g.dir)
	}
	return "", g.f.fetchInto(r, from, remote)
}

func (g fakeRepo) RemoteHas(ctx context.Context, remote, branch string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	r, unlock, err := g.lock()
	defer unlock()
	if err != nil {
		return false, err
	}
	to, _, _, out, err := g.resolveRemote(r, remote)
	if err != nil {
		return false, fmt.Errorf("git ls-remote --heads %s refs/heads/%s: %v: %s", remote, branch, err, out)
	}
	_, ok := to.refs["refs/heads/"+branch]
	return ok, nil
}
