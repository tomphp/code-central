package git_test

import (
	"context"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/tomoram/code-central/factory/git"
)

// newFakeWithRemote is newFake with an empty bare repository as the
// repository's remote origin, and that remote's folder.
func newFakeWithRemote(tb testing.TB) (f *git.Fake, dir, first, remote string) {
	f, dir, first = newFake(tb)
	remote = filepath.Join(tb.TempDir(), "remote.git")
	f.InitBare(remote)
	f.AddRemote(dir, "origin", remote)
	return f, dir, first, remote
}

func TestAFakeRemoteBrokenFailsEverythingAskingItUntilRestored(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f, dir, first, remote := newFakeWithRemote(t)
	repo := f.Open(dir)
	if out, err := repo.Push(ctx, "origin", "main", ""); err != nil {
		t.Fatalf("Push: %v: %s", err, out)
	}
	unreachable := func(what, out string, err error) {
		t.Helper()
		if err == nil || !strings.Contains(out+err.Error(), "does not appear to be a git repository") {
			t.Errorf("%s of a broken remote: %v: %s, want it unreachable", what, err, out)
		}
	}

	f.BreakRemote(remote)
	out, err := repo.Push(ctx, "origin", "main", "")
	unreachable("a push", out, err)
	out, err = repo.Fetch(ctx, "origin")
	unreachable("a fetch", out, err)
	_, err = repo.RemoteHas(ctx, "origin", "main")
	unreachable("ls-remote", "", err)
	out, err = f.Clone(ctx, "origin", remote, filepath.Join(t.TempDir(), "clone"))
	unreachable("a clone", out, err)

	f.RestoreRemote(remote)
	if out, err := repo.Fetch(ctx, "origin"); err != nil {
		t.Errorf("Fetch once restored: %v: %s", err, out)
	}
	if has, err := repo.RemoteHas(ctx, "origin", "main"); err != nil || !has {
		t.Errorf("RemoteHas(main) once restored = %t, %v; want true", has, err)
	}
	next := f.MakeCommit(dir, git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n"}})
	if err := repo.UpdateRef("refs/heads/main", next, first); err != nil {
		t.Fatal(err)
	}
	if out, err := repo.Push(ctx, "origin", "main", ""); err != nil {
		t.Errorf("Push once restored: %v: %s", err, out)
	}
	if out, err := f.Clone(ctx, "origin", remote, filepath.Join(t.TempDir(), "clone")); err != nil {
		t.Errorf("Clone once restored: %v: %s", err, out)
	}
}

func TestAFakeRemoteRefusingPushesStillServesFetchesUntilItAcceptsThem(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f, dir, first, remote := newFakeWithRemote(t)
	repo := f.Open(dir)
	if out, err := repo.Push(ctx, "origin", "main", ""); err != nil {
		t.Fatalf("Push: %v: %s", err, out)
	}
	next := f.MakeCommit(dir, git.Change{Parents: []string{first}, Files: map[string]string{"a.txt": "one\n"}})
	if err := repo.UpdateRef("refs/heads/main", next, first); err != nil {
		t.Fatal(err)
	}

	f.RefusePushes(remote)
	out, err := repo.Push(ctx, "origin", "main", "")
	if err == nil || !strings.Contains(out, "pushes are refused") || !strings.Contains(out, "[remote rejected]") {
		t.Errorf("Push to a remote refusing pushes: %v: %s, want it refused", err, out)
	}
	if c, err := f.Open(remote).Commit("main"); err != nil || c != first {
		t.Errorf("the remote's main after a refused push = %s, %v; want %s", c, err, first)
	}
	if out, err := repo.Fetch(ctx, "origin"); err != nil {
		t.Errorf("Fetch from a remote refusing pushes: %v: %s", err, out)
	}
	if c, err := repo.Commit("origin/main"); err != nil || c != first {
		t.Errorf("origin/main after a refused push = %s, %v; want %s", c, err, first)
	}

	f.AcceptPushes(remote)
	if out, err := repo.Push(ctx, "origin", "main", ""); err != nil {
		t.Fatalf("Push once accepting: %v: %s", err, out)
	}
	if c, err := f.Open(remote).Commit("main"); err != nil || c != next {
		t.Errorf("the remote's main once accepting = %s, %v; want %s", c, err, next)
	}
}

func TestAFakeGitsVersionIsOneTheFactoryRunsWithUntilSet(t *testing.T) {
	t.Parallel()
	f, _, _ := newFake(t)

	v, err := f.Version()
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`^git version (\d+)\.(\d+)\.\d+$`).FindStringSubmatch(v)
	if m == nil {
		t.Fatalf("Version = %q, want git version and a number", v)
	}
	if major, _ := strconv.Atoi(m[1]); major < 2 {
		t.Errorf("Version = %q, want 2.38 or later", v)
	} else if minor, _ := strconv.Atoi(m[2]); major == 2 && minor < 38 {
		t.Errorf("Version = %q, want 2.38 or later", v)
	}

	f.SetVersion("git version 2.37.0")
	if v, err := f.Version(); err != nil || v != "git version 2.37.0" {
		t.Errorf("Version once set = %q, %v; want git version 2.37.0", v, err)
	}
}

func TestFakeGitFailsTheTestOnRemotesItDoesntModel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for _, c := range []struct {
		name string
		// do does it with the repository at dir, whose main holds
		// README.md, and whose remote origin, the empty bare repository
		// at remote, has its main.
		do func(t *testing.T, f *git.Fake, dir, remote string) error
	}{
		{"a push to a folder outside the fake", func(t *testing.T, f *git.Fake, dir, remote string) error {
			_, err := f.Open(dir).Push(ctx, t.TempDir(), "main", "")
			return err
		}},
		{"a remote URL that isn't a folder", func(t *testing.T, f *git.Fake, dir, remote string) error {
			f.AddRemote(dir, "web", "https://example.com/repo.git")
			_, err := f.Open(dir).Fetch(ctx, "web")
			return err
		}},
		{"a push to a repository that isn't bare", func(t *testing.T, f *git.Fake, dir, remote string) error {
			other := t.TempDir()
			f.Init(other, map[string]string{"b.txt": "b\n"}, "Other")
			f.AddRemote(dir, "other", other)
			_, err := f.Open(dir).Push(ctx, "other", "main", "")
			return err
		}},
		{"a push of a refspec", func(t *testing.T, f *git.Fake, dir, remote string) error {
			_, err := f.Open(dir).Push(ctx, "origin", "main:other", "")
			return err
		}},
		{"a fetch from a URL rather than a remote", func(t *testing.T, f *git.Fake, dir, remote string) error {
			_, err := f.Open(dir).Fetch(ctx, remote)
			return err
		}},
		{"a clone of an empty repository", func(t *testing.T, f *git.Fake, dir, remote string) error {
			empty := filepath.Join(t.TempDir(), "empty.git")
			f.InitBare(empty)
			_, err := f.Clone(ctx, "origin", empty, filepath.Join(t.TempDir(), "clone"))
			return err
		}},
		{"a fetch of a tag", func(t *testing.T, f *git.Fake, dir, remote string) error {
			bare := f.Open(remote)
			main, err := bare.Commit("main")
			if err != nil {
				t.Fatal(err)
			}
			if err := bare.UpdateRef("refs/tags/v1", main, ""); err != nil {
				t.Fatal(err)
			}
			_, err = f.Open(dir).Fetch(ctx, "origin")
			return err
		}},
		{"the top level of a bare repository", func(t *testing.T, f *git.Fake, dir, remote string) error {
			_, err := f.TopLevel(remote)
			return err
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rec := &recorder{TB: t}
			f, dir, _, remote := newFakeWithRemote(rec)
			if out, err := f.Open(dir).Push(ctx, "origin", "main", ""); err != nil {
				t.Fatalf("Push: %v: %s", err, out)
			}

			if err := c.do(t, f, dir, remote); err == nil {
				t.Error("it succeeded, want an error")
			}
			if len(rec.failures) != 1 || !strings.Contains(rec.failures[0], "not modelled") {
				t.Errorf("the test's failures = %q, want one that it isn't modelled", rec.failures)
			}
		})
	}
}
