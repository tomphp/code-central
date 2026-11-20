package gittest

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/tomoram/code-central/factory/git"
)

// scriptCases are the contract cases for the git commands a fake claude's
// Script runs in a worktree (Setup.Script), each setting up its git with
// open.
func scriptCases(t *testing.T, open func(t *testing.T) (Setup, git.Repo, string)) {
	t.Run("a Script's add and commit commit only what was added, onto what is checked out", func(t *testing.T) {
		s, repo, first := open(t)
		writeFile(t, s.Dir, "a.txt", "a\n")
		writeFile(t, s.Dir, "b.txt", "b\n")

		script(t, s, s.Dir, "add", "a.txt")
		script(t, s, s.Dir, "commit", "-q", "-m", "Add a")

		main := must(repo.Commit("main"))(t)
		if got := must(repo.ParentsOf(main))(t); !slices.Equal(got, []string{first}) {
			t.Errorf("the commit's parents = %q, want %s", got, first)
		}
		if !repo.HasFile("main", "a.txt") || repo.HasFile("main", "b.txt") {
			t.Errorf("main has a.txt %v and b.txt %v, want only a.txt", repo.HasFile("main", "a.txt"), repo.HasFile("main", "b.txt"))
		}
		checkLastCommit(t, repo, s.Dir, main, "Add a")
		checkStatus(t, repo, s.Dir, "?? b.txt")
		if got := script(t, s, s.Dir, "rev-parse", "HEAD"); got != main {
			t.Errorf("rev-parse HEAD = %q, want %s", got, main)
		}
	})

	t.Run("a Script's commit takes the message of each -m as a paragraph", func(t *testing.T) {
		s, repo, _ := open(t)
		writeFile(t, s.Dir, "a.txt", "a\n")

		script(t, s, s.Dir, "add", "a.txt")
		script(t, s, s.Dir, "commit", "-q", "-m", "Add a", "-m", "Because.")

		checkLastCommit(t, repo, s.Dir, must(repo.Commit("main"))(t), "Add a")
	})

	t.Run("Status tells what a Script staged from what it didn't", func(t *testing.T) {
		s, repo, _ := open(t)
		writeFile(t, s.Dir, "README.md", "Staged\n")
		script(t, s, s.Dir, "add", "README.md")
		writeFile(t, s.Dir, "README.md", "Not staged\n")
		writeFile(t, s.Dir, "a.txt", "a\n")
		script(t, s, s.Dir, "add", "a.txt")
		writeFile(t, s.Dir, "c.txt", "c\n")

		checkStatus(t, repo, s.Dir, "MM README.md\nA  a.txt\n?? c.txt")

		script(t, s, s.Dir, "commit", "-q", "-m", "Stage")
		if got := must(repo.TreeOf("main"))(t); got != must(repo.TreeOf(s.Commit(git.Change{Files: map[string]string{"README.md": "Staged\n", "a.txt": "a\n"}})))(t) {
			t.Errorf("the commit's tree %s isn't what was staged", got)
		}
		checkStatus(t, repo, s.Dir, "M README.md\n?? c.txt")
	})

	t.Run("a Script's add of a deleted file stages its removal", func(t *testing.T) {
		s, repo, _ := open(t)
		removeFile(t, s.Dir, "README.md")

		script(t, s, s.Dir, "add", "README.md")
		checkStatus(t, repo, s.Dir, "D  README.md")
		script(t, s, s.Dir, "commit", "-q", "-m", "Remove README.md")

		if repo.HasFile("main", "README.md") {
			t.Error("main has README.md, want it removed")
		}
		checkStatus(t, repo, s.Dir, "")
	})

	t.Run("a Script's add of . or -A stages everything in the worktree", func(t *testing.T) {
		for _, args := range [][]string{{"add", "."}, {"add", "-A"}} {
			t.Run(args[1], func(t *testing.T) {
				s, repo, _ := open(t)
				removeFile(t, s.Dir, "README.md")
				writeFile(t, s.Dir, "a.txt", "a\n")
				writeFile(t, s.Dir, "e/u.txt", "u\n")

				script(t, s, s.Dir, args...)

				checkStatus(t, repo, s.Dir, "D  README.md\nA  a.txt\nA  e/u.txt")
			})
		}
	})

	t.Run("a Script's add of a path matching nothing fails, staging nothing", func(t *testing.T) {
		s, repo, _ := open(t)
		writeFile(t, s.Dir, "a.txt", "a\n")

		if out, err := s.Script(s.Dir, "add", "a.txt", "nothing.txt"); err == nil {
			t.Errorf("add of nothing.txt = %q, want an error", out)
		}
		checkStatus(t, repo, s.Dir, "?? a.txt")
	})

	t.Run("a Script's rm removes a file from the folder and the next commit", func(t *testing.T) {
		s, repo, _ := open(t)
		writeFile(t, s.Dir, "a.txt", "a\n")
		writeFile(t, s.Dir, "d/x.txt", "x\n")
		script(t, s, s.Dir, "add", "a.txt", "d")
		script(t, s, s.Dir, "commit", "-q", "-m", "Add a and d")

		script(t, s, s.Dir, "rm", "-q", "a.txt")
		if out, err := s.Script(s.Dir, "rm", "-q", "d"); err == nil {
			t.Errorf("rm of the folder d without -r = %q, want an error", out)
		}
		script(t, s, s.Dir, "rm", "-q", "-r", "d")

		checkFolder(t, s.Dir, map[string]string{"README.md": "Hello\n"})
		checkNoFolder(t, filepath.Join(s.Dir, "d"))
		checkStatus(t, repo, s.Dir, "D  a.txt\nD  d/x.txt")
		script(t, s, s.Dir, "commit", "-q", "-m", "Remove a and d")
		if repo.HasFile("main", "a.txt") || repo.HasFile("main", "d/x.txt") {
			t.Error("main still has a.txt or d/x.txt")
		}
		if out, err := s.Script(s.Dir, "rm", "-q", "a.txt"); err == nil {
			t.Errorf("rm of a file no longer tracked = %q, want an error", out)
		}
	})

	t.Run("a Script's commit fails with nothing staged or no message, moving nothing", func(t *testing.T) {
		s, repo, first := open(t)
		writeFile(t, s.Dir, "a.txt", "a\n")

		if out, err := s.Script(s.Dir, "commit", "-q", "-m", "Nothing"); err == nil {
			t.Errorf("commit with nothing staged = %q, want an error", out)
		}
		script(t, s, s.Dir, "add", "a.txt")
		if out, err := s.Script(s.Dir, "commit", "-q", "-m", ""); err == nil {
			t.Errorf("commit with no message = %q, want an error", out)
		}
		checkBranch(t, repo, "main", first)
		checkStatus(t, repo, s.Dir, "A  a.txt")
	})

	t.Run("a Script's checkout -b makes a branch and checks it out, and checkout of a branch goes back", func(t *testing.T) {
		s, repo, first := open(t)

		script(t, s, s.Dir, "checkout", "-q", "-b", "topic")
		if got := must(repo.CurrentBranch(s.Dir))(t); got != "topic" {
			t.Errorf("CurrentBranch = %s, want topic", got)
		}
		writeFile(t, s.Dir, "a.txt", "a\n")
		script(t, s, s.Dir, "add", "a.txt")
		script(t, s, s.Dir, "commit", "-q", "-m", "Add a")
		topic := must(repo.Commit("topic"))(t)
		checkBranch(t, repo, "main", first)

		script(t, s, s.Dir, "checkout", "-q", "main")
		checkFolder(t, s.Dir, map[string]string{"README.md": "Hello\n"})
		if got := must(repo.CurrentBranch(s.Dir))(t); got != "main" {
			t.Errorf("CurrentBranch = %s, want main", got)
		}
		if out, err := s.Script(s.Dir, "checkout", "-q", "-b", "topic"); err == nil {
			t.Errorf("checkout -b of the existing topic = %q, want an error", out)
		}
		checkBranch(t, repo, "topic", topic)

		script(t, s, s.Dir, "checkout", "-q", "-b", "other", "topic")
		checkBranch(t, repo, "other", topic)
		checkFolder(t, s.Dir, map[string]string{"README.md": "Hello\n", "a.txt": "a\n"})
	})

	t.Run("a Script's checkout --detach detaches, and a commit there moves no branch", func(t *testing.T) {
		s, repo, first := open(t)
		topic := s.Commit(git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "a\n"}})
		noError(t, repo.UpdateRef("refs/heads/topic", topic, ""))

		script(t, s, s.Dir, "checkout", "-q", "--detach")
		if got, err := repo.CurrentBranch(s.Dir); err == nil {
			t.Errorf("CurrentBranch = %s, want an error: it is detached", got)
		}
		script(t, s, s.Dir, "checkout", "-q", "--detach", "topic")
		checkFolder(t, s.Dir, map[string]string{"README.md": "Hello\n", "a.txt": "a\n"})
		writeFile(t, s.Dir, "b.txt", "b\n")
		script(t, s, s.Dir, "add", "b.txt")
		script(t, s, s.Dir, "commit", "-q", "-m", "Add b")

		head := script(t, s, s.Dir, "rev-parse", "HEAD")
		if got := must(repo.ParentsOf(head))(t); !slices.Equal(got, []string{topic}) {
			t.Errorf("the detached commit's parents = %q, want %s", got, topic)
		}
		checkBranch(t, repo, "topic", topic)
		checkBranch(t, repo, "main", first)
	})

	t.Run("a Script's commit in a worktree moves that worktree's branch, and checkout there of a branch in use fails", func(t *testing.T) {
		s, repo, first := open(t)
		wt := filepath.Join(t.TempDir(), "wt")
		noError(t, repo.AddWorktreeOnNewBranch(wt, "topic", first))
		writeFile(t, wt, "a.txt", "a\n")

		script(t, s, wt, "add", "a.txt")
		script(t, s, wt, "commit", "-q", "-m", "Add a")

		topic := must(repo.Commit("topic"))(t)
		checkLastCommit(t, repo, wt, topic, "Add a")
		checkBranch(t, repo, "main", first)
		checkStatus(t, repo, s.Dir, "")
		if out, err := s.Script(wt, "checkout", "-q", "main"); err == nil {
			t.Errorf("checkout of main, checked out in the repository = %q, want an error", out)
		}
		if got := must(repo.CurrentBranch(wt))(t); got != "topic" {
			t.Errorf("CurrentBranch(wt) = %s, want topic", got)
		}
	})
}

// script runs args with s.Script in the worktree dir, failing t when they
// fail, and returns their output.
func script(t *testing.T, s Setup, dir string, args ...string) string {
	t.Helper()
	return must(s.Script(dir, args...))(t)
}
