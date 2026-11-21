// More typed git helpers, for the tests of Repositories, Remotes and
// worktrees, that serve real git and the Fake git alike (see git.go).

package factorytest

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
)

// SubjectIn is the subject of commit in the repository at dir.
func (h *Harness) SubjectIn(dir, commit string) string {
	h.t.Helper()
	if h.fake != nil {
		return h.fake.Subject(dir, commit)
	}
	return h.git(dir, "log", "-1", "--format=%s", commit)
}

// Head is the commit checked out in the repo's worktree, an absolute
// path.
func (h *Harness) Head(worktree string) string { h.t.Helper(); return h.head(h.Repo, worktree) }

// Head is Harness.Head for r's clone.
func (r *Repository) Head(worktree string) string { r.h.t.Helper(); return r.h.head(r.Path, worktree) }

// CheckedOut is the branch checked out in the repo's worktree, an
// absolute path, failing the test when it is detached.
func (h *Harness) CheckedOut(worktree string) string {
	h.t.Helper()
	return h.checkedOut(h.Repo, worktree)
}

// CheckedOut is Harness.CheckedOut for r's clone.
func (r *Repository) CheckedOut(worktree string) string {
	r.h.t.Helper()
	return r.h.checkedOut(r.Path, worktree)
}

// HasRemote reports whether the repo has the remote name.
func (h *Harness) HasRemote(name string) bool { h.t.Helper(); return h.hasRemote(h.Repo, name) }

// HasRemote is Harness.HasRemote for r's clone.
func (r *Repository) HasRemote(name string) bool { r.h.t.Helper(); return r.h.hasRemote(r.Path, name) }

// RemoteURL is the URL of the repo's remote name, failing the test when
// it has none.
func (h *Harness) RemoteURL(name string) string { h.t.Helper(); return h.remoteURL(h.Repo, name) }

// RemoteURL is Harness.RemoteURL for r's clone.
func (r *Repository) RemoteURL(name string) string {
	r.h.t.Helper()
	return r.h.remoteURL(r.Path, name)
}

// SetRemote makes the repo's remote name have the URL url, adding the
// remote when it has none.
func (h *Harness) SetRemote(name, url string) { h.t.Helper(); h.setRemote(h.Repo, name, url) }

// SetRemote is Harness.SetRemote for r's clone.
func (r *Repository) SetRemote(name, url string) { r.h.t.Helper(); r.h.setRemote(r.Path, name, url) }

// Fetch fetches the remote in r's clone, as a human would, so its
// remote-tracking branches are up to date.
func (r *Repository) Fetch(remote string) { r.h.t.Helper(); r.h.fetch(r.Path, remote) }

// SpecBranches is the names of the spec/ branches in r's clone, sorted.
func (r *Repository) SpecBranches() []string { r.h.t.Helper(); return r.h.specBranches(r.Path) }

// SpecBranches is Repository.SpecBranches in the repo.
func (h *Harness) SpecBranches() []string { h.t.Helper(); return h.specBranches(h.Repo) }

// Status is the porcelain status of the repo's worktree, an absolute path.
func (h *Harness) Status(worktree string) string { h.t.Helper(); return h.status(h.Repo, worktree) }

// Status is Harness.Status for r's clone.
func (r *Repository) Status(worktree string) string {
	r.h.t.Helper()
	return r.h.status(r.Path, worktree)
}

// AddDetachedWorktree adds a worktree of r's clone at path, detached at
// rev, as a human would by hand.
func (r *Repository) AddDetachedWorktree(path, rev string) {
	r.h.t.Helper()
	if r.h.fake != nil {
		must(r.h.t, r.h.fake.Open(r.Path).AddDetachedWorktree(path, rev))
		return
	}
	r.h.git(r.Path, "worktree", "add", "-q", "--detach", path, rev)
}

// DiscardWorktree removes r's worktree at path and force-deletes its
// branch, as a human who merged it by hand would.
func (r *Repository) DiscardWorktree(path, branch string) {
	r.h.t.Helper()
	if r.h.fake != nil {
		g := r.h.fake.Open(r.Path)
		must(r.h.t, g.RemoveWorktree(path))
		must(r.h.t, g.DeleteRef("refs/heads/"+branch, r.h.tip(r.Path, branch)))
		return
	}
	r.h.git(r.Path, "worktree", "remove", path)
	r.h.git(r.Path, "branch", "-D", branch)
}

// Branch makes the branch in the bare repository at from, a branch or
// commit, as a push from elsewhere would.
func (r *Remote) Branch(branch, from string) {
	r.h.t.Helper()
	if r.h.fake != nil {
		must(r.h.t, r.h.fake.Open(r.Path).UpdateRef("refs/heads/"+branch, r.h.commit(r.Path, from), ""))
		return
	}
	r.h.git(r.Path, "branch", branch, from)
}

// CloneByHand clones url into path, which must not exist, as a human
// would; the clone's remote is origin.
func (h *Harness) CloneByHand(url, path string) {
	h.t.Helper()
	if h.fake != nil {
		if out, err := h.fake.Clone(context.Background(), "origin", url, path); err != nil {
			h.t.Fatalf("factorytest: clone %s to %s: %v\n%s", url, path, err, out)
		}
		return
	}
	h.git(filepath.Dir(path), "clone", "-q", url, path)
}

func (h *Harness) head(dir, worktree string) string {
	h.t.Helper()
	if h.fake == nil {
		return h.git(worktree, "rev-parse", "HEAD")
	}
	id, _, err := h.fake.Open(dir).LastCommit(worktree)
	must(h.t, err)
	return h.commit(dir, id)
}

func (h *Harness) checkedOut(dir, worktree string) string {
	h.t.Helper()
	if h.fake == nil {
		b := h.git(worktree, "branch", "--show-current")
		if b == "" {
			h.t.Fatalf("factorytest: %s has no branch checked out", worktree)
		}
		return b
	}
	b, err := h.fake.Open(dir).CurrentBranch(worktree)
	must(h.t, err)
	return b
}

func (h *Harness) hasRemote(dir, name string) bool {
	h.t.Helper()
	if h.fake != nil {
		return h.fake.Open(dir).HasRemote(name)
	}
	for _, r := range strings.Fields(h.git(dir, "remote")) {
		if r == name {
			return true
		}
	}
	return false
}

func (h *Harness) remoteURL(dir, name string) string {
	h.t.Helper()
	if h.fake != nil {
		url, err := h.fake.Open(dir).RemoteURL(name)
		must(h.t, err)
		return url
	}
	return h.git(dir, "remote", "get-url", name)
}

func (h *Harness) setRemote(dir, name, url string) {
	h.t.Helper()
	switch {
	case h.fake != nil && h.fake.Open(dir).HasRemote(name):
		h.fake.RemoveRemote(dir, name)
		h.fake.AddRemote(dir, name, url)
	case h.fake != nil:
		h.fake.AddRemote(dir, name, url)
	case h.hasRemote(dir, name):
		h.git(dir, "remote", "set-url", name, url)
	default:
		h.git(dir, "remote", "add", name, url)
	}
}

func (h *Harness) fetch(dir, remote string) {
	h.t.Helper()
	if h.fake != nil {
		if out, err := h.fake.Open(dir).Fetch(context.Background(), remote); err != nil {
			h.t.Fatalf("factorytest: fetch %s in %s: %v\n%s", remote, dir, err, out)
		}
		return
	}
	h.git(dir, "fetch", "-q", remote)
}

func (h *Harness) specBranches(dir string) []string {
	h.t.Helper()
	if h.fake != nil {
		names, err := h.fake.Open(dir).SpecBranches()
		must(h.t, err)
		return names
	}
	out := h.git(dir, "for-each-ref", "--format=%(refname:short)", "refs/heads/spec/")
	if out == "" {
		return nil
	}
	names := strings.Split(out, "\n")
	sort.Strings(names)
	return names
}

func (h *Harness) status(dir, worktree string) string {
	h.t.Helper()
	if h.fake != nil {
		out, err := h.fake.Open(dir).Status(worktree)
		must(h.t, err)
		return strings.TrimSpace(out)
	}
	return h.git(worktree, "status", "--porcelain")
}

// HasRemoteTip reports whether r's clone has a remote-tracking branch for
// branch on remote.
func (r *Repository) HasRemoteTip(remote, branch string) bool {
	r.h.t.Helper()
	_, ok := r.h.lookup(r.Path, "refs/remotes/"+remote+"/"+branch)
	return ok
}

// BranchOf is the branch checked out in the worktree, "" when it is
// detached.
func (h *Harness) BranchOf(worktree string) string {
	h.t.Helper()
	if h.fake == nil {
		return h.git(worktree, "branch", "--show-current")
	}
	b, err := h.fake.Open(worktree).CurrentBranch(worktree)
	if err != nil {
		return ""
	}
	return b
}

// RepositoryAt is a Repository named name whose clone is at path, one
// the test made or will have made there: it has no Clone URL.
func (h *Harness) RepositoryAt(name, path string) *Repository {
	return &Repository{Name: name, Path: path, h: h}
}

// CheckoutNewBranch makes the branch name at what r's clone has checked
// out and checks it out, as a human would by hand.
func (r *Repository) CheckoutNewBranch(name string) {
	r.h.t.Helper()
	if r.h.fake != nil {
		if out, err := r.h.fake.Run(r.Path, "checkout", "-q", "-b", name); err != nil {
			r.h.t.Fatalf("factorytest: checkout -b %s in %s: %v\n%s", name, r.Path, err, out)
		}
		return
	}
	r.h.git(r.Path, "checkout", "-q", "-b", name)
}
