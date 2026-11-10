// The service's side of the check mode: it runs its own executable's
// hidden CI Watcher subcommand once and reads the answer.

package ciwatcher

import (
	"context"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/watchers"
)

// Command is the hidden subcommand the CI Watcher runs as.
const Command = "ci-watcher"

// CheckArgs are the arguments that run program's CI Watcher in its check
// mode for in, after the program itself.
func CheckArgs(in core.CICheckInput) []string {
	args := []string{Command, "--check", "--provider", string(in.Provider), "--repository", in.Repository}
	if in.GHPath != "" {
		args = append(args, "--gh-path", in.GHPath)
	}
	return args
}

// Checker is the service's CI check: program, the service's own
// executable, run once in the CI Watcher's check mode with environ, the
// service's environment, so gh is found on the service's PATH.
func Checker(program string, environ []string) core.CICheck {
	return func(ctx context.Context, in core.CICheckInput) core.WatcherCheckResult {
		return watchers.Check(ctx, program, environ, CheckArgs(in), core.WatcherCI)
	}
}
