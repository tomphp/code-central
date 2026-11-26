package claude_test

import (
	"testing"

	"github.com/tomoram/code-central/factory/harness/claude"
)

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
