package factory

import (
	"strings"

	"github.com/tomoram/code-central/factory/harness"
)

// addDirFlags is the --add-dir flags for dirs as a command line ends
// with them, each after a space, quoted as harness.JoinArgs quotes.
func addDirFlags(dirs []string) string {
	var b strings.Builder
	for _, d := range dirs {
		b.WriteString(" --add-dir " + harness.JoinArgs([]string{d}))
	}
	return b.String()
}
