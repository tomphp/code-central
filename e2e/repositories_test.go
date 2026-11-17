//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A Project of two Repositories under Merge + Push, from claim to Done: the
// Factory clones the second beside the Home Repository, gives the Ticket a
// worktree and branch in each, the session commits in both and finishes with
// `repositories`, and accepting lands each Repository in the landing order
// and pushes each base.
func TestATwoRepositoryProjectGoesFromClaimToDoneUnderMergeAndPush(t *testing.T) {
	var lib, libRemote string
	w := startWorldSetUp(t, "", true, func(root string, svc *service, inRepo func(args ...string) string, env []string) {
		// lib's remote holds a first commit on main, as a repository to
		// clone does.
		libRemote = filepath.Join(root, "lib-remote.git")
		seed := filepath.Join(root, "lib-seed")
		for _, args := range [][]string{
			{"init", "-q", "--bare", "-b", "main", libRemote},
			{"init", "-q", "-b", "main", seed},
		} {
			gitIn(t, root, env, args...)
		}
		if err := os.WriteFile(filepath.Join(seed, "README"), []byte("lib\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		gitIn(t, seed, env, "add", "README")
		gitIn(t, seed, env, "commit", "-q", "-m", "Start lib")
		gitIn(t, seed, env, "push", "-q", libRemote, "main")

		// The human lists lib after the Home Repository and sets Merge + Push.
		repo := filepath.Join(root, "work", "acme")
		if code, out := svc.cli(t, repo, "", "repository", "add", "lib", "--clone-url", libRemote, "--base", "main"); code != 0 {
			t.Fatalf("repository add: exit %d: %s", code, out)
		}
		svc.human(t, "/api/p/acme/set_landing_method", `{"method":"merge","push":true,"remote":"origin","land_per_spec":false}`)
		if code, out := svc.cli(t, repo, "", "create-spec", "--title", "Greet in both"); code != 0 {
			t.Fatalf("create-spec: exit %d: %s", code, out)
		}
		if code, out := svc.cli(t, repo, "", "create-ticket", "--spec", "acme-1", "--title", "Say hello", "--state", "Ready"); code != 0 {
			t.Fatalf("create-ticket: exit %d: %s", code, out)
		}
		lib = filepath.Join(root, "work", "lib")
	})
	svc, repo := w.svc, w.repo
	libGit := func(args ...string) string { t.Helper(); return gitIn(t, lib, nil, args...) }

	// The Ticket's claim: a worktree in each Repository on the same branch,
	// and a finish that names both Changed Repositories.
	in := w.waitFor("acme-2", "In Review")
	if len(in.Repositories) != 2 || in.Repositories[0].Name != "acme" || in.Repositories[1].Name != "lib" {
		t.Fatalf("Changed Repositories: %+v, want acme then lib", in.Repositories)
	}
	for _, r := range in.Repositories {
		if r.Branch != "acme-2" || r.Commit == "" {
			t.Errorf("%s changed on branch %q at %q, want acme-2 at a commit", r.Name, r.Branch, r.Commit)
		}
	}
	if got := libGit("rev-parse", "acme-2"); got != in.Repositories[1].Commit {
		t.Errorf("lib's branch acme-2 is at %s, finish named %s", got, in.Repositories[1].Commit)
	}
	for _, wt := range []string{
		filepath.Join(filepath.Dir(repo), "acme.worktrees", "acme-2"),
		filepath.Join(filepath.Dir(repo), "lib.worktrees", "acme-2"),
	} {
		if _, err := os.Stat(filepath.Join(wt, "hello.txt")); err != nil {
			t.Errorf("no hello.txt in the worktree %s: %v", wt, err)
		}
	}
	homeBase, libBase := w.base, libGit("rev-parse", "main")

	// Accepting lands each Repository's work, Home first, and pushes each base.
	svc.human(t, "/api/p/acme/tickets/2/accept", `{}`)
	done := w.waitFor("acme-2", "Done")
	for _, r := range done.Repositories {
		if r.Landed == nil || r.Landed.Commit == "" {
			t.Fatalf("%s didn't land: %+v", r.Name, done.Repositories)
		}
	}
	homeRemote := filepath.Join(filepath.Dir(filepath.Dir(repo)), "remote.git")
	for i, r := range []struct {
		name, remote string
		git          func(args ...string) string
		base         string
	}{
		{"acme", homeRemote, w.inRepo, homeBase},
		{"lib", libRemote, libGit, libBase},
	} {
		merge := r.git("rev-parse", "main")
		if merge == r.base {
			t.Errorf("%s's main didn't move", r.name)
		}
		if got := done.Repositories[i].Landed.Commit; got != merge {
			t.Errorf("%s recorded landed commit %s, want its main's tip %s", r.name, got, merge)
		}
		if got := r.git("log", "-1", "--format=%s", "main"); got != "Merge acme-2: Say hello" {
			t.Errorf("%s's merge commit subject %q", r.name, got)
		}
		r.git("merge-base", "--is-ancestor", in.Repositories[i].Commit, "main") // fails the test if not
		// Push was on: the remote's main is the merge.
		if got := gitIn(t, repo, nil, "--git-dir", r.remote, "rev-parse", "main"); got != merge {
			t.Errorf("%s's remote main is %s, want %s", r.name, got, merge)
		}
	}
	out := w.factoryOut.String()
	for _, want := range []string{"repository lib: cloned ", "acme-2: pushed main to origin in Repository lib"} {
		if !strings.Contains(out, want) {
			t.Errorf("the Factory's output doesn't have %q:\n%s", want, out)
		}
	}
	w.stop(t)
}

// gitIn runs git in dir, with env when set, and returns its output.
func gitIn(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command(gitBinary(t), args...)
	cmd.Dir = dir
	cmd.Env = append(env, "GIT_CONFIG_NOSYSTEM=1", "GIT_AUTHOR_NAME=E2E", "GIT_AUTHOR_EMAIL=e2e@example.com",
		"GIT_COMMITTER_NAME=E2E", "GIT_COMMITTER_EMAIL=e2e@example.com", "HOME="+t.TempDir())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}
