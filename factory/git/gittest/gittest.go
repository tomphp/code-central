// Package gittest holds the Git contract suite every git.Git
// implementation must pass, so that the Fake git behaves as real git does
// wherever the Factory relies on it. Run it from each implementation's
// tests with a function that sets up a fresh repository.
package gittest

import (
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/tomoram/code-central/factory/git"
)

// Setup is one case's git: a Git with a repository in it.
type Setup struct {
	// Git is the implementation under test.
	Git git.Git
	// Dir is the repository's folder. main is checked out there, at a
	// commit holding only README.md, which reads "Hello\n", and nothing
	// is uncommitted.
	Dir string
	// Commit makes c in the repository at Dir and returns its ID. It
	// moves no ref and touches no folder.
	Commit func(c git.Change) string
}

// Run runs the contract suite, with newGit setting up each case's git.
func Run(t *testing.T, newGit func(t *testing.T) Setup) {
	t.Run("a commit is found by its branch, its full ref, its ID and its short ID", func(t *testing.T) {
		s := newGit(t)
		repo := s.Git.Open(s.Dir)
		main := must(repo.Commit("refs/heads/main"))(t)
		c := s.Commit(git.Change{Parents: []string{main}, Files: map[string]string{"a.txt": "one\n"}})
		ok(t, repo.UpdateRef("refs/heads/topic", c, ""))

		for _, rev := range []string{"topic", "refs/heads/topic", c, c[:7]} {
			if got := must(repo.Commit(rev))(t); got != c {
				t.Errorf("Commit(%q) = %s, want %s", rev, got, c)
			}
		}
		if got, err := repo.Commit("nothing"); err == nil {
			t.Errorf("Commit of a missing branch = %s, want an error", got)
		}
	})

	t.Run("a one-line change on both sides conflicts on merge", func(t *testing.T) {
		s := newGit(t)
		repo := s.Git.Open(s.Dir)
		base := s.Commit(git.Change{Files: map[string]string{"a.txt": "one\n", "b.txt": "b\n"}})
		ours := s.Commit(git.Change{Parents: []string{base}, Files: map[string]string{"a.txt": "ours\n"}})
		theirs := s.Commit(git.Change{Parents: []string{base}, Files: map[string]string{"a.txt": "theirs\n", "b.txt": "theirs\n"}})

		if _, conflicts := mergeTree(t, repo, ours, theirs); !slices.Equal(conflicts, []string{"a.txt"}) {
			t.Errorf("conflicts = %q, want a.txt", conflicts)
		}
	})

	t.Run("a file added on both sides with different lines conflicts on merge", func(t *testing.T) {
		s := newGit(t)
		repo := s.Git.Open(s.Dir)
		base := s.Commit(git.Change{Files: map[string]string{"b.txt": "b\n"}})
		ours := s.Commit(git.Change{Parents: []string{base}, Files: map[string]string{"a.txt": "ours\n"}})
		theirs := s.Commit(git.Change{Parents: []string{base}, Files: map[string]string{"a.txt": "theirs\n"}})

		if _, conflicts := mergeTree(t, repo, ours, theirs); !slices.Equal(conflicts, []string{"a.txt"}) {
			t.Errorf("conflicts = %q, want a.txt", conflicts)
		}
	})

	t.Run("a file changed on one side and deleted on the other conflicts on merge", func(t *testing.T) {
		s := newGit(t)
		repo := s.Git.Open(s.Dir)
		base := s.Commit(git.Change{Files: map[string]string{"a.txt": "one\n", "b.txt": "b\n"}})
		ours := s.Commit(git.Change{Parents: []string{base}, Files: map[string]string{"a.txt": "ours\n"}})
		theirs := s.Commit(git.Change{Parents: []string{base}, Removed: []string{"a.txt"}})

		if _, conflicts := mergeTree(t, repo, ours, theirs); !slices.Equal(conflicts, []string{"a.txt"}) {
			t.Errorf("conflicts = %q, want a.txt", conflicts)
		}
	})

	t.Run("a change on one side merges cleanly, whichever side made it", func(t *testing.T) {
		s := newGit(t)
		repo := s.Git.Open(s.Dir)
		base := s.Commit(git.Change{Files: map[string]string{"a.txt": "one\n", "b.txt": "b\n"}})
		changed := s.Commit(git.Change{Parents: []string{base}, Files: map[string]string{"a.txt": "two\nlines\n"}, Removed: []string{"b.txt"}})
		other := s.Commit(git.Change{Parents: []string{base}, Message: "Nothing changed"})

		want := must(repo.TreeOf(changed))(t)
		for _, sides := range [][2]string{{changed, other}, {other, changed}} {
			if tree, conflicts := mergeTree(t, repo, sides[0], sides[1]); conflicts != nil || tree != want {
				t.Errorf("MergeTree(%s, %s) = %s, %q, want %s and no conflicts", sides[0], sides[1], tree, conflicts, want)
			}
		}
	})

	t.Run("the same change on both sides merges cleanly", func(t *testing.T) {
		s := newGit(t)
		repo := s.Git.Open(s.Dir)
		base := s.Commit(git.Change{Files: map[string]string{"a.txt": "one\n", "b.txt": "b\n"}})
		ours := s.Commit(git.Change{Parents: []string{base}, Files: map[string]string{"a.txt": "two\nlines\n", "c.txt": "new\n"}, Message: "Ours"})
		theirs := s.Commit(git.Change{Parents: []string{base}, Files: map[string]string{"a.txt": "two\nlines\n", "c.txt": "new\n"}, Message: "Theirs"})

		tree, conflicts := mergeTree(t, repo, ours, theirs)
		if want := must(repo.TreeOf(ours))(t); conflicts != nil || tree != want {
			t.Errorf("MergeTree = %s, %q, want %s and no conflicts", tree, conflicts, want)
		}
	})

	t.Run("changes to different files on each side merge cleanly into both", func(t *testing.T) {
		s := newGit(t)
		repo := s.Git.Open(s.Dir)
		base := s.Commit(git.Change{Files: map[string]string{"a.txt": "one\n", "b.txt": "b\n", "c.txt": "c\n"}})
		ours := s.Commit(git.Change{Parents: []string{base}, Files: map[string]string{"a.txt": "ours\n", "d/new.txt": "new\n"}})
		theirs := s.Commit(git.Change{Parents: []string{base}, Files: map[string]string{"b.txt": "theirs\n"}, Removed: []string{"c.txt"}})
		both := s.Commit(git.Change{Parents: []string{ours}, Files: map[string]string{"b.txt": "theirs\n"}, Removed: []string{"c.txt"}})

		tree, conflicts := mergeTree(t, repo, ours, theirs)
		if want := must(repo.TreeOf(both))(t); conflicts != nil || tree != want {
			t.Errorf("MergeTree = %s, %q, want %s and no conflicts", tree, conflicts, want)
		}
	})

	t.Run("a commit replays onto a base with only its own change, from its parent", func(t *testing.T) {
		s := newGit(t)
		repo := s.Git.Open(s.Dir)
		fork := s.Commit(git.Change{Files: map[string]string{"a.txt": "one\n", "b.txt": "b\n"}})
		earlier := s.Commit(git.Change{Parents: []string{fork}, Files: map[string]string{"b.txt": "branch\n"}})
		commit := s.Commit(git.Change{Parents: []string{earlier}, Files: map[string]string{"a.txt": "commit\n"}})
		base := s.Commit(git.Change{Parents: []string{fork}, Files: map[string]string{"b.txt": "base\n"}})
		want := s.Commit(git.Change{Parents: []string{base}, Files: map[string]string{"a.txt": "commit\n"}})

		tree, conflicts, err := repo.ReplayTree(base, commit)
		ok(t, err)
		if want := must(repo.TreeOf(want))(t); conflicts != nil || tree != want {
			t.Errorf("ReplayTree = %s, %q, want %s and no conflicts", tree, conflicts, want)
		}
	})

	t.Run("a commit replayed onto a base that changed its line conflicts", func(t *testing.T) {
		s := newGit(t)
		repo := s.Git.Open(s.Dir)
		fork := s.Commit(git.Change{Files: map[string]string{"a.txt": "one\n", "b.txt": "b\n"}})
		earlier := s.Commit(git.Change{Parents: []string{fork}, Files: map[string]string{"b.txt": "branch\n"}})
		commit := s.Commit(git.Change{Parents: []string{earlier}, Files: map[string]string{"a.txt": "commit\n"}})
		base := s.Commit(git.Change{Parents: []string{fork}, Files: map[string]string{"a.txt": "base\n"}})

		_, conflicts, err := repo.ReplayTree(base, commit)
		ok(t, err)
		if !slices.Equal(conflicts, []string{"a.txt"}) {
			t.Errorf("conflicts = %q, want a.txt", conflicts)
		}
	})

	t.Run("a root commit doesn't replay", func(t *testing.T) {
		s := newGit(t)
		repo := s.Git.Open(s.Dir)
		root := s.Commit(git.Change{Files: map[string]string{"a.txt": "one\n"}})

		if tree, _, err := repo.ReplayTree(must(repo.Commit("main"))(t), root); err == nil {
			t.Errorf("ReplayTree of a root commit = %s, want an error", tree)
		}
	})

	t.Run("a fast-forward moves the checked-out branch and its files, keeping changes it doesn't touch", func(t *testing.T) {
		s := newGit(t)
		repo := s.Git.Open(s.Dir)
		first := must(repo.Commit("main"))(t)
		next := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n", "d/b.txt": "b\n"}})
		last := s.Commit(git.Change{Parents: []string{next}, Files: map[string]string{"a.txt": "two\n"}, Removed: []string{"d/b.txt"}})
		write(t, s.Dir, "README.md", "Changed\n")
		write(t, s.Dir, "u.txt", "untracked\n")

		fastForward(t, repo, s.Dir, next)
		checkFolder(t, s.Dir, map[string]string{"README.md": "Changed\n", "u.txt": "untracked\n", "a.txt": "one\n", "d/b.txt": "b\n"})
		fastForward(t, repo, s.Dir, last)
		fastForward(t, repo, s.Dir, first)

		checkBranch(t, repo, "main", last)
		checkFolder(t, s.Dir, map[string]string{"README.md": "Changed\n", "u.txt": "untracked\n", "a.txt": "two\n"})
	})

	t.Run("a fast-forward is refused when the branch has diverged, and moves nothing", func(t *testing.T) {
		s := newGit(t)
		repo := s.Git.Open(s.Dir)
		first := must(repo.Commit("main"))(t)
		ours := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "ours\n"}})
		theirs := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "theirs\n"}})
		fastForward(t, repo, s.Dir, ours)

		if out, err := repo.FastForward(s.Dir, theirs); err == nil {
			t.Errorf("FastForward to a diverged commit = %q, want an error", out)
		}
		checkBranch(t, repo, "main", ours)
		checkFolder(t, s.Dir, map[string]string{"README.md": "Hello\n", "a.txt": "ours\n"})
	})

	t.Run("a fast-forward stopped by changes in its way moves nothing", func(t *testing.T) {
		// A change to a file it changes, and an untracked file where it
		// adds one.
		for _, path := range []string{"README.md", "a.txt"} {
			s := newGit(t)
			repo := s.Git.Open(s.Dir)
			first := must(repo.Commit("main"))(t)
			next := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"README.md": "Bye\n", "a.txt": "one\n"}})
			write(t, s.Dir, path, "in the way\n")
			want := map[string]string{"README.md": "Hello\n", path: "in the way\n"}

			if out, err := repo.FastForward(s.Dir, next); err == nil {
				t.Errorf("FastForward with %s in the way = %q, want an error", path, out)
			}
			checkBranch(t, repo, "main", first)
			checkFolder(t, s.Dir, want)
		}
	})

	t.Run("a reset moves the branch and its files, keeping changes it doesn't touch", func(t *testing.T) {
		s := newGit(t)
		repo := s.Git.Open(s.Dir)
		first := must(repo.Commit("main"))(t)
		next := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n", "d/b.txt": "b\n"}})
		fastForward(t, repo, s.Dir, next)
		write(t, s.Dir, "README.md", "Changed\n")
		write(t, s.Dir, "u.txt", "untracked\n")

		if out, err := repo.Reset(s.Dir, first, next); err != nil {
			t.Fatalf("Reset: %v: %s", err, out)
		}
		checkBranch(t, repo, "main", first)
		checkFolder(t, s.Dir, map[string]string{"README.md": "Changed\n", "u.txt": "untracked\n"})
	})

	t.Run("a reset stopped by changes in its way moves nothing", func(t *testing.T) {
		s := newGit(t)
		repo := s.Git.Open(s.Dir)
		first := must(repo.Commit("main"))(t)
		next := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n"}})
		fastForward(t, repo, s.Dir, next)
		write(t, s.Dir, "a.txt", "changed\n")

		if out, err := repo.Reset(s.Dir, first, next); err == nil {
			t.Errorf("Reset with a.txt changed = %q, want an error", out)
		}
		checkBranch(t, repo, "main", next)
		checkFolder(t, s.Dir, map[string]string{"README.md": "Hello\n", "a.txt": "changed\n"})
	})

	t.Run("a reset puts back a branch that moved since the caller looked", func(t *testing.T) {
		s := newGit(t)
		repo := s.Git.Open(s.Dir)
		first := must(repo.Commit("main"))(t)
		moved := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n"}})
		other := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"b.txt": "b\n"}})
		fastForward(t, repo, s.Dir, moved)

		if out, err := repo.Reset(s.Dir, other, first); err == nil {
			t.Errorf("Reset of a branch that moved = %q, want an error", out)
		}
		checkBranch(t, repo, "main", moved)
		checkFolder(t, s.Dir, map[string]string{"README.md": "Hello\n", "a.txt": "one\n"})
	})

	t.Run("a diverged branch's merge base is where it forked", func(t *testing.T) {
		s := newGit(t)
		repo := s.Git.Open(s.Dir)
		first := must(repo.Commit("main"))(t)
		fork := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n"}})
		ours := s.Commit(git.Change{Parents: []string{fork}, Files: map[string]string{"b.txt": "ours\n"}})
		theirs := s.Commit(git.Change{Parents: []string{fork}, Files: map[string]string{"c.txt": "theirs\n"}})
		ok(t, repo.UpdateRef("refs/heads/topic", theirs, ""))

		if got := must(repo.MergeBase(ours, "topic"))(t); got != fork {
			t.Errorf("MergeBase = %s, want %s", got, fork)
		}
		for _, c := range []struct {
			commit, rev string
			want        bool
		}{{first, ours, true}, {fork, "topic", true}, {ours, ours, true}, {ours, theirs, false}, {theirs, ours, false}, {ours, fork, false}} {
			if got := repo.IsAncestor(c.commit, c.rev); got != c.want {
				t.Errorf("IsAncestor(%s, %s) = %t, want %t", c.commit, c.rev, got, c.want)
			}
		}
	})

	t.Run("histories with no common ancestor have no merge base, and don't merge", func(t *testing.T) {
		s := newGit(t)
		repo := s.Git.Open(s.Dir)
		unrelated := s.Commit(git.Change{Files: map[string]string{"a.txt": "one\n"}})

		if got, err := repo.MergeBase("main", unrelated); err == nil {
			t.Errorf("MergeBase of unrelated histories = %s, want an error", got)
		}
		if tree, _, err := repo.MergeTree("main", unrelated); err == nil {
			t.Errorf("MergeTree of unrelated histories = %s, want an error", tree)
		}
	})

	t.Run("updateRef moves a ref only from the old value it is given", func(t *testing.T) {
		s := newGit(t)
		repo := s.Git.Open(s.Dir)
		first := must(repo.Commit("main"))(t)
		one := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n"}})
		two := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "two\n"}})
		ok(t, repo.UpdateRef("refs/heads/topic", one, ""))

		if err := repo.UpdateRef("refs/heads/topic", two, ""); err == nil {
			t.Error("UpdateRef of an existing ref, as new, succeeded, want an error")
		}
		if err := repo.UpdateRef("refs/heads/topic", two, first); err == nil {
			t.Error("UpdateRef from the wrong old value succeeded, want an error")
		}
		if err := repo.UpdateRef("refs/heads/missing", two, first); err == nil {
			t.Error("UpdateRef of a missing ref from an old value succeeded, want an error")
		}
		checkBranch(t, repo, "topic", one)
		ok(t, repo.UpdateRef("refs/heads/topic", two, one))
		checkBranch(t, repo, "topic", two)
	})

	t.Run("deleteRef deletes a ref only at the old value it is given", func(t *testing.T) {
		s := newGit(t)
		repo := s.Git.Open(s.Dir)
		first := must(repo.Commit("main"))(t)
		one := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n"}})
		ok(t, repo.UpdateRef("refs/code-central/fork/x", one, ""))

		if err := repo.DeleteRef("refs/code-central/fork/x", first); err == nil {
			t.Error("DeleteRef at the wrong old value succeeded, want an error")
		}
		if err := repo.DeleteRef("refs/code-central/fork/missing", first); err == nil {
			t.Error("DeleteRef of a missing ref succeeded, want an error")
		}
		checkBranch(t, repo, "refs/code-central/fork/x", one)
		ok(t, repo.DeleteRef("refs/code-central/fork/x", one))
		if got, err := repo.Commit("refs/code-central/fork/x"); err == nil {
			t.Errorf("the deleted ref = %s, want an error", got)
		}
	})

	t.Run("createBranch makes a branch only where there is none", func(t *testing.T) {
		s := newGit(t)
		repo := s.Git.Open(s.Dir)
		first := must(repo.Commit("main"))(t)
		one := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n"}})

		ok(t, repo.CreateBranch("topic", one))
		if err := repo.CreateBranch("topic", first); err == nil {
			t.Error("CreateBranch of an existing branch succeeded, want an error")
		}
		checkBranch(t, repo, "topic", one)
	})

	t.Run("firstParents is the first-parent line from a commit back to what another reaches, oldest first", func(t *testing.T) {
		s := newGit(t)
		repo := s.Git.Open(s.Dir)
		first := must(repo.Commit("main"))(t)
		side := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"s.txt": "side\n"}})
		one := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n"}})
		merge := s.Commit(git.Change{Parents: []string{one, side}, Files: map[string]string{"s.txt": "side\n"}})
		last := s.Commit(git.Change{Parents: []string{merge}, Files: map[string]string{"a.txt": "last\n"}})
		ok(t, repo.UpdateRef("refs/heads/topic", last, ""))

		for _, c := range []struct {
			exclude string
			want    []string
		}{{first, []string{one, merge, last}}, {side, []string{one, merge, last}}, {one, []string{merge, last}}, {last, nil}} {
			if got := must(repo.FirstParents("topic", c.exclude))(t); !slices.Equal(got, c.want) {
				t.Errorf("FirstParents(topic, %s) = %q, want %q", c.exclude, got, c.want)
			}
		}
		if got := must(repo.ParentsOf(merge))(t); !slices.Equal(got, []string{one, side}) {
			t.Errorf("ParentsOf the merge = %q, want %s and %s", got, one, side)
		}
		if got := must(repo.ParentsOf(first))(t); len(got) != 0 {
			t.Errorf("ParentsOf a root commit = %q, want none", got)
		}
	})

	t.Run("commitTreeLike keeps the author and message of the commit it is like", func(t *testing.T) {
		s := newGit(t)
		repo := s.Git.Open(s.Dir)
		first := must(repo.Commit("main"))(t)
		msg := "Add a.txt\n\nWith a body\nof two lines."
		like := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n"}, Message: msg, Author: "Ada Lovelace <ada@example.com>"})
		plain := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n"}, Message: msg})
		tree := must(repo.TreeOf(like))(t)

		if got := must(repo.CommitTreeLike(tree, like, first))(t); got != like {
			t.Errorf("CommitTreeLike = %s, want %s, the commit it is like", got, like)
		}
		if got := must(repo.CommitTree(tree, msg, first))(t); got != plain {
			t.Errorf("CommitTree = %s, want %s, made by the default author", got, plain)
		}
		other := s.Commit(git.Change{Files: map[string]string{"b.txt": "b\n"}})
		made := must(repo.CommitTreeLike(must(repo.TreeOf(other))(t), like, like))(t)
		if got := must(repo.ParentsOf(made))(t); !slices.Equal(got, []string{like}) {
			t.Errorf("the commit's parents = %q, want %s", got, like)
		}
		if got := must(repo.TreeOf(made))(t); got != must(repo.TreeOf(other))(t) {
			t.Errorf("the commit's tree = %s, want %s's", got, other)
		}
	})

	t.Run("specBranches are the branches under spec/, and forkPoints the refs under the fork refs", func(t *testing.T) {
		s := newGit(t)
		repo := s.Git.Open(s.Dir)
		first := must(repo.Commit("main"))(t)

		if got := must(repo.SpecBranches())(t); len(got) != 0 {
			t.Errorf("SpecBranches = %q, want none", got)
		}
		if got := must(repo.ForkPoints())(t); len(got) != 0 {
			t.Errorf("ForkPoints = %q, want none", got)
		}
		for _, ref := range []string{"refs/heads/spec/b", "refs/heads/spec/a", "refs/heads/specs", "refs/heads/topic", git.ForkRefs + "y", git.ForkRefs + "x", "refs/code-central/other"} {
			ok(t, repo.UpdateRef(ref, first, ""))
		}
		if got := must(repo.SpecBranches())(t); !slices.Equal(got, []string{"spec/a", "spec/b"}) {
			t.Errorf("SpecBranches = %q, want spec/a and spec/b", got)
		}
		if got := must(repo.ForkPoints())(t); !slices.Equal(got, []string{"x", "y"}) {
			t.Errorf("ForkPoints = %q, want x and y", got)
		}
	})

	t.Run("hasFile reports whether a commit holds a file", func(t *testing.T) {
		s := newGit(t)
		repo := s.Git.Open(s.Dir)
		first := must(repo.Commit("main"))(t)
		one := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"d/a.txt": "one\n"}})

		for _, c := range []struct {
			rev, path string
			want      bool
		}{{one, "d/a.txt", true}, {one, "README.md", true}, {"main", "d/a.txt", false}, {one, "b.txt", false}} {
			if got := repo.HasFile(c.rev, c.path); got != c.want {
				t.Errorf("HasFile(%s, %s) = %t, want %t", c.rev, c.path, got, c.want)
			}
		}
	})
}

// fastForward fast-forwards the branch checked out at dir to commit,
// failing t when it can't.
func fastForward(t *testing.T, repo git.Repo, dir, commit string) {
	t.Helper()
	if out, err := repo.FastForward(dir, commit); err != nil {
		t.Fatalf("FastForward(%s): %v: %s", commit, err, out)
	}
}

// checkBranch fails t unless branch is at commit.
func checkBranch(t *testing.T, repo git.Repo, branch, commit string) {
	t.Helper()
	if got, err := repo.Commit(branch); err != nil || got != commit {
		t.Errorf("%s = %s, %v, want %s", branch, got, err, commit)
	}
}

// write writes content to the file at the slash-separated path in dir.
func write(t *testing.T, dir, path, content string) {
	t.Helper()
	name := filepath.Join(dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// checkFolder fails t unless dir holds these files, by slash-separated
// path, and no others, git's own aside.
func checkFolder(t *testing.T, dir string, want map[string]string) {
	t.Helper()
	got := map[string]string{}
	err := filepath.WalkDir(dir, func(name string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && d.Name() == ".git":
			return filepath.SkipDir
		case d.IsDir():
			return nil
		}
		content, err := os.ReadFile(name)
		rel, _ := filepath.Rel(dir, name)
		got[filepath.ToSlash(rel)] = string(content)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(got, want) {
		t.Errorf("the folder holds %q, want %q", got, want)
	}
}

// mergeTree is MergeTree of base and head, failing t on an error.
func mergeTree(t *testing.T, repo git.Repo, base, head string) (string, []string) {
	t.Helper()
	tree, conflicts, err := repo.MergeTree(base, head)
	if err != nil {
		t.Fatalf("MergeTree(%s, %s): %v", base, head, err)
	}
	return tree, conflicts
}

// must is the result of a call that has to succeed:
// must(repo.Commit(rev))(t) fails t on Commit's error.
func must[T any](v T, err error) func(t *testing.T) T {
	return func(t *testing.T) T {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
}

// ok fails t on err.
func ok(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
