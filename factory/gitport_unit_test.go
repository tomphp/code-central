package factory

import (
	"slices"
	"testing"

	"github.com/tomoram/code-central/factory/git"
)

// An Env with no Git opens its Repositories through real git: the git
// binary found on PATH, run with the Env's environment.
func TestAnEnvWithNoGitOpensRepositoriesThroughRealGit(t *testing.T) {
	e := Env{Environ: []string{"PATH=/bin", "HOME=/home/me"}}
	got, ok := e.gitPort("/bin/git").(git.Real)
	if !ok || got.Binary != "/bin/git" || !slices.Equal(got.Environ, e.Environ) {
		t.Errorf("the Git port is %#v, want git.Real with /bin/git and the Env's environment", e.gitPort("/bin/git"))
	}
}

// An Env's own Git is the one its Repositories are opened through.
func TestAnEnvsOwnGitOpensItsRepositories(t *testing.T) {
	own := &fakeGit{}
	if got := (Env{Git: own}).gitPort("/bin/git"); got != own {
		t.Errorf("the Git port is %#v, want the Env's own", got)
	}
}

// fakeGit is a Git that opens fakeRepos.
type fakeGit struct{}

func (*fakeGit) Open(dir string) git.Repo { return newFakeRepo(dir) }
