package claude_test

import (
	"testing"

	"github.com/tomoram/code-central/factory/harness/claude"
)

func TestLocateFindsClaudeOnThePath(t *testing.T) {
	t.Parallel()
	path, err := claude.Locate(func(name string) (string, bool) {
		if name != "claude" {
			t.Errorf("looked for %q, want claude", name)
		}
		return "/bin/claude.exe", true
	})
	if err != nil || path != "/bin/claude.exe" {
		t.Fatalf("Locate = %q, %v; want /bin/claude.exe", path, err)
	}
}

func TestLocateRefusesWhenClaudeIsNotOnThePath(t *testing.T) {
	t.Parallel()
	_, err := claude.Locate(func(string) (string, bool) { return "", false })
	if err == nil || err.Error() != "claude isn't on PATH" {
		t.Fatalf("Locate fails with %v, want claude isn't on PATH", err)
	}
}

func TestTheResumeCommandResumesTheSessionWithTheOtherWorktrees(t *testing.T) {
	t.Parallel()
	h := claude.New(claude.Options{})
	if got, want := h.ResumeCommand("s1", nil), "claude --resume s1"; got != want {
		t.Errorf("Resume Command %q, want %q", got, want)
	}
	if got, want := h.ResumeCommand("s1", []string{"/wt/api", "/wt/my web"}), `claude --resume s1 --add-dir /wt/api --add-dir "/wt/my web"`; got != want {
		t.Errorf("Resume Command %q, want %q", got, want)
	}
}

func TestClaudeNamesMCPJSONAsItsMCPConfig(t *testing.T) {
	t.Parallel()
	if got := claude.New(claude.Options{}).MCPConfig(); got != ".mcp.json" {
		t.Errorf("MCP config %q, want .mcp.json", got)
	}
}
