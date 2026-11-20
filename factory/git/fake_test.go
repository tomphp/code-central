package git_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/tomoram/code-central/factory/git"
	"github.com/tomoram/code-central/factory/git/gittest"
)

func TestFakeGitContract(t *testing.T) {
	t.Parallel()
	gittest.Run(t, func(t *testing.T) gittest.Setup {
		f, dir, _, remote := newFakeWithRemote(t)
		return gittest.Setup{Git: f, Dir: dir, Commit: func(c git.Change) string { return f.MakeCommit(dir, c) }, CommitAll: f.CommitAll, Remote: remote}
	})
}

// newFake is a Fake failing tb, with a repository whose main holds
// README.md, and that repository's folder and first commit.
func newFake(tb testing.TB) (*git.Fake, string, string) {
	f := git.NewFake(tb)
	dir := tb.TempDir()
	return f, dir, f.Init(dir, map[string]string{"README.md": "Hello\n"}, "First")
}

// commitFunc makes a commit in a case's repository and returns its ID.
type commitFunc func(git.Change) string

// recorder is a test's testing.TB that records the failures it is given
// with Errorf, rather than failing the test.
type recorder struct {
	testing.TB
	failures []string
}

func (r *recorder) Errorf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}

func TestFakeGitFailsTheTestOnWhatItDoesntModel(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		// do does it in a repository whose main holds README.md.
		do func(t *testing.T, dir string, repo git.Repo, commit commitFunc) error
	}{
		{"a change of several lines on both sides, merged", func(t *testing.T, dir string, repo git.Repo, commit commitFunc) error {
			ours, theirs := bothSides(commit, map[string]string{"a.txt": "one\ntwo\n"}, map[string]string{"a.txt": "one\nours\n"}, map[string]string{"a.txt": "one\ntheirs\n"})
			_, _, err := repo.MergeTree(ours, theirs)
			return err
		}},
		{"a change of several lines on both sides, replayed", func(t *testing.T, dir string, repo git.Repo, commit commitFunc) error {
			ours, theirs := bothSides(commit, map[string]string{"a.txt": "one\ntwo\n"}, map[string]string{"a.txt": "one\nours\n"}, map[string]string{"a.txt": "one\ntheirs\n"})
			_, _, err := repo.ReplayTree(ours, theirs)
			return err
		}},
		{"an empty file changed on both sides", func(t *testing.T, dir string, repo git.Repo, commit commitFunc) error {
			ours, theirs := bothSides(commit, map[string]string{"a.txt": ""}, map[string]string{"a.txt": "ours\n"}, map[string]string{"a.txt": "theirs\n"})
			_, _, err := repo.MergeTree(ours, theirs)
			return err
		}},
		{"a file renamed on one side and changed on the other", func(t *testing.T, dir string, repo git.Repo, commit commitFunc) error {
			base := commit(git.Change{Files: map[string]string{"a.txt": "one\n"}})
			ours := commit(git.Change{Parents: []string{base}, Files: map[string]string{"b.txt": "one\n"}, Removed: []string{"a.txt"}})
			theirs := commit(git.Change{Parents: []string{base}, Files: map[string]string{"a.txt": "two\n"}})
			_, _, err := repo.MergeTree(ours, theirs)
			return err
		}},
		{"a folder renamed on one side and added to on the other", func(t *testing.T, dir string, repo git.Repo, commit commitFunc) error {
			base := commit(git.Change{Files: map[string]string{"d/a.txt": "one\n"}})
			ours := commit(git.Change{Parents: []string{base}, Files: map[string]string{"e/a.txt": "one\n"}, Removed: []string{"d/a.txt"}})
			theirs := commit(git.Change{Parents: []string{base}, Files: map[string]string{"d/b.txt": "b\n"}})
			_, _, err := repo.MergeTree(ours, theirs)
			return err
		}},
		{"a file on one side where the other has a folder", func(t *testing.T, dir string, repo git.Repo, commit commitFunc) error {
			ours, theirs := bothSides(commit, map[string]string{"a.txt": "one\n"}, map[string]string{"p": "file\n"}, map[string]string{"p/q.txt": "q\n"})
			_, _, err := repo.MergeTree(ours, theirs)
			return err
		}},
		{"several merge bases", func(t *testing.T, dir string, repo git.Repo, commit commitFunc) error {
			base := commit(git.Change{Files: map[string]string{"a.txt": "one\n"}})
			x := commit(git.Change{Parents: []string{base}, Files: map[string]string{"x.txt": "x\n"}})
			y := commit(git.Change{Parents: []string{base}, Files: map[string]string{"y.txt": "y\n"}})
			xy := commit(git.Change{Parents: []string{x, y}, Message: "x and y"})
			yx := commit(git.Change{Parents: []string{y, x}, Message: "y and x"})
			_, _, err := repo.MergeTree(xy, yx)
			return err
		}},
		{"revision syntax", func(t *testing.T, dir string, repo git.Repo, commit commitFunc) error {
			_, err := repo.Commit("main^")
			return err
		}},
		{"a fast-forward in a folder that isn't a worktree", func(t *testing.T, dir string, repo git.Repo, commit commitFunc) error {
			_, err := repo.FastForward(t.TempDir(), "main")
			return err
		}},
		{"a worktree inside another's folder", func(t *testing.T, dir string, repo git.Repo, commit commitFunc) error {
			return repo.AddDetachedWorktree(filepath.Join(dir, "inside"), "main")
		}},
		{"a worktree at a relative path", func(t *testing.T, dir string, repo git.Repo, commit commitFunc) error {
			return repo.AddDetachedWorktree("relative", "main")
		}},
		{"a worktree on a commit rather than a branch", func(t *testing.T, dir string, repo git.Repo, commit commitFunc) error {
			c := commit(git.Change{Files: map[string]string{"a.txt": "a\n"}})
			return repo.AddWorktree(filepath.Join(t.TempDir(), "wt"), c)
		}},
		{"a worktree whose .git file is gone", func(t *testing.T, dir string, repo git.Repo, commit commitFunc) error {
			wt := filepath.Join(t.TempDir(), "wt")
			if err := repo.AddDetachedWorktree(wt, "main"); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(wt, ".git")); err != nil {
				t.Fatal(err)
			}
			return repo.PruneWorktrees()
		}},
		{"an ignore file in a worktree", func(t *testing.T, dir string, repo git.Repo, commit commitFunc) error {
			writeFile(t, filepath.Join(dir, ".gitignore"), "*.log\n")
			_, err := repo.Status(dir)
			return err
		}},
		{"a repository inside a worktree", func(t *testing.T, dir string, repo git.Repo, commit commitFunc) error {
			if err := os.MkdirAll(filepath.Join(dir, "sub", ".git"), 0o755); err != nil {
				t.Fatal(err)
			}
			_, err := repo.Status(dir)
			return err
		}},
		{"a path status would quote", func(t *testing.T, dir string, repo git.Repo, commit commitFunc) error {
			writeFile(t, filepath.Join(dir, "a b.txt"), "a\n")
			_, err := repo.Status(dir)
			return err
		}},
		{"a link in a worktree", func(t *testing.T, dir string, repo git.Repo, commit commitFunc) error {
			if err := os.Symlink("README.md", filepath.Join(dir, "link")); err != nil {
				t.Skipf("no symbolic links here: %v", err)
			}
			_, err := repo.Status(dir)
			return err
		}},
		{"an executable in a worktree", func(t *testing.T, dir string, repo git.Repo, commit commitFunc) error {
			if runtime.GOOS == "windows" {
				t.Skip("files have no executable bit on Windows")
			}
			if err := os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			_, err := repo.Status(dir)
			return err
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rec := &recorder{TB: t}
			f, dir, _ := newFake(rec)

			err := c.do(t, dir, f.Open(dir), func(change git.Change) string { return f.MakeCommit(dir, change) })
			if err == nil {
				t.Error("it succeeded, want an error")
			}
			if len(rec.failures) != 1 || !strings.Contains(rec.failures[0], "not modelled") {
				t.Errorf("the test's failures = %q, want one that it isn't modelled", rec.failures)
			}
		})
	}
}

func TestFakeGitFailsTheTestOnARepositoryOpenedInsideAWorktree(t *testing.T) {
	t.Parallel()
	rec := &recorder{TB: t}
	f, dir, _ := newFake(rec)
	sub := filepath.Join(dir, "d")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := f.Open(sub).Commit("main"); err == nil {
		t.Error("it succeeded, want an error")
	}
	if len(rec.failures) != 1 || !strings.Contains(rec.failures[0], "not modelled") {
		t.Errorf("the test's failures = %q, want one that it isn't modelled", rec.failures)
	}
}

// writeFile writes content to the file name.
func writeFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// bothSides commits base, and ours and theirs on it, returning ours and
// theirs.
func bothSides(commit commitFunc, base, ours, theirs map[string]string) (string, string) {
	b := commit(git.Change{Files: base})
	return commit(git.Change{Parents: []string{b}, Files: ours}), commit(git.Change{Parents: []string{b}, Files: theirs})
}

func TestFakeGitIDsAreTheirContent(t *testing.T) {
	t.Parallel()
	fortyHex := regexp.MustCompile(`^[0-9a-f]{40}$`)
	commits := func(msg string) (*git.Fake, string, string) {
		f, dir, first := newFake(t)
		return f, dir, f.MakeCommit(dir, git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n"}, Message: msg})
	}
	f, dir, c := commits("Second")
	_, _, again := commits("Second")
	_, _, other := commits("Other")
	repo := f.Open(dir)

	if !fortyHex.MatchString(c) {
		t.Errorf("ID %q isn't 40 hex digits", c)
	}
	if again != c {
		t.Errorf("the same commit in another repository = %s, want %s", again, c)
	}
	if other == c {
		t.Errorf("a commit with another message = %s, the same ID", other)
	}
	if got, err := repo.Commit(c[:7]); err != nil || got != c {
		t.Errorf("Commit(%s) = %s, %v, want %s", c[:7], got, err, c)
	}
	tree, err := repo.TreeOf(c)
	if err != nil {
		t.Fatal(err)
	}
	if !fortyHex.MatchString(tree) || tree == c {
		t.Errorf("tree ID %q isn't 40 hex digits of its own", tree)
	}
}

func TestAFakeGitReadsBackARefAndACommitsSubjectAndFiles(t *testing.T) {
	t.Parallel()
	f, dir, first := newFake(t)
	c := f.MakeCommit(dir, git.Change{Parents: []string{first}, Files: map[string]string{"d/a.txt": "one\n"}, Message: "Add\n  a file\n\nWhy it was added."})

	if got, ok := f.Ref(dir, "refs/heads/main"); !ok || got != first {
		t.Errorf("Ref(refs/heads/main) = %q, %v, want %s", got, ok, first)
	}
	if got, ok := f.Ref(dir, "refs/heads/none"); ok {
		t.Errorf("Ref(refs/heads/none) = %q, want no commit", got)
	}

	if got := f.Subject(dir, c); got != "Add a file" {
		t.Errorf("Subject = %q, want the first paragraph on one line, as %%s gives it", got)
	}
	if got := f.Subject(dir, "main"); got != "First" {
		t.Errorf("Subject(main) = %q, want First", got)
	}
	if got, ok := f.File(dir, c, "d/a.txt"); !ok || got != "one\n" {
		t.Errorf("File(d/a.txt) = %q, %v, want its content", got, ok)
	}
	if got, ok := f.File(dir, c, "README.md"); !ok || got != "Hello\n" {
		t.Errorf("File(README.md) = %q, %v, want the first parent's", got, ok)
	}
	for _, p := range []string{"b.txt", "d"} {
		if got, ok := f.File(dir, c, p); ok {
			t.Errorf("File(%s) = %q, want no file", p, got)
		}
	}
}
