package cli_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomoram/code-central/cli"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// init's teaching files, --local and the Project's mode (ticket 41).

const trackerDoc = "docs/agents/issue-tracker.md"

func read(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func exists(dir, rel string) bool {
	_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
	return err == nil
}

func wantMode(t *testing.T, srv *codecentraltest.Server, key string, mode store.Mode) {
	t.Helper()
	p, err := srv.Core.GetProject(context.Background(), key)
	if err != nil || p.Mode != mode {
		t.Fatalf("Project %s: mode %q (%v), want %q", key, p.Mode, err, mode)
	}
}

// wantTeachingFiles checks every docs/agents/ file is the current
// template under a header naming the writing version.
func wantTeachingFiles(t *testing.T, dir string) {
	t.Helper()
	for _, f := range core.TeachingFiles() {
		got := read(t, dir, f.Path)
		header, body, _ := strings.Cut(got, "\n")
		if !strings.HasPrefix(header, "<!--") || !strings.Contains(header, "code-central init "+cli.Version) {
			t.Errorf("%s header %q", f.Path, header)
		}
		if strings.TrimLeft(body, "\n") != f.Body {
			t.Errorf("%s isn't the current template:\n%s", f.Path, got)
		}
	}
}

// wantHooksIn checks our hooks are in exactly one settings file.
func wantHooksIn(t *testing.T, dir, file string) {
	t.Helper()
	for _, f := range []string{".claude/settings.json", ".claude/settings.local.json"} {
		has := exists(dir, f) && strings.Contains(read(t, dir, f), "code-central ping")
		if has != (f == file) {
			t.Errorf("hooks in %s: %v", f, has)
		}
	}
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	return lines[len(lines)-1]
}

func TestInitWritesTheTeachingFilesAndAPointerToThem(t *testing.T) {
	t.Parallel()
	srv := codecentraltest.NewServer(t)
	dir := gitRepo(t, "api")
	write(t, dir, "CLAUDE.md", "# API\n\nBe kind.\n")
	write(t, dir, "AGENTS.md", "agents\n")

	r := ok(t, runIn(t, dir, map[string]string{"CODE_CENTRAL_URL": srv.URL}, "\ny\n", "init"))
	wantMode(t, srv, "api", core.ModeCommitted)
	wantTeachingFiles(t, dir)
	wantHooksIn(t, dir, ".claude/settings.json")
	if got := read(t, dir, "CLAUDE.md"); !strings.HasPrefix(got, "# API\n\nBe kind.\n") || !strings.Contains(got, trackerDoc) {
		t.Fatalf("CLAUDE.md:\n%s", got)
	}
	if read(t, dir, "AGENTS.md") != "agents\n" || exists(dir, "CLAUDE.local.md") {
		t.Fatal("touched AGENTS.md or wrote CLAUDE.local.md")
	}
	if exists(dir, ".git/info/exclude") && strings.Contains(read(t, dir, ".git/info/exclude"), ".mcp.json") {
		t.Fatal("excluded files in committed mode")
	}
	if !strings.Contains(lastLine(r.stdout), "code-central help sandboxes") {
		t.Fatalf("last line %q", lastLine(r.stdout))
	}
}

func TestInitAddsNoSecondPointer(t *testing.T) {
	t.Parallel()
	srv := codecentraltest.NewServer(t)
	dir := gitRepo(t, "api")
	claude := "Issues: see `docs/agents/issue-tracker.md`.\n"
	write(t, dir, "CLAUDE.md", claude)
	ok(t, runIn(t, dir, map[string]string{"CODE_CENTRAL_URL": srv.URL}, "\ny\n", "init"))
	if got := read(t, dir, "CLAUDE.md"); got != claude {
		t.Fatalf("CLAUDE.md changed:\n%s", got)
	}
}

func TestInitLocalKeepsEverythingOutOfTheRepository(t *testing.T) {
	t.Parallel()
	srv := codecentraltest.NewServer(t)
	dir := gitRepo(t, "api")
	env := map[string]string{"CODE_CENTRAL_URL": srv.URL}
	ok(t, runIn(t, dir, env, "\ny\n", "init", "--local"))

	wantMode(t, srv, "api", core.ModeLocal)
	wantURL(t, dir, "api")
	wantTeachingFiles(t, dir)
	wantHooksIn(t, dir, ".claude/settings.local.json")
	if !strings.Contains(read(t, dir, "CLAUDE.local.md"), trackerDoc) || exists(dir, "CLAUDE.md") {
		t.Fatal("pointer not in CLAUDE.local.md alone")
	}
	exclude := read(t, dir, ".git/info/exclude")
	for _, f := range []string{".mcp.json", "docs/agents/issue-tracker.md", "docs/agents/triage-labels.md"} {
		if !strings.Contains(exclude, "/"+f+"\n") {
			t.Errorf("exclude doesn't list %s:\n%s", f, exclude)
		}
	}
	if out := git(t, dir, "status", "--porcelain", "--untracked-files=all"); out != "" {
		t.Fatalf("git sees init's files:\n%s", out)
	}

	// running it again follows the stored mode without the flag, and
	// doesn't list anything twice
	ok(t, runIn(t, dir, env, "", "init"))
	wantHooksIn(t, dir, ".claude/settings.local.json")
	if again := read(t, dir, ".git/info/exclude"); again != exclude {
		t.Fatalf("exclude changed on a re-run:\n%s", again)
	}

	// a fresh clone joins the key and follows the stored mode too
	clone := gitRepo(t, "api")
	ok(t, runIn(t, clone, env, "\ny\n", "init"))
	wantHooksIn(t, clone, ".claude/settings.local.json")
	if out := git(t, clone, "status", "--porcelain", "--untracked-files=all"); out != "" {
		t.Fatalf("git sees init's files in the clone:\n%s", out)
	}
}

func TestAFlagThatConflictsWithTheStoredModeAsksBeforeSwitching(t *testing.T) {
	t.Parallel()
	srv := codecentraltest.NewServer(t)
	dir := gitRepo(t, "api")
	env := map[string]string{"CODE_CENTRAL_URL": srv.URL}
	ok(t, runIn(t, dir, env, "\ny\n", "init"))

	r := ok(t, runIn(t, dir, env, "n\n", "init", "--local"))
	if !strings.Contains(r.stdout, "Switch it to local?") {
		t.Fatalf("no question in %q", r.stdout)
	}
	wantMode(t, srv, "api", core.ModeCommitted)
	wantHooksIn(t, dir, ".claude/settings.json")

	ok(t, runIn(t, dir, env, "y\n", "init", "--local"))
	wantMode(t, srv, "api", core.ModeLocal)
	wantHooksIn(t, dir, ".claude/settings.local.json")
	if !strings.Contains(read(t, dir, ".git/info/exclude"), "/.mcp.json\n") {
		t.Fatal("not excluded after switching to local")
	}

	ok(t, runIn(t, dir, env, "y\n", "init", "--committed"))
	wantMode(t, srv, "api", core.ModeCommitted)
	wantHooksIn(t, dir, ".claude/settings.json")
	if strings.Contains(read(t, dir, ".git/info/exclude"), ".mcp.json") {
		t.Fatal("still excluded after switching back")
	}

	if r := runIn(t, dir, env, "", "init", "--local", "--committed"); r.code != 1 {
		t.Fatalf("both flags: exit %d", r.code)
	}
}

func TestReRunningInitRefreshesWhatItWroteAndAsksAboutAnythingElse(t *testing.T) {
	t.Parallel()
	srv := codecentraltest.NewServer(t)
	dir := gitRepo(t, "api")
	env := map[string]string{"CODE_CENTRAL_URL": srv.URL}
	// the repository already has its own tracker doc
	mine := "# Issue tracker: GitHub\n\nUse gh.\n"
	write(t, dir, trackerDoc, mine)

	r := ok(t, runIn(t, dir, env, "\ny\nn\n", "init"))
	if !strings.Contains(r.stdout, "-Use gh.") || !strings.Contains(r.stdout, "+# Issue tracker: Code Central") {
		t.Fatalf("no diff shown:\n%s", r.stdout)
	}
	if read(t, dir, trackerDoc) != mine {
		t.Fatal("replaced a file the human declined to replace")
	}

	// a known key: the key is left alone, and agreeing replaces the file
	r = ok(t, runIn(t, dir, env, "y\n", "init"))
	if strings.Contains(r.stdout, "Project Key [") {
		t.Fatalf("asked for the key again:\n%s", r.stdout)
	}
	wantTeachingFiles(t, dir)

	// files init wrote, from an older version or edited, are rewritten
	// without asking, and so are the hook entries
	stale := "<!-- Written by code-central init 0.0.1. Re-running init rewrites this file. -->\n\nold\n"
	write(t, dir, "docs/agents/triage-labels.md", stale)
	settings := read(t, dir, ".claude/settings.json")
	write(t, dir, ".claude/settings.json", strings.ReplaceAll(settings, "code-central ping", "code-central hook old-ping"))
	r = ok(t, runIn(t, dir, env, "", "init"))
	if strings.Contains(r.stdout, "[y/N]") {
		t.Fatalf("asked about its own files:\n%s", r.stdout)
	}
	wantTeachingFiles(t, dir)
	if got := read(t, dir, ".claude/settings.json"); got != settings {
		t.Fatalf("hooks not rewritten:\n%s", got)
	}
}

func TestHelpSandboxesCoversSbxAndBind(t *testing.T) {
	t.Parallel()
	r := ok(t, run(t, nil, "", "help", "sandboxes"))
	for _, want := range []string{"sbx policy allow network localhost:7733", "--bind", "CODE_CENTRAL_URL"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("help sandboxes doesn't mention %q", want)
		}
	}
	if r := run(t, nil, "", "help", "nope"); r.code != 1 {
		t.Fatalf("unknown topic: exit %d", r.code)
	}
}

func TestConnectionErrorsPointAtTheSandboxHelp(t *testing.T) {
	t.Parallel()
	r := run(t, map[string]string{"CODE_CENTRAL_URL": "http://" + freeAddr(t), "CODE_CENTRAL_PROJECT": "acme"}, "", "get-ticket", "1")
	if e := wantError(t, r, core.ServiceUnavailable); !strings.Contains(e.Message, "code-central help sandboxes") {
		t.Fatalf("message %q", e.Message)
	}
}
