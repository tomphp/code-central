package factory

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/tomoram/code-central/factory/git"
)

// An Env with no Git goes through real git: the git binary found on
// PATH, run with the Env's environment.
func TestAnEnvWithNoGitGoesThroughRealGit(t *testing.T) {
	t.Parallel()
	bin := t.TempDir()
	name := "git"
	if runtime.GOOS == "windows" {
		name = "git.exe"
	}
	binary := filepath.Join(bin, name)
	if err := os.WriteFile(binary, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	e := Env{Environ: []string{"PATH=" + bin, "HOME=/home/me"}}
	g, err := e.gitPort()
	got, ok := g.(git.Real)
	if err != nil || !ok || got.Binary != binary || !slices.Equal(got.Environ, e.Environ) {
		t.Errorf("the Git port is %#v, %v, want git.Real with %s and the Env's environment", g, err, binary)
	}
}

// An Env with no Git and no git on PATH has no Git port.
func TestAnEnvWithNoGitAndNoGitOnPathHasNoGitPort(t *testing.T) {
	t.Parallel()
	e := Env{Environ: []string{"PATH=" + t.TempDir()}}
	if _, err := e.gitPort(); err == nil || err.Error() != "git isn't on PATH" {
		t.Errorf("the Git port fails with %v, want git isn't on PATH", err)
	}
}

// An Env's own Git is the one the Factory goes through, with no git on
// PATH needed.
func TestAnEnvsOwnGitIsTheFactorys(t *testing.T) {
	t.Parallel()
	own := &fakeGit{}
	if got, err := (Env{Git: own}).gitPort(); got != own || err != nil {
		t.Errorf("the Git port is %#v, %v, want the Env's own", got, err)
	}
}

// fakeGit is a Git that opens fakeRepos.
type fakeGit struct{}

func (*fakeGit) Open(dir string) git.Repo { return newFakeRepo(dir) }

func (*fakeGit) Clone(ctx context.Context, remote, url, path string) (string, error) {
	panic("fakeGit doesn't clone")
}

func (*fakeGit) TopLevel(dir string) (string, error) { return dir, nil }

func (*fakeGit) Version() (string, error) { return "git version 2.50.0", nil }
