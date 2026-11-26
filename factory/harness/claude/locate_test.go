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
