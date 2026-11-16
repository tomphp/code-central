package cli_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
)

// The repository command (Spec code-central-848) manages a Project's
// Repositories through the client, as the human, outside the operation
// table.

func repositoryEnv(t *testing.T) (*codecentraltest.Server, map[string]string) {
	t.Helper()
	srv := codecentraltest.NewServer(t)
	if _, err := srv.Client.CreateProject(context.Background(), core.CreateProjectInput{Key: "acme"}); err != nil {
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
}

func TestRepositoryAddEditMoveAndRemove(t *testing.T) {
	t.Parallel()
	srv, env := repositoryEnv(t)

	out := printed(t, run(t, env, "", "repository", "add", "web", "--clone-url", "git@github.com:acme/web.git"))
	if !strings.Contains(out, "git@github.com:acme/web.git") {
		t.Fatalf("add printed:\n%s\nwant the list with web", out)
	}
	printed(t, run(t, env, "", "repository", "add", "api", "--clone-url", "https://github.com/acme/api", "--base", "develop", "--local-path", "/src/api"))
	if got := landingOrder(t, srv); !slices.Equal(got, []string{"acme", "web", "api"}) {
		t.Fatalf("order = %v", got)
	}

	printed(t, run(t, env, "", "repository", "edit", "web", "--base", "trunk", "--local-path", "/src/web"))
	p, _ := srv.Client.GetProject(context.Background(), "acme")
	if web := p.Repositories[1]; web.Base != "trunk" || web.LocalPath != "/src/web" || web.CloneURL != "git@github.com:acme/web.git" {
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

func TestRepositoryRefusalsCarryTheirFix(t *testing.T) {
	t.Parallel()
	_, env := repositoryEnv(t)
	printed(t, run(t, env, "", "repository", "add", "web", "--clone-url", "git@github.com:acme/web.git"))

	e := wantError(t, run(t, env, "", "repository", "add", "api", "--clone-url", "https://github.com/acme/web"), core.Invalid)
	if e.Field != "clone_url" || e.Fix == "" {
		t.Fatalf("error %+v, want clone_url refused with a fix", e)
	}
	if e := wantError(t, run(t, env, "", "repository", "remove", "acme"), core.Invalid); e.Fix == "" {
		t.Fatalf("removing the Home Repository: %+v, want a fix", e)
	}
	if e := wantError(t, run(t, env, "", "repository", "edit", "acme", "--base", "trunk"), core.Invalid); e.Field != "base" {
		t.Fatalf("editing the Home Repository's Base: %+v", e)
	}
	wantError(t, run(t, env, "", "repository", "move", "shop", "--before", "web"), core.NotFound)
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
