package gittest

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/tomoram/code-central/factory/git"
)

// remoteCases are the contract cases for remotes, clone, the top level
// and the version, each setting up its git with open.
func remoteCases(t *testing.T, open func(t *testing.T) (Setup, git.Repo, string)) {
	ctx := context.Background()

	t.Run("the repository has its remote origin at its URL, and no other", func(t *testing.T) {
		s, repo, _ := open(t)

		if !repo.HasRemote("origin") {
			t.Error("HasRemote(origin) = false, want true")
		}
		if url := must(repo.RemoteURL("origin"))(t); !samePath(url, s.Remote) {
			t.Errorf("RemoteURL(origin) = %s, want %s", url, s.Remote)
		}
		if repo.HasRemote("nope") {
			t.Error("HasRemote(nope) = true, want false")
		}
		if url, err := repo.RemoteURL("nope"); err == nil {
			t.Errorf("RemoteURL(nope) = %s, want an error", url)
		}
	})

	t.Run("a push makes the remote's branch and fast-forwards it, moving its remote-tracking branch", func(t *testing.T) {
		s, repo, first := open(t)
		remote := s.Git.Open(s.Remote)
		next := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n"}})
		noError(t, repo.UpdateRef("refs/heads/topic", first, ""))

		push(t, repo, "topic", "")
		checkBranch(t, remote, "topic", first)
		checkBranch(t, repo, "origin/topic", first)
		push(t, repo, "topic", "")
		checkBranch(t, remote, "topic", first)

		noError(t, repo.UpdateRef("refs/heads/topic", next, first))
		push(t, repo, "topic", "")
		checkBranch(t, remote, "topic", next)
		checkBranch(t, repo, "origin/topic", next)
		if got := must(remote.TreeOf(next))(t); got != must(repo.TreeOf(next))(t) {
			t.Errorf("the remote's tree of the pushed commit = %s, want the repository's", got)
		}
	})

	t.Run("a push of a branch the repository hasn't fails", func(t *testing.T) {
		_, repo, _ := open(t)

		if out, err := repo.Push(ctx, "origin", "nothing", ""); err == nil {
			t.Errorf("Push of a missing branch succeeded: %s", out)
		}
		if has := must(repo.RemoteHas(ctx, "origin", "nothing"))(t); has {
			t.Error("RemoteHas(nothing) = true after a failed push, want false")
		}
	})

	t.Run("a push onto a branch moved on the remote is refused, leaving it as it was", func(t *testing.T) {
		s, repo, first := open(t)
		theirs := movedOnRemote(t, s, repo, first)
		ours := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "ours\n"}})
		noError(t, repo.UpdateRef("refs/heads/topic", ours, first))

		if out, err := repo.Push(ctx, "origin", "topic", ""); err == nil {
			t.Errorf("Push onto a moved branch succeeded: %s", out)
		}
		checkBranch(t, s.Git.Open(s.Remote), "topic", theirs)
		checkBranch(t, repo, "origin/topic", first)
	})

	t.Run("a fetch brings the remote's branches and their commits to its remote-tracking branches", func(t *testing.T) {
		s, repo, first := open(t)
		theirs := movedOnRemote(t, s, repo, first)
		if c, err := repo.Commit(theirs); err == nil {
			t.Fatalf("the repository has the commit made elsewhere, %s, before fetching", c)
		}

		if out, err := repo.Fetch(ctx, "origin"); err != nil {
			t.Fatalf("Fetch: %v: %s", err, out)
		}
		checkBranch(t, repo, "origin/topic", theirs)
		checkBranch(t, repo, "origin/main", first)
		checkBranch(t, repo, "topic", first)
		if got := must(repo.ParentsOf(theirs))(t); len(got) != 1 || got[0] != first {
			t.Errorf("the fetched commit's parents = %q, want %s", got, first)
		}
	})

	t.Run("a push with a lease forces over the remote's branch only where the lease is", func(t *testing.T) {
		s, repo, first := open(t)
		theirs := movedOnRemote(t, s, repo, first)
		if out, err := repo.Fetch(ctx, "origin"); err != nil {
			t.Fatalf("Fetch: %v: %s", err, out)
		}
		ours := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "ours\n"}})
		noError(t, repo.UpdateRef("refs/heads/topic", ours, first))
		noError(t, repo.UpdateRef("refs/heads/new", ours, ""))
		remote := s.Git.Open(s.Remote)

		if out, err := repo.Push(ctx, "origin", "topic", first); err == nil {
			t.Errorf("Push with a stale lease succeeded: %s", out)
		}
		checkBranch(t, remote, "topic", theirs)
		if out, err := repo.Push(ctx, "origin", "new", first); err == nil {
			t.Errorf("Push with a lease on a branch the remote hasn't succeeded: %s", out)
		}
		if has := must(repo.RemoteHas(ctx, "origin", "new"))(t); has {
			t.Error("RemoteHas(new) = true after a refused push, want false")
		}

		push(t, repo, "topic", theirs)
		checkBranch(t, remote, "topic", ours)
		checkBranch(t, repo, "origin/topic", ours)
		// A push that changes nothing is up to date before its lease is
		// checked, so a stale lease doesn't refuse it.
		push(t, repo, "topic", first)
	})

	t.Run("remoteHas asks the remote, so a branch deleted there since the last fetch isn't counted", func(t *testing.T) {
		s, repo, first := open(t)
		noError(t, repo.UpdateRef("refs/heads/topic", first, ""))
		push(t, repo, "main", "")
		push(t, repo, "topic", "")

		noError(t, s.Git.Open(s.Remote).DeleteRef("refs/heads/topic", first))
		if out, err := repo.Fetch(ctx, "origin"); err != nil {
			t.Fatalf("Fetch: %v: %s", err, out)
		}
		if has := must(repo.RemoteHas(ctx, "origin", "topic"))(t); has {
			t.Error("RemoteHas(topic) = true after the remote deleted it, want false")
		}
		if has := must(repo.RemoteHas(ctx, "origin", "main"))(t); !has {
			t.Error("RemoteHas(main) = false, want true")
		}
		checkBranch(t, repo, "origin/topic", first)
	})

	t.Run("a remote the repository hasn't fails push, fetch and remoteHas", func(t *testing.T) {
		_, repo, _ := open(t)

		if out, err := repo.Push(ctx, "nope", "main", ""); err == nil {
			t.Errorf("Push to a missing remote succeeded: %s", out)
		}
		if out, err := repo.Fetch(ctx, "nope"); err == nil {
			t.Errorf("Fetch from a missing remote succeeded: %s", out)
		}
		if has, err := repo.RemoteHas(ctx, "nope", "main"); err == nil {
			t.Errorf("RemoteHas on a missing remote = %t, want an error", has)
		}
	})

	t.Run("a clone has the remote's branches as remote-tracking branches, under the name given, with its HEAD checked out", func(t *testing.T) {
		s, repo, first := open(t)
		next := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n"}})
		noError(t, repo.UpdateRef("refs/heads/topic", next, ""))
		push(t, repo, "main", "")
		push(t, repo, "topic", "")
		path := filepath.Join(t.TempDir(), "clone")

		if out, err := s.Git.Clone(ctx, "upstream", s.Remote, path); err != nil {
			t.Fatalf("Clone: %v: %s", err, out)
		}
		clone := s.Git.Open(path)
		checkBranch(t, clone, "main", first)
		checkBranch(t, clone, "upstream/main", first)
		checkBranch(t, clone, "upstream/topic", next)
		if c, err := clone.Commit("refs/heads/topic"); err == nil {
			t.Errorf("the clone has a branch topic, at %s, want only upstream/topic", c)
		}
		if url := must(clone.RemoteURL("upstream"))(t); !samePath(url, s.Remote) {
			t.Errorf("the clone's RemoteURL(upstream) = %s, want %s", url, s.Remote)
		}
		if clone.HasRemote("origin") {
			t.Error("the clone has a remote origin, want only upstream")
		}
		checkFolder(t, path, map[string]string{"README.md": "Hello\n"})
	})

	t.Run("a clone into a folder holding files fails", func(t *testing.T) {
		s, repo, _ := open(t)
		push(t, repo, "main", "")

		if out, err := s.Git.Clone(ctx, "origin", s.Remote, s.Dir); err == nil {
			t.Errorf("Clone into the repository's own folder succeeded: %s", out)
		}
	})

	t.Run("the top level is the repository's folder, from it or a folder in it, and there is none outside a repository", func(t *testing.T) {
		s, repo, _ := open(t)
		sub := filepath.Join(s.Dir, "d", "e")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}

		for _, dir := range []string{s.Dir, sub} {
			if top := must(s.Git.TopLevel(dir))(t); !samePath(top, s.Dir) {
				t.Errorf("TopLevel(%s) = %s, want %s", dir, top, s.Dir)
			}
		}
		if top := must(repo.TopLevel())(t); !samePath(top, s.Dir) {
			t.Errorf("the repository's TopLevel = %s, want %s", top, s.Dir)
		}
		outside := t.TempDir()
		if top, err := s.Git.TopLevel(outside); err == nil {
			t.Errorf("TopLevel outside a repository = %s, want an error", top)
		}
		if top, err := s.Git.Open(outside).TopLevel(); err == nil {
			t.Errorf("TopLevel of a folder that isn't a repository = %s, want an error", top)
		}
	})

	t.Run("the version is git's, as git version reports it", func(t *testing.T) {
		s, _, _ := open(t)

		if v := must(s.Git.Version())(t); !regexp.MustCompile(`^git version \d+\.\d+`).MatchString(v) {
			t.Errorf("Version = %q, want git version and a number", v)
		}
	})
}

// movedOnRemote pushes the branch topic, at first, from repo to its
// remote, and moves it there to a commit made in another clone, which
// repo hasn't, and returns that commit.
func movedOnRemote(t *testing.T, s Setup, repo git.Repo, first string) string {
	t.Helper()
	noError(t, repo.UpdateRef("refs/heads/topic", first, ""))
	push(t, repo, "main", "")
	push(t, repo, "topic", "")
	path := filepath.Join(t.TempDir(), "elsewhere")
	if out, err := s.Git.Clone(context.Background(), "origin", s.Remote, path); err != nil {
		t.Fatalf("Clone: %v: %s", err, out)
	}
	elsewhere := s.Git.Open(path)
	tree := must(elsewhere.TreeOf("origin/topic"))(t)
	theirs := must(elsewhere.CommitTree(tree, "Theirs", "origin/topic"))(t)
	noError(t, elsewhere.UpdateRef("refs/heads/topic", theirs, ""))
	push(t, elsewhere, "topic", "")
	return theirs
}

// push pushes branch from repo to origin, with lease, failing t when it
// can't.
func push(t *testing.T, repo git.Repo, branch, lease string) {
	t.Helper()
	if out, err := repo.Push(context.Background(), "origin", branch, lease); err != nil {
		t.Fatalf("Push(%s, %q): %v: %s", branch, lease, err, out)
	}
}

// samePath reports whether a and b are the same folder, however each is
// written: git may resolve symbolic links, and writes / on Windows.
func samePath(a, b string) bool {
	ia, err := os.Stat(a)
	if err != nil {
		return false
	}
	ib, err := os.Stat(b)
	return err == nil && os.SameFile(ia, ib)
}
