package claude

import (
	"strings"

	"github.com/tomoram/code-central/factory/harness"
)

// ResumeCommand is claude --resume session, with the --add-dir flags
// that give a Turn its other Repositories' worktrees.
func (*Harness) ResumeCommand(session string, addDirs []string) string {
	var b strings.Builder
	b.WriteString("claude --resume " + session)
	for _, d := range addDirs {
		b.WriteString(" --add-dir " + harness.JoinArgs([]string{d}))
	}
	return b.String()
}
