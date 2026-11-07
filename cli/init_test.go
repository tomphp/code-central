package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomoram/code-central/cli"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
)

// runIn runs one command in-process from dir.
func runIn(t *testing.T, dir string, env map[string]string, stdin string, args ...string) result {
	t.Helper()
	var out, errOut bytes.Buffer
	code := cli.Run(context.Background(), args, cli.Env{
		Stdin:  strings.NewReader(stdin),
		Stdout: &out,
		Stderr: &errOut,
		Getenv: func(k string) string { return env[k] },
		Dir:    dir,
	})
	return result{code, out.String(), errOut.String()}
}

// gitRepo makes a git repository in a new folder called name, with the
// given remotes as name=url pairs.
func gitRepo(t *testing.T, name string, remotes ...string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "-q")
	for _, r := range remotes {
		n, u, _ := strings.Cut(r, "=")
		git(t, dir, "remote", "add", n, u)
	}
	return dir
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// mcpServer reads the code-central entry from dir/.mcp.json.
func mcpServer(t *testing.T, dir string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		MCPServers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatalf(".mcp.json: %v\n%s", err, b)
	}
	return f.MCPServers["code-central"]
}

func wantURL(t *testing.T, dir, key string) {
	t.Helper()
	want := "${CODE_CENTRAL_URL:-http://localhost:7733}/p/" + key + "/mcp"
	if got := mcpServer(t, dir); got["url"] != want || got["type"] != "http" {
		t.Fatalf("code-central server %v, want http %s", got, want)
	}
}

func wantProject(t *testing.T, srv *codecentraltest.Server, key string) {
	t.Helper()
	if _, err := srv.Core.GetProject(context.Background(), key); err != nil {
		t.Fatalf("Project %s: %v", key, err)
	}
}

func ok(t *testing.T, r result) result {
	t.Helper()
	if r.code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", r.code, r.stdout, r.stderr)
	}
	return r
}

func TestInitProposesTheKeyFromTheRepository(t *testing.T) {
	for _, tc := range []struct {
		name    string
		folder  string
		remotes []string
		want    string
	}{
		{"the only remote", "checkout", []string{"upstream=git@github.com:Acme/API.git"}, "api"},
		{"origin among several", "checkout", []string{"fork=https://github.com/me/fork.git", "origin=https://github.com/acme/Web-App/"}, "web-app"},
		{"the folder without origin", "Widget", []string{"a=https://x/a.git", "b=https://x/b.git"}, "widget"},
		{"the folder with no remote", "Gadget", nil, "gadget"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := codecentraltest.NewServer(t)
			dir := gitRepo(t, tc.folder, tc.remotes...)
			r := ok(t, runIn(t, dir, map[string]string{"CODE_CENTRAL_URL": srv.URL}, "\ny\n", "init"))
			if !strings.Contains(r.stdout, "["+tc.want+"]") {
				t.Fatalf("no proposal of %q in %q", tc.want, r.stdout)
			}
			wantProject(t, srv, tc.want)
			wantURL(t, dir, tc.want)
		})
	}
}

func TestInitTakesAnOverride(t *testing.T) {
	srv := codecentraltest.NewServer(t)
	dir := gitRepo(t, "checkout")
	ok(t, runIn(t, dir, map[string]string{"CODE_CENTRAL_URL": srv.URL}, "acme\ny\n", "init"))
	wantProject(t, srv, "acme")
	wantURL(t, dir, "acme")
	if _, err := srv.Core.GetProject(context.Background(), "checkout"); err == nil {
		t.Fatal("the proposed key was created too")
	}

	// --key overrides without the key prompt
	other := gitRepo(t, "checkout")
	r := ok(t, runIn(t, other, map[string]string{"CODE_CENTRAL_URL": srv.URL}, "y\n", "init", "--key", "beta"))
	if strings.Contains(r.stdout, "Project Key [") {
		t.Fatalf("asked for a key: %q", r.stdout)
	}
	wantURL(t, other, "beta")
}

func TestInitRefusesATakenKeyUnlessTheHumanJoinsIt(t *testing.T) {
	srv := codecentraltest.NewServer(t)
	srv.Client.CreateProject(context.Background(), core.CreateProjectInput{Key: "api"})
	env := map[string]string{"CODE_CENTRAL_URL": srv.URL}

	// declining to join asks for an override, which is then created
	dir := gitRepo(t, "api")
	r := ok(t, runIn(t, dir, env, "\nn\napi2\ny\n", "init"))
	if !strings.Contains(r.stdout, "already exists") {
		t.Fatalf("no refusal in %q", r.stdout)
	}
	wantProject(t, srv, "api2")
	wantURL(t, dir, "api2")

	// confirming joins the existing Project without creating anything
	clone := gitRepo(t, "api")
	ok(t, runIn(t, clone, env, "\ny\n", "init"))
	wantURL(t, clone, "api")

	// with no answer, nothing is written
	other := gitRepo(t, "api")
	if r := runIn(t, other, env, "\n", "init"); r.code != 1 {
		t.Fatalf("exit %d: %s", r.code, r.stdout)
	}
	if _, err := os.Stat(filepath.Join(other, ".mcp.json")); err == nil {
		t.Fatal(".mcp.json written without an answer")
	}
}

func TestInitAsksBeforeCreatingAProject(t *testing.T) {
	srv := codecentraltest.NewServer(t)
	dir := gitRepo(t, "api")
	r := runIn(t, dir, map[string]string{"CODE_CENTRAL_URL": srv.URL}, "\nn\n", "init")
	if r.code != 1 || !strings.Contains(r.stdout, "Create Project api?") {
		t.Fatalf("exit %d: %s", r.code, r.stdout)
	}
	if _, err := srv.Core.GetProject(context.Background(), "api"); err == nil {
		t.Fatal("created after the human declined")
	}
	if _, err := os.Stat(filepath.Join(dir, ".mcp.json")); err == nil {
		t.Fatal(".mcp.json written after the human declined")
	}
}

func TestInitAsksAgainAfterAnInvalidKey(t *testing.T) {
	srv := codecentraltest.NewServer(t)
	dir := gitRepo(t, "api")
	ok(t, runIn(t, dir, map[string]string{"CODE_CENTRAL_URL": srv.URL}, "Bad Key\ny\ngood\ny\n", "init"))
	wantURL(t, dir, "good")
}

func TestInitMergesIntoAnExistingMCPFileAndNeverCommits(t *testing.T) {
	srv := codecentraltest.NewServer(t)
	dir := gitRepo(t, "api")
	existing := `{"mcpServers": {"other": {"command": "other-server"}}, "extra": true}`
	os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(existing), 0o644)
	// run from a subdirectory: the file goes at the top of the repository
	sub := filepath.Join(dir, "pkg", "deep")
	os.MkdirAll(sub, 0o755)
	ok(t, runIn(t, sub, map[string]string{"CODE_CENTRAL_URL": srv.URL}, "\ny\n", "init"))

	wantURL(t, dir, "api")
	var doc struct {
		MCPServers map[string]map[string]any `json:"mcpServers"`
		Extra      bool                      `json:"extra"`
	}
	b, _ := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	json.Unmarshal(b, &doc)
	if doc.MCPServers["other"]["command"] != "other-server" || !doc.Extra {
		t.Fatalf("lost existing content: %s", b)
	}
	if out := git(t, dir, "status", "--porcelain"); strings.TrimSpace(out) != "?? .mcp.json" {
		t.Fatalf("git status: %q", out)
	}
	if cmd := exec.Command("git", "rev-parse", "HEAD"); func() error { cmd.Dir = dir; return cmd.Run() }() == nil {
		t.Fatal("init made a commit")
	}
}

func TestInitLeavesAKnownKeyAlone(t *testing.T) {
	srv := codecentraltest.NewServer(t)
	srv.Client.CreateProject(context.Background(), core.CreateProjectInput{Key: "acme"})
	dir := gitRepo(t, "api", "origin=https://github.com/acme/api.git")
	content := `{"mcpServers":{"code-central":{"type":"http","url":"http://custom:1/p/acme/mcp"}}}`
	os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(content), 0o644)

	ok(t, runIn(t, dir, map[string]string{"CODE_CENTRAL_URL": srv.URL}, "", "init"))
	if b, _ := os.ReadFile(filepath.Join(dir, ".mcp.json")); string(b) != content {
		t.Fatalf(".mcp.json changed: %s", b)
	}
	if _, err := srv.Core.GetProject(context.Background(), "api"); err == nil {
		t.Fatal("created a Project for a repository already set up")
	}
}

func TestInitProposesTheFilesKeyWhenTheStoreDoesNotKnowIt(t *testing.T) {
	srv := codecentraltest.NewServer(t)
	dir := gitRepo(t, "api")
	os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(`{"mcpServers":{"code-central":{"type":"http","url":"`+"${CODE_CENTRAL_URL:-http://localhost:7733}/p/acme/mcp"+`"}}}`), 0o644)
	r := ok(t, runIn(t, dir, map[string]string{"CODE_CENTRAL_URL": srv.URL}, "\ny\n", "init"))
	if !strings.Contains(r.stdout, "[acme]") {
		t.Fatalf("proposal: %q", r.stdout)
	}
	wantProject(t, srv, "acme")
}

func TestInitWorksOutsideGit(t *testing.T) {
	srv := codecentraltest.NewServer(t)
	dir := filepath.Join(t.TempDir(), "Notes")
	os.Mkdir(dir, 0o755)
	r := ok(t, runIn(t, dir, map[string]string{"CODE_CENTRAL_URL": srv.URL}, "\ny\n", "init"))
	if !strings.Contains(r.stdout, "[notes]") {
		t.Fatalf("proposal: %q", r.stdout)
	}
	wantProject(t, srv, "notes")
	wantURL(t, dir, "notes")
}

func TestShellCommandsFindTheProjectFromTheNearestMCPFile(t *testing.T) {
	srv := codecentraltest.NewServer(t)
	for _, k := range []string{"acme", "flag", "envkey"} {
		srv.Client.CreateProject(context.Background(), core.CreateProjectInput{Key: k})
	}
	dir := gitRepo(t, "api")
	env := map[string]string{"CODE_CENTRAL_URL": srv.URL}
	ok(t, runIn(t, dir, env, "acme\ny\n", "init"))
	// a nearer .mcp.json without a code-central entry is passed over
	sub := filepath.Join(dir, "a", "b")
	os.MkdirAll(sub, 0o755)
	os.WriteFile(filepath.Join(dir, "a", ".mcp.json"), []byte(`{"mcpServers":{"other":{"command":"x"}}}`), 0o644)

	if spec := decode[core.SpecView](t, runIn(t, sub, env, "", "create-spec", "--title", "S")); spec.ID != "acme-1" {
		t.Fatalf(".mcp.json: %+v", spec)
	}
	env["CODE_CENTRAL_PROJECT"] = "envkey"
	if spec := decode[core.SpecView](t, runIn(t, sub, env, "", "create-spec", "--title", "S")); spec.ID != "envkey-1" {
		t.Fatalf("CODE_CENTRAL_PROJECT: %+v", spec)
	}
	if spec := decode[core.SpecView](t, runIn(t, sub, env, "", "create-spec", "--title", "S", "--project", "flag")); spec.ID != "flag-1" {
		t.Fatalf("--project: %+v", spec)
	}
}

func TestInitReportsAnUnreachableService(t *testing.T) {
	dir := gitRepo(t, "api")
	wantError(t, runIn(t, dir, map[string]string{"CODE_CENTRAL_URL": "http://" + freeAddr(t)}, "\ny\n", "init"), core.ServiceUnavailable)
}
