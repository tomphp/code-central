// Package gittest holds the Git contract suite every git.Git
// implementation must pass, so that the Fake git behaves as real git does
// wherever the Factory relies on it. Run it from each implementation's
// tests with a function that sets up a fresh repository.
package gittest

import (
	"errors"
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
	// CommitAll commits what the worktree dir's folder holds, untracked
	// files included, onto what is checked out there, with the message
	// msg, as `git add -A` then `git commit` do, and returns the commit's
	// ID. It fails when there is nothing to commit.
	CommitAll func(dir, msg string) (string, error)
	// Remote is the folder of an empty bare repository, with main its
	// HEAD, that the repository at Dir has as its remote origin.
	Remote string
}

// Run runs the contract suite, with newGit setting up each case's git.
// The cases run in parallel.
func Run(t *testing.T, newGit func(t *testing.T) Setup) {
	// open sets up a case's git, running the case in parallel with the
	// others, and opens its repository, returning it with the commit
	// main is at.
	open := func(t *testing.T) (Setup, git.Repo, string) {
		t.Parallel()
		s := newGit(t)
		repo := s.Git.Open(s.Dir)
		return s, repo, must(repo.Commit("main"))(t)
	}
	remoteCases(t, open)

	t.Run("a commit is found by its branch, its full ref, its ID and its short ID", func(t *testing.T) {
		s, repo, first := open(t)
		c := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n"}})
		noError(t, repo.UpdateRef("refs/heads/topic", c, ""))

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
		s, repo, _ := open(t)
		base := s.Commit(git.Change{Files: map[string]string{"a.txt": "one\n", "b.txt": "b\n"}})
		ours := s.Commit(git.Change{Parents: []string{base}, Files: map[string]string{"a.txt": "ours\n"}})
		theirs := s.Commit(git.Change{Parents: []string{base}, Files: map[string]string{"a.txt": "theirs\n", "b.txt": "theirs\n"}})

		checkConflicts(t, repo, ours, theirs)
	})

	t.Run("a file added on both sides with different lines conflicts on merge", func(t *testing.T) {
		s, repo, _ := open(t)
		base := s.Commit(git.Change{Files: map[string]string{"b.txt": "b\n"}})
		ours := s.Commit(git.Change{Parents: []string{base}, Files: map[string]string{"a.txt": "ours\n"}})
		theirs := s.Commit(git.Change{Parents: []string{base}, Files: map[string]string{"a.txt": "theirs\n"}})

		checkConflicts(t, repo, ours, theirs)
	})

	t.Run("a file changed on one side and deleted on the other conflicts on merge", func(t *testing.T) {
		s, repo, _ := open(t)
		base := s.Commit(git.Change{Files: map[string]string{"a.txt": "one\n", "b.txt": "b\n"}})
		ours := s.Commit(git.Change{Parents: []string{base}, Files: map[string]string{"a.txt": "ours\n"}})
		theirs := s.Commit(git.Change{Parents: []string{base}, Removed: []string{"a.txt"}})

		checkConflicts(t, repo, ours, theirs)
	})

	t.Run("a change on one side merges cleanly, whichever side made it", func(t *testing.T) {
		s, repo, _ := open(t)
		base := s.Commit(git.Change{Files: map[string]string{"a.txt": "one\n", "b.txt": "b\n"}})
		changed := s.Commit(git.Change{Parents: []string{base}, Files: map[string]string{"a.txt": "two\nlines\n"}, Removed: []string{"b.txt"}})
		other := s.Commit(git.Change{Parents: []string{base}, Message: "Nothing changed"})

		checkMerge(t, repo, changed, other, changed)
		checkMerge(t, repo, other, changed, changed)
	})

	t.Run("the same change on both sides merges cleanly", func(t *testing.T) {
		s, repo, _ := open(t)
		base := s.Commit(git.Change{Files: map[string]string{"a.txt": "one\n", "b.txt": "b\n"}})
		ours := s.Commit(git.Change{Parents: []string{base}, Files: map[string]string{"a.txt": "two\nlines\n", "c.txt": "new\n"}, Message: "Ours"})
		theirs := s.Commit(git.Change{Parents: []string{base}, Files: map[string]string{"a.txt": "two\nlines\n", "c.txt": "new\n"}, Message: "Theirs"})

		checkMerge(t, repo, ours, theirs, ours)
	})

	t.Run("changes to different files on each side merge cleanly into both", func(t *testing.T) {
		s, repo, _ := open(t)
		base := s.Commit(git.Change{Files: map[string]string{"a.txt": "one\n", "b.txt": "b\n", "c.txt": "c\n"}})
		ours := s.Commit(git.Change{Parents: []string{base}, Files: map[string]string{"a.txt": "ours\n", "d/new.txt": "new\n"}})
		theirs := s.Commit(git.Change{Parents: []string{base}, Files: map[string]string{"b.txt": "theirs\n"}, Removed: []string{"c.txt"}})
		both := s.Commit(git.Change{Parents: []string{ours}, Files: map[string]string{"b.txt": "theirs\n"}, Removed: []string{"c.txt"}})

		checkMerge(t, repo, ours, theirs, both)
	})

	t.Run("a commit replays onto a base with only its own change, from its parent", func(t *testing.T) {
		s, repo, _ := open(t)
		fork := s.Commit(git.Change{Files: map[string]string{"a.txt": "one\n", "b.txt": "b\n"}})
		earlier := s.Commit(git.Change{Parents: []string{fork}, Files: map[string]string{"b.txt": "branch\n"}})
		commit := s.Commit(git.Change{Parents: []string{earlier}, Files: map[string]string{"a.txt": "commit\n"}})
		base := s.Commit(git.Change{Parents: []string{fork}, Files: map[string]string{"b.txt": "base\n"}})
		want := s.Commit(git.Change{Parents: []string{base}, Files: map[string]string{"a.txt": "commit\n"}})

		tree, conflicts, err := repo.ReplayTree(base, commit)
		noError(t, err)
		if want := must(repo.TreeOf(want))(t); conflicts != nil || tree != want {
			t.Errorf("ReplayTree = %s, %q, want %s and no conflicts", tree, conflicts, want)
		}
	})

	t.Run("a commit replayed onto a base that changed its line conflicts", func(t *testing.T) {
		s, repo, _ := open(t)
		fork := s.Commit(git.Change{Files: map[string]string{"a.txt": "one\n", "b.txt": "b\n"}})
		earlier := s.Commit(git.Change{Parents: []string{fork}, Files: map[string]string{"b.txt": "branch\n"}})
		commit := s.Commit(git.Change{Parents: []string{earlier}, Files: map[string]string{"a.txt": "commit\n"}})
		base := s.Commit(git.Change{Parents: []string{fork}, Files: map[string]string{"a.txt": "base\n"}})

		_, conflicts, err := repo.ReplayTree(base, commit)
		noError(t, err)
		if !slices.Equal(conflicts, []string{"a.txt"}) {
			t.Errorf("conflicts = %q, want a.txt", conflicts)
		}
	})

	t.Run("a root commit doesn't replay", func(t *testing.T) {
		s, repo, first := open(t)
		root := s.Commit(git.Change{Files: map[string]string{"a.txt": "one\n"}})

		if tree, _, err := repo.ReplayTree(first, root); err == nil {
			t.Errorf("ReplayTree of a root commit = %s, want an error", tree)
		}
	})

	t.Run("a fast-forward moves the checked-out branch and its files, keeping changes it doesn't touch", func(t *testing.T) {
		s, repo, first := open(t)
		next := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n", "d/b.txt": "b\n"}})
		last := s.Commit(git.Change{Parents: []string{next}, Files: map[string]string{"a.txt": "two\n"}, Removed: []string{"d/b.txt"}})
		writeFile(t, s.Dir, "README.md", "Changed\n")
		writeFile(t, s.Dir, "u.txt", "untracked\n")

		fastForward(t, repo, s.Dir, next)
		checkFolder(t, s.Dir, map[string]string{"README.md": "Changed\n", "u.txt": "untracked\n", "a.txt": "one\n", "d/b.txt": "b\n"})
		fastForward(t, repo, s.Dir, last)
		fastForward(t, repo, s.Dir, first)

		checkBranch(t, repo, "main", last)
		checkFolder(t, s.Dir, map[string]string{"README.md": "Changed\n", "u.txt": "untracked\n", "a.txt": "two\n"})
	})

	t.Run("a fast-forward is refused when the branch has diverged, and moves nothing", func(t *testing.T) {
		s, repo, first := open(t)
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
		t.Parallel()
		// A change to a file it changes, and an untracked file where it
		// adds one.
		for _, path := range []string{"README.md", "a.txt"} {
			t.Run(path, func(t *testing.T) {
				s, repo, first := open(t)
				next := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"README.md": "Bye\n", "a.txt": "one\n"}})
				writeFile(t, s.Dir, path, "in the way\n")
				want := map[string]string{"README.md": "Hello\n", path: "in the way\n"}

				if out, err := repo.FastForward(s.Dir, next); err == nil {
					t.Errorf("FastForward with %s in the way = %q, want an error", path, out)
				}
				checkBranch(t, repo, "main", first)
				checkFolder(t, s.Dir, want)
			})
		}
	})

	t.Run("a reset moves the branch and its files, keeping changes it doesn't touch", func(t *testing.T) {
		s, repo, first := open(t)
		next := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n", "d/b.txt": "b\n"}})
		fastForward(t, repo, s.Dir, next)
		writeFile(t, s.Dir, "README.md", "Changed\n")
		writeFile(t, s.Dir, "u.txt", "untracked\n")

		if out, err := repo.Reset(s.Dir, first, next); err != nil {
			t.Fatalf("Reset: %v: %s", err, out)
		}
		checkBranch(t, repo, "main", first)
		checkFolder(t, s.Dir, map[string]string{"README.md": "Changed\n", "u.txt": "untracked\n"})
	})

	t.Run("a reset stopped by changes in its way moves nothing", func(t *testing.T) {
		s, repo, first := open(t)
		next := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n"}})
		fastForward(t, repo, s.Dir, next)
		writeFile(t, s.Dir, "a.txt", "changed\n")

		if out, err := repo.Reset(s.Dir, first, next); err == nil {
			t.Errorf("Reset with a.txt changed = %q, want an error", out)
		}
		checkBranch(t, repo, "main", next)
		checkFolder(t, s.Dir, map[string]string{"README.md": "Hello\n", "a.txt": "changed\n"})
	})

	t.Run("a reset puts back a branch that moved since the caller looked", func(t *testing.T) {
		s, repo, first := open(t)
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
		s, repo, first := open(t)
		fork := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n"}})
		ours := s.Commit(git.Change{Parents: []string{fork}, Files: map[string]string{"b.txt": "ours\n"}})
		theirs := s.Commit(git.Change{Parents: []string{fork}, Files: map[string]string{"c.txt": "theirs\n"}})
		noError(t, repo.UpdateRef("refs/heads/topic", theirs, ""))

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
		s, repo, _ := open(t)
		unrelated := s.Commit(git.Change{Files: map[string]string{"a.txt": "one\n"}})

		if got, err := repo.MergeBase("main", unrelated); err == nil {
			t.Errorf("MergeBase of unrelated histories = %s, want an error", got)
		}
		if tree, _, err := repo.MergeTree("main", unrelated); err == nil {
			t.Errorf("MergeTree of unrelated histories = %s, want an error", tree)
		}
	})

	t.Run("updateRef moves a ref only from the old value it is given", func(t *testing.T) {
		s, repo, first := open(t)
		one := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n"}})
		two := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "two\n"}})
		noError(t, repo.UpdateRef("refs/heads/topic", one, ""))

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
		noError(t, repo.UpdateRef("refs/heads/topic", two, one))
		checkBranch(t, repo, "topic", two)
	})

	t.Run("deleteRef deletes a ref only at the old value it is given", func(t *testing.T) {
		s, repo, first := open(t)
		one := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n"}})
		noError(t, repo.UpdateRef("refs/code-central/fork/x", one, ""))

		if err := repo.DeleteRef("refs/code-central/fork/x", first); err == nil {
			t.Error("DeleteRef at the wrong old value succeeded, want an error")
		}
		if err := repo.DeleteRef("refs/code-central/fork/missing", first); err == nil {
			t.Error("DeleteRef of a missing ref succeeded, want an error")
		}
		checkBranch(t, repo, "refs/code-central/fork/x", one)
		noError(t, repo.DeleteRef("refs/code-central/fork/x", one))
		if got, err := repo.Commit("refs/code-central/fork/x"); err == nil {
			t.Errorf("the deleted ref = %s, want an error", got)
		}
	})

	t.Run("createBranch makes a branch only where there is none", func(t *testing.T) {
		s, repo, first := open(t)
		one := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n"}})

		noError(t, repo.CreateBranch("topic", one))
		if err := repo.CreateBranch("topic", first); err == nil {
			t.Error("CreateBranch of an existing branch succeeded, want an error")
		}
		checkBranch(t, repo, "topic", one)
	})

	t.Run("firstParents is the first-parent line from a commit back to what another reaches, oldest first", func(t *testing.T) {
		s, repo, first := open(t)
		side := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"s.txt": "side\n"}})
		one := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n"}})
		merge := s.Commit(git.Change{Parents: []string{one, side}, Files: map[string]string{"s.txt": "side\n"}})
		last := s.Commit(git.Change{Parents: []string{merge}, Files: map[string]string{"a.txt": "last\n"}})
		noError(t, repo.UpdateRef("refs/heads/topic", last, ""))

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
		s, repo, first := open(t)
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
		_, repo, first := open(t)

		if got := must(repo.SpecBranches())(t); len(got) != 0 {
			t.Errorf("SpecBranches = %q, want none", got)
		}
		if got := must(repo.ForkPoints())(t); len(got) != 0 {
			t.Errorf("ForkPoints = %q, want none", got)
		}
		for _, ref := range []string{"refs/heads/spec/b", "refs/heads/spec/a", "refs/heads/specs", "refs/heads/topic", git.ForkRefs + "y", git.ForkRefs + "x", "refs/code-central/other"} {
			noError(t, repo.UpdateRef(ref, first, ""))
		}
		if got := must(repo.SpecBranches())(t); !slices.Equal(got, []string{"spec/a", "spec/b"}) {
			t.Errorf("SpecBranches = %q, want spec/a and spec/b", got)
		}
		if got := must(repo.ForkPoints())(t); !slices.Equal(got, []string{"x", "y"}) {
			t.Errorf("ForkPoints = %q, want x and y", got)
		}
	})

	t.Run("hasFile reports whether a commit holds a file", func(t *testing.T) {
		s, repo, first := open(t)
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

	t.Run("a worktree added on a new branch holds its commit's files, on that branch", func(t *testing.T) {
		s, repo, first := open(t)
		next := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n", "d/b.txt": "b\n"}})
		wt := filepath.Join(t.TempDir(), "topic")

		noError(t, repo.AddWorktreeOnNewBranch(wt, "topic", next))
		checkBranch(t, repo, "topic", next)
		checkBranch(t, repo, "main", first)
		checkFolder(t, wt, map[string]string{"README.md": "Hello\n", "a.txt": "one\n", "d/b.txt": "b\n"})
		checkWorktrees(t, repo, git.Worktree{Path: s.Dir, Branch: "main"}, git.Worktree{Path: wt, Branch: "topic"})
		if got := must(repo.CurrentBranch(wt))(t); got != "topic" {
			t.Errorf("CurrentBranch = %q, want topic", got)
		}
		checkStatus(t, repo, wt, "")
		again := filepath.Join(t.TempDir(), "again")
		if err := repo.AddWorktreeOnNewBranch(again, "topic", first); err == nil {
			t.Error("AddWorktreeOnNewBranch on an existing branch succeeded, want an error")
		}
		checkNoFolder(t, again)
		checkBranch(t, repo, "topic", next)
		// git makes the branch before it looks at the folder.
		if err := repo.AddWorktreeOnNewBranch(wt, "other", first); err == nil {
			t.Error("AddWorktreeOnNewBranch in a worktree's folder succeeded, want an error")
		}
		checkBranch(t, repo, "other", first)
	})

	t.Run("the top level in a worktree is the worktree's folder, from it or a folder in it", func(t *testing.T) {
		s, repo, first := open(t)
		wt := filepath.Join(t.TempDir(), "topic")
		noError(t, repo.AddWorktreeOnNewBranch(wt, "topic", first))
		sub := filepath.Join(wt, "d")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}

		for _, dir := range []string{wt, sub} {
			if top := must(s.Git.TopLevel(dir))(t); !samePath(top, wt) {
				t.Errorf("TopLevel(%s) = %s, want %s", dir, top, wt)
			}
		}
		if top := must(s.Git.Open(wt).TopLevel())(t); !samePath(top, wt) {
			t.Errorf("the worktree's TopLevel = %s, want %s", top, wt)
		}
	})

	t.Run("a worktree added on an existing branch holds its files, and none is added where it can't be", func(t *testing.T) {
		s, repo, first := open(t)
		next := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n"}})
		noError(t, repo.CreateBranch("topic", next))
		noError(t, repo.CreateBranch("other", first))
		parent := t.TempDir()
		wt := filepath.Join(parent, "a")

		noError(t, repo.AddWorktree(wt, "topic"))
		checkFolder(t, wt, map[string]string{"README.md": "Hello\n", "a.txt": "one\n"})
		// A branch checked out in a worktree, or in the repository's own
		// folder, and one that doesn't exist.
		for _, c := range []struct{ name, branch string }{{"b", "topic"}, {"c", "main"}, {"d", "missing"}} {
			path := filepath.Join(parent, c.name)
			if err := repo.AddWorktree(path, c.branch); err == nil {
				t.Errorf("AddWorktree on %s succeeded, want an error", c.branch)
			}
			checkNoFolder(t, path)
		}
		stray := filepath.Join(parent, "e")
		writeFile(t, stray, "f.txt", "stray\n")
		if err := repo.AddWorktree(stray, "other"); err == nil {
			t.Error("AddWorktree on a folder with files in it succeeded, want an error")
		}
		checkFolder(t, stray, map[string]string{"f.txt": "stray\n"})
		empty := filepath.Join(parent, "f")
		if err := os.Mkdir(empty, 0o755); err != nil {
			t.Fatal(err)
		}
		noError(t, repo.AddWorktree(empty, "other"))
		checkWorktrees(t, repo, git.Worktree{Path: s.Dir, Branch: "main"}, git.Worktree{Path: wt, Branch: "topic"}, git.Worktree{Path: empty, Branch: "other"})
	})

	t.Run("a detached worktree is on no branch, and checks out other commits detached", func(t *testing.T) {
		s, repo, first := open(t)
		next := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n"}, Message: "Add a.txt\nover two lines\n\nWith a body."})
		parent := t.TempDir()
		stage, topic := filepath.Join(parent, "stage"), filepath.Join(parent, "topic")

		noError(t, repo.AddDetachedWorktree(stage, "main"))
		noError(t, repo.CheckoutDetached(stage, next))
		checkFolder(t, stage, map[string]string{"README.md": "Hello\n", "a.txt": "one\n"})
		if got, err := repo.CurrentBranch(stage); err == nil {
			t.Errorf("CurrentBranch of a detached worktree = %q, want an error", got)
		}
		checkLastCommit(t, repo, stage, next, "Add a.txt over two lines")
		checkLastCommit(t, repo, s.Dir, first, "First")
		if got := must(repo.CurrentBranch(s.Dir))(t); got != "main" {
			t.Errorf("CurrentBranch of the repository's folder = %q, want main", got)
		}
		// A worktree on a branch is detached from it, which stays.
		noError(t, repo.AddWorktreeOnNewBranch(topic, "topic", first))
		noError(t, repo.CheckoutDetached(topic, next))
		checkBranch(t, repo, "topic", first)
		checkBranch(t, repo, "main", first)
		checkWorktrees(t, repo, git.Worktree{Path: s.Dir, Branch: "main"}, git.Worktree{Path: stage}, git.Worktree{Path: topic})
	})

	t.Run("a checkout in a worktree keeps changes it doesn't touch, and is stopped by changes in its way", func(t *testing.T) {
		s, repo, first := open(t)
		next := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n", "d/b.txt": "b\n"}})
		other := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "other\n"}})
		wt := filepath.Join(t.TempDir(), "stage")
		noError(t, repo.AddDetachedWorktree(wt, next))
		writeFile(t, wt, "README.md", "Changed\n")
		writeFile(t, wt, "u.txt", "untracked\n")

		noError(t, repo.CheckoutDetached(wt, other))
		checkFolder(t, wt, map[string]string{"README.md": "Changed\n", "u.txt": "untracked\n", "a.txt": "other\n"})
		writeFile(t, wt, "a.txt", "changed\n")
		if err := repo.CheckoutDetached(wt, first); err == nil {
			t.Error("CheckoutDetached with a.txt changed succeeded, want an error")
		}
		checkFolder(t, wt, map[string]string{"README.md": "Changed\n", "u.txt": "untracked\n", "a.txt": "changed\n"})
		checkLastCommit(t, repo, wt, other, "")
	})

	t.Run("status is each changed, deleted and untracked file, and nothing once they are put back", func(t *testing.T) {
		s, repo, first := open(t)
		next := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n", "d/b.txt": "b\n"}})
		wt := filepath.Join(t.TempDir(), "topic")
		noError(t, repo.AddWorktreeOnNewBranch(wt, "topic", next))
		writeFile(t, wt, "README.md", "Changed\n")
		removeFile(t, wt, "a.txt")
		writeFile(t, wt, "e/u.txt", "untracked\n")
		if err := os.Mkdir(filepath.Join(wt, "empty"), 0o755); err != nil {
			t.Fatal(err)
		}

		// git's porcelain status, trimmed: the first line's leading space
		// goes.
		checkStatus(t, repo, wt, "M README.md\n D a.txt\n?? e/u.txt")
		checkStatus(t, repo, s.Dir, "")
		writeFile(t, wt, "README.md", "Hello\n")
		writeFile(t, wt, "a.txt", "one\n")
		removeFile(t, wt, "e/u.txt")
		checkStatus(t, repo, wt, "")
	})

	t.Run("removing a worktree deletes its folder, but not one with changes or untracked files, nor the repository's own", func(t *testing.T) {
		s, repo, first := open(t)
		wt := filepath.Join(t.TempDir(), "topic")
		noError(t, repo.AddWorktreeOnNewBranch(wt, "topic", first))

		writeFile(t, wt, "u.txt", "untracked\n")
		if err := repo.RemoveWorktree(wt); err == nil {
			t.Error("RemoveWorktree with an untracked file succeeded, want an error")
		}
		removeFile(t, wt, "u.txt")
		writeFile(t, wt, "README.md", "Changed\n")
		if err := repo.RemoveWorktree(wt); err == nil {
			t.Error("RemoveWorktree with a changed file succeeded, want an error")
		}
		checkFolder(t, wt, map[string]string{"README.md": "Changed\n"})
		writeFile(t, wt, "README.md", "Hello\n")
		noError(t, repo.RemoveWorktree(wt))
		checkNoFolder(t, wt)
		checkWorktrees(t, repo, git.Worktree{Path: s.Dir, Branch: "main"})
		checkBranch(t, repo, "topic", first)
		if err := repo.RemoveWorktree(s.Dir); err == nil {
			t.Error("RemoveWorktree of the repository's own folder succeeded, want an error")
		}
		if err := repo.RemoveWorktree(filepath.Join(t.TempDir(), "none")); err == nil {
			t.Error("RemoveWorktree of a folder that is no worktree succeeded, want an error")
		}
	})

	t.Run("a worktree whose folder is gone is listed until pruned or removed, and its branch is then free", func(t *testing.T) {
		s, repo, first := open(t)
		parent := t.TempDir()
		a, b, c := filepath.Join(parent, "a"), filepath.Join(parent, "b"), filepath.Join(parent, "c")
		for _, wt := range []string{a, b, c} {
			noError(t, repo.AddWorktreeOnNewBranch(wt, filepath.Base(wt), first))
		}
		for _, wt := range []string{a, b} {
			if err := os.RemoveAll(wt); err != nil {
				t.Fatal(err)
			}
		}

		noError(t, repo.RemoveWorktree(b))
		checkWorktrees(t, repo, git.Worktree{Path: s.Dir, Branch: "main"}, git.Worktree{Path: a, Branch: "a"}, git.Worktree{Path: c, Branch: "c"})
		noError(t, repo.PruneWorktrees())
		checkWorktrees(t, repo, git.Worktree{Path: s.Dir, Branch: "main"}, git.Worktree{Path: c, Branch: "c"})
		noError(t, repo.AddWorktree(a, "a"))
		checkFolder(t, a, map[string]string{"README.md": "Hello\n"})
	})

	t.Run("a commit of a worktree's folder holds what is in it, untracked files included, on what is checked out there", func(t *testing.T) {
		s, repo, first := open(t)
		parent := t.TempDir()
		topic, stage := filepath.Join(parent, "topic"), filepath.Join(parent, "stage")
		noError(t, repo.AddWorktreeOnNewBranch(topic, "topic", first))
		writeFile(t, topic, "a.txt", "one\n")
		writeFile(t, topic, "d/b.txt", "b\n")
		removeFile(t, topic, "README.md")

		made := must(s.CommitAll(topic, "Add a and b"))(t)
		want := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n", "d/b.txt": "b\n"}, Removed: []string{"README.md"}, Message: "Add a and b"})
		if made != want {
			t.Errorf("CommitAll = %s, want %s, the commit of the folder's files", made, want)
		}
		checkBranch(t, repo, "topic", made)
		checkBranch(t, repo, "main", first)
		checkStatus(t, repo, topic, "")
		if got, err := s.CommitAll(topic, "Nothing"); err == nil {
			t.Errorf("CommitAll of a clean worktree = %s, want an error", got)
		}
		writeFile(t, topic, "a.txt", "two\n")
		if got, err := s.CommitAll(topic, ""); err == nil {
			t.Errorf("CommitAll with no message = %s, want an error", got)
		}
		checkBranch(t, repo, "topic", made)

		noError(t, repo.AddDetachedWorktree(stage, "topic"))
		writeFile(t, stage, "a.txt", "two\n")
		again := must(s.CommitAll(stage, "Change a"))(t)
		checkLastCommit(t, repo, stage, again, "Change a")
		if got := must(repo.ParentsOf(again))(t); !slices.Equal(got, []string{made}) {
			t.Errorf("the detached commit's parents = %q, want %s", got, made)
		}
		checkBranch(t, repo, "topic", made)
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

// writeFile writes content to the file at the slash-separated path in
// dir.
func writeFile(t *testing.T, dir, path, content string) {
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
		// A worktree's .git is a file.
		case d.Name() == ".git" && d.IsDir():
			return filepath.SkipDir
		case d.Name() == ".git", d.IsDir():
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

// removeFile removes the file at the slash-separated path in dir.
func removeFile(t *testing.T, dir, path string) {
	t.Helper()
	if err := os.Remove(filepath.Join(dir, filepath.FromSlash(path))); err != nil {
		t.Fatal(err)
	}
}

// checkNoFolder fails t unless there is nothing at path.
func checkNoFolder(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%s is there, want nothing: %v", path, err)
	}
}

// checkWorktrees fails t unless the repository's worktrees are want, in
// order, each path compared once its symbolic links are resolved, as git
// reports it.
func checkWorktrees(t *testing.T, repo git.Repo, want ...git.Worktree) {
	t.Helper()
	got := must(repo.Worktrees())(t)
	resolve := func(list []git.Worktree) []git.Worktree {
		var out []git.Worktree
		for _, w := range list {
			out = append(out, git.Worktree{Path: resolved(t, w.Path), Branch: w.Branch})
		}
		return out
	}
	if got, want := resolve(got), resolve(want); !slices.Equal(got, want) {
		t.Errorf("Worktrees = %+v, want %+v", got, want)
	}
}

// resolved is path with its symbolic links resolved, or, when it is gone,
// its folder's.
func resolved(t *testing.T, path string) string {
	t.Helper()
	if p, err := filepath.EvalSymlinks(path); err == nil {
		return p
	}
	dir := must(filepath.EvalSymlinks(filepath.Dir(path)))(t)
	return filepath.Join(dir, filepath.Base(path))
}

// checkStatus fails t unless the worktree dir's status is want.
func checkStatus(t *testing.T, repo git.Repo, dir, want string) {
	t.Helper()
	if got := must(repo.Status(dir))(t); got != want {
		t.Errorf("Status(%s) = %q, want %q", dir, got, want)
	}
}

// checkLastCommit fails t unless the worktree dir has commit checked out,
// with this subject.
func checkLastCommit(t *testing.T, repo git.Repo, dir, commit, subject string) {
	t.Helper()
	id, got, err := repo.LastCommit(dir)
	if err != nil || id != commit[:7] || got != subject {
		t.Errorf("LastCommit(%s) = %s, %q, %v, want %s, %q", dir, id, got, err, commit[:7], subject)
	}
}

// checkMerge fails t unless merging base and head gives commit's tree,
// with no conflicts.
func checkMerge(t *testing.T, repo git.Repo, base, head, commit string) {
	t.Helper()
	tree, conflicts := mergeTree(t, repo, base, head)
	if want := must(repo.TreeOf(commit))(t); conflicts != nil || tree != want {
		t.Errorf("MergeTree(%s, %s) = %s, %q, want %s and no conflicts", base, head, tree, conflicts, want)
	}
}

// checkConflicts fails t unless merging base and head conflicts on a.txt
// alone.
func checkConflicts(t *testing.T, repo git.Repo, base, head string) {
	t.Helper()
	if _, conflicts := mergeTree(t, repo, base, head); !slices.Equal(conflicts, []string{"a.txt"}) {
		t.Errorf("MergeTree(%s, %s) conflicts on %q, want a.txt", base, head, conflicts)
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

// noError fails t on err.
func noError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
