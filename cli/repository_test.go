package cli_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/codecentraltest"
)

// srcPath is a clone's absolute path on this OS: "/src/<name>" isn't
// absolute on Windows.
func srcPath(name string) string { return filepath.ToSlash(filepath.Join(os.TempDir(), "src", name)) }

// The repository command (Spec code-central-848) manages a Project's
// Repositories through the client, as the human, outside the operation
// table.

func repositoryEnv(t *testing.T) (*codecentraltest.Server, map[string]string) {
	t.Helper()
	srv := codecentraltest.NewServer(t)
	if _, err := srv.Client.CreateProject(context.Background(), api.CreateProjectInput{Key: "acme"}); err != nil {
		t.Fatal(err)
	}
	return srv, map[string]string{"CODE_CENTRAL_URL": srv.URL, "CODE_CENTRAL_PROJECT": "acme"}
}

// landingOrder is the Project's Repository Names, in landing order.
func landingOrder(t *testing.T, srv *codecentraltest.Server) []string {
	t.Helper()
	p, err := srv.Client.GetProject(context.Background(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range p.Repositories {
		out = append(out, r.Name)
	}
	return out
}

// printed is a command's stdout, once it has exited 0.
func printed(t *testing.T, r result) string {
	t.Helper()
	if r.code != 0 {
		t.Fatalf("exit %d, stderr: %s", r.code, r.stderr)
	}
	return r.stdout
}

func TestRepositoryListShowsTheHomeRepositoryAlone(t *testing.T) {
	t.Parallel()
	_, env := repositoryEnv(t)
	out := printed(t, run(t, env, "", "repository", "list"))
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || !strings.Contains(lines[1], "acme (home)") || !strings.Contains(lines[1], "main") {
		t.Fatalf("list:\n%s\nwant a header and the Home Repository acme on main", out)
	}
	if !strings.Contains(lines[1], "(not reported yet)") {
		t.Fatalf("list:\n%s\nwant the Home's Clone URL not reported yet", out)
	}
}

// The Factory reports the Home's clone has no Push remote: it is
// local-only (Spec code-central-890).
func TestRepositoryListShowsALocalOnlyHome(t *testing.T) {
	t.Parallel()
	srv, env := repositoryEnv(t)
	if _, err := srv.Client.ReportPushRemote(context.Background(), "acme", api.ReportPushRemoteInput{URL: ""}); err != nil {
		t.Fatal(err)
	}
	out := printed(t, run(t, env, "", "repository", "list"))
	if lines := strings.Split(strings.TrimSpace(out), "\n"); len(lines) != 2 || !strings.Contains(lines[1], "acme (home)") || !strings.Contains(lines[1], "(none: local only)") {
		t.Fatalf("list:\n%s\nwant the Home shown local only", out)
	}
}

func TestRepositoryAddEditMoveAndRemove(t *testing.T) {
	t.Parallel()
	srv, env := repositoryEnv(t)

	out := printed(t, run(t, env, "", "repository", "add", "web", "--clone-url", "git@github.com:acme/web.git"))
	if !strings.Contains(out, "git@github.com:acme/web.git") {
		t.Fatalf("add printed:\n%s\nwant the list with web", out)
	}
	printed(t, run(t, env, "", "repository", "add", "api", "--clone-url", "https://github.com/acme/api", "--base", "develop", "--local-path", srcPath("api")))
	if got := landingOrder(t, srv); !slices.Equal(got, []string{"acme", "web", "api"}) {
		t.Fatalf("order = %v", got)
	}

	printed(t, run(t, env, "", "repository", "edit", "web", "--base", "trunk", "--local-path", srcPath("web")))
	p, _ := srv.Client.GetProject(context.Background(), "acme")
	if web := p.Repositories[1]; web.Base != "trunk" || web.LocalPath != srcPath("web") || web.CloneURL != "git@github.com:acme/web.git" {
		t.Fatalf("web = %+v, want Base trunk at /src/web with its Clone URL kept", web)
	}
	// An empty --local-path clears it; a flag left out keeps its value.
	printed(t, run(t, env, "", "repository", "edit", "web", "--local-path", ""))
	p, _ = srv.Client.GetProject(context.Background(), "acme")
	if web := p.Repositories[1]; web.LocalPath != "" || web.Base != "trunk" {
		t.Fatalf("web = %+v, want no local path and Base trunk", web)
	}

	printed(t, run(t, env, "", "repository", "move", "api", "--before", "acme"))
	if got := landingOrder(t, srv); !slices.Equal(got, []string{"api", "acme", "web"}) {
		t.Fatalf("after moving api before acme, order = %v", got)
	}
	printed(t, run(t, env, "", "repository", "move", "api", "--after", "web"))
	if got := landingOrder(t, srv); !slices.Equal(got, []string{"acme", "web", "api"}) {
		t.Fatalf("after moving api after web, order = %v", got)
	}

	printed(t, run(t, env, "", "repository", "remove", "web"))
	if got := landingOrder(t, srv); !slices.Equal(got, []string{"acme", "api"}) {
		t.Fatalf("after removing web, order = %v", got)
	}
}

// A Repository other than the Home needs a Clone URL, a local path or
// both (Spec code-central-879): one with no Clone URL is local-only.
func TestRepositoryAddsALocalOnlyRepositoryAndClearsACloneURL(t *testing.T) {
	t.Parallel()
	srv, env := repositoryEnv(t)

	out := printed(t, run(t, env, "", "repository", "add", "notes", "--local-path", srcPath("notes")))
	if !strings.Contains(out, "notes") || !strings.Contains(out, "(none: local only)") {
		t.Fatalf("add printed:\n%s\nwant the list with notes shown local only", out)
	}
	printed(t, run(t, env, "", "repository", "add", "web", "--clone-url", "git@github.com:acme/web.git", "--local-path", srcPath("web")))

	// An empty --clone-url, alone, clears it.
	printed(t, run(t, env, "", "repository", "edit", "web", "--clone-url", ""))
	p, _ := srv.Client.GetProject(context.Background(), "acme")
	if web := p.Repositories[2]; web.Name != "web" || web.CloneURL != "" || web.LocalPath != srcPath("web") {
		t.Fatalf("web = %+v, want no Clone URL and its local path kept", web)
	}
	// The service decides the rule: a local-only Repository keeps its
	// local path.
	if e := wantError(t, run(t, env, "", "repository", "edit", "web", "--local-path", ""), api.Invalid); e.Field != "clone_url" || e.Fix == "" {
		t.Fatalf("clearing local-only web's local path: %+v, want clone_url refused with a fix", e)
	}
}

// A Repository, the Home included, follows the Default Landing or has its
// Own Landing, and may name its own Push remote (Spec code-central-890).
func TestRepositoryTakesALandingAndPushRemoteAndListsThem(t *testing.T) {
	t.Parallel()
	srv, env := repositoryEnv(t)

	out := printed(t, run(t, env, "", "repository", "add", "web", "--clone-url", "git@github.com:acme/web.git", "--landing", "branch+push", "--push-remote", "upstream"))
	web := lineOf(t, out, "web")
	if !strings.Contains(web, "Branch + Push (own)") || !strings.Contains(web, "upstream") {
		t.Fatalf("add printed:\n%s\nwant web on its Own Landing, Branch + Push, to upstream", out)
	}
	if home := lineOf(t, out, "acme (home)"); !strings.Contains(home, "Merge (default)") || !strings.Contains(home, "origin") {
		t.Fatalf("add printed:\n%s\nwant the Home on the Default Landing, Merge, to origin", out)
	}
	if !strings.Contains(out, "LANDING") || !strings.Contains(out, "PUSH REMOTE") {
		t.Fatalf("list:\n%s\nwant LANDING and PUSH REMOTE columns", out)
	}

	printed(t, run(t, env, "", "repository", "edit", "acme", "--landing", "merge+push"))
	printed(t, run(t, env, "", "repository", "edit", "web", "--landing", "default", "--push-remote", ""))
	p, _ := srv.Client.GetProject(context.Background(), "acme")
	if home := p.Repositories[0]; home.Landing != api.RepositoryLandingMergePush || !home.OwnLanding {
		t.Fatalf("the Home = %+v, want its Own Landing, Merge + Push", home)
	}
	if web := p.Repositories[1]; web.Landing != api.RepositoryLandingMerge || web.OwnLanding || web.OwnPushRemote {
		t.Fatalf("web = %+v, want it back on the Default Landing and the Project's remote", web)
	}

	printed(t, run(t, env, "", "repository", "add", "notes", "--local-path", srcPath("notes"), "--landing", "branch"))
	if e := wantError(t, run(t, env, "", "repository", "edit", "notes", "--landing", "pull-request"), api.Invalid); e.Field != "landing" || !strings.Contains(e.Fix, "notes") {
		t.Fatalf("Pull Request on local-only notes: %+v, want landing refused with a fix naming notes", e)
	}
	if r := run(t, env, "", "repository", "edit", "web", "--landing", "rebase"); r.code != 1 || !strings.Contains(r.stderr, "merge+push") {
		t.Fatalf("an unknown --landing: exit %d, stderr %q; want a usage error listing the Landings", r.code, r.stderr)
	}
}

// lineOf is the line of out that starts with name.
func lineOf(t *testing.T, out, name string) string {
	t.Helper()
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, name+" ") {
			return l
		}
	}
	t.Fatalf("no line for %s in:\n%s", name, out)
	return ""
}

func TestRepositoryAddNeedsACloneURLOrALocalPath(t *testing.T) {
	t.Parallel()
	srv, env := repositoryEnv(t)
	r := run(t, env, "", "repository", "add", "web", "--base", "main")
	if r.code != 1 || !strings.Contains(r.stderr, "needs --clone-url or --local-path") {
		t.Fatalf("add with neither: exit %d, stderr %q, want 1 naming --clone-url and --local-path", r.code, r.stderr)
	}
	if got := landingOrder(t, srv); !slices.Equal(got, []string{"acme"}) {
		t.Fatalf("order = %v, want the Home Repository alone", got)
	}
}

func TestRepositoryRefusalsCarryTheirFix(t *testing.T) {
	t.Parallel()
	_, env := repositoryEnv(t)
	printed(t, run(t, env, "", "repository", "add", "web", "--clone-url", "git@github.com:acme/web.git"))

	e := wantError(t, run(t, env, "", "repository", "add", "api", "--clone-url", "https://github.com/acme/web"), api.Invalid)
	if e.Field != "clone_url" || e.Fix == "" {
		t.Fatalf("error %+v, want clone_url refused with a fix", e)
	}
	if e := wantError(t, run(t, env, "", "repository", "remove", "acme"), api.Invalid); e.Fix == "" {
		t.Fatalf("removing the Home Repository: %+v, want a fix", e)
	}
	if e := wantError(t, run(t, env, "", "repository", "edit", "acme", "--base", "trunk"), api.Invalid); e.Field != "base" {
		t.Fatalf("editing the Home Repository's Base: %+v", e)
	}
	wantError(t, run(t, env, "", "repository", "move", "shop", "--before", "web"), api.NotFound)
}

func TestRepositoryUsageErrorsExitOne(t *testing.T) {
	t.Parallel()
	_, env := repositoryEnv(t)
	for _, args := range [][]string{
		{"repository"},
		{"repository", "rename", "web"},
		{"repository", "add"},
		{"repository", "add", "web", "extra", "--clone-url", "u"},
		{"repository", "edit", "web"},
		{"repository", "move", "web"},
		{"repository", "move", "web", "--before", "acme", "--after", "acme"},
		{"repository", "list", "extra"},
	} {
		if r := run(t, env, "", args...); r.code != 1 {
			t.Errorf("%v: exit %d, want 1 (stderr %q)", args, r.code, r.stderr)
		}
	}
}
