package git_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/tomoram/code-central/factory/git"
	"github.com/tomoram/code-central/factory/git/gittest"
)

func TestFakeGitContract(t *testing.T) {
	t.Parallel()
	gittest.Run(t, func(t *testing.T) gittest.Setup {
		f, dir, _ := newFake(t)
		return gittest.Setup{Git: f, Dir: dir, Commit: func(c git.Change) string { return f.MakeCommit(dir, c) }}
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
		do func(t *testing.T, repo git.Repo, commit commitFunc) error
	}{
		{"a change of several lines on both sides, merged", func(t *testing.T, repo git.Repo, commit commitFunc) error {
			ours, theirs := bothSides(commit, map[string]string{"a.txt": "one\ntwo\n"}, map[string]string{"a.txt": "one\nours\n"}, map[string]string{"a.txt": "one\ntheirs\n"})
			_, _, err := repo.MergeTree(ours, theirs)
			return err
		}},
		{"a change of several lines on both sides, replayed", func(t *testing.T, repo git.Repo, commit commitFunc) error {
			ours, theirs := bothSides(commit, map[string]string{"a.txt": "one\ntwo\n"}, map[string]string{"a.txt": "one\nours\n"}, map[string]string{"a.txt": "one\ntheirs\n"})
			_, _, err := repo.ReplayTree(ours, theirs)
			return err
		}},
		{"an empty file changed on both sides", func(t *testing.T, repo git.Repo, commit commitFunc) error {
			ours, theirs := bothSides(commit, map[string]string{"a.txt": ""}, map[string]string{"a.txt": "ours\n"}, map[string]string{"a.txt": "theirs\n"})
			_, _, err := repo.MergeTree(ours, theirs)
			return err
		}},
		{"a file renamed on one side and changed on the other", func(t *testing.T, repo git.Repo, commit commitFunc) error {
			base := commit(git.Change{Files: map[string]string{"a.txt": "one\n"}})
			ours := commit(git.Change{Parents: []string{base}, Files: map[string]string{"b.txt": "one\n"}, Removed: []string{"a.txt"}})
			theirs := commit(git.Change{Parents: []string{base}, Files: map[string]string{"a.txt": "two\n"}})
			_, _, err := repo.MergeTree(ours, theirs)
			return err
		}},
		{"a folder renamed on one side and added to on the other", func(t *testing.T, repo git.Repo, commit commitFunc) error {
			base := commit(git.Change{Files: map[string]string{"d/a.txt": "one\n"}})
			ours := commit(git.Change{Parents: []string{base}, Files: map[string]string{"e/a.txt": "one\n"}, Removed: []string{"d/a.txt"}})
			theirs := commit(git.Change{Parents: []string{base}, Files: map[string]string{"d/b.txt": "b\n"}})
			_, _, err := repo.MergeTree(ours, theirs)
			return err
		}},
		{"a file on one side where the other has a folder", func(t *testing.T, repo git.Repo, commit commitFunc) error {
			ours, theirs := bothSides(commit, map[string]string{"a.txt": "one\n"}, map[string]string{"p": "file\n"}, map[string]string{"p/q.txt": "q\n"})
			_, _, err := repo.MergeTree(ours, theirs)
			return err
		}},
		{"several merge bases", func(t *testing.T, repo git.Repo, commit commitFunc) error {
			base := commit(git.Change{Files: map[string]string{"a.txt": "one\n"}})
			x := commit(git.Change{Parents: []string{base}, Files: map[string]string{"x.txt": "x\n"}})
			y := commit(git.Change{Parents: []string{base}, Files: map[string]string{"y.txt": "y\n"}})
			xy := commit(git.Change{Parents: []string{x, y}, Message: "x and y"})
			yx := commit(git.Change{Parents: []string{y, x}, Message: "y and x"})
			_, _, err := repo.MergeTree(xy, yx)
			return err
		}},
		{"revision syntax", func(t *testing.T, repo git.Repo, commit commitFunc) error {
			_, err := repo.Commit("main^")
			return err
		}},
		{"a push", func(t *testing.T, repo git.Repo, commit commitFunc) error {
			_, err := repo.Push(context.Background(), "origin", "main", "")
			return err
		}},
		{"a fast-forward in a worktree", func(t *testing.T, repo git.Repo, commit commitFunc) error {
			_, err := repo.FastForward(t.TempDir(), "main")
			return err
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rec := &recorder{TB: t}
			f, dir, _ := newFake(rec)

			err := c.do(t, f.Open(dir), func(change git.Change) string { return f.MakeCommit(dir, change) })
			if err == nil {
				t.Error("it succeeded, want an error")
			}
			if len(rec.failures) != 1 || !strings.Contains(rec.failures[0], "not modelled") {
				t.Errorf("the test's failures = %q, want one that it isn't modelled", rec.failures)
			}
		})
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
