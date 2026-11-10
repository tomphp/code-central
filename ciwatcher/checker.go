// The service's side of the check mode: it runs its own executable's
// hidden CI Watcher subcommand once and reads the answer.

package ciwatcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/gh"
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

// checkLimit bounds a check: gh auth status and one API read.
const checkLimit = time.Minute

// Checker is the service's CI check: program, the service's own
// executable, run once in the CI Watcher's check mode with environ, the
// service's environment, so gh is found on the service's PATH.
func Checker(program string, environ []string) core.CICheck {
	return func(ctx context.Context, in core.CICheckInput) core.WatcherCheckResult {
		ctx, cancel := context.WithTimeout(ctx, checkLimit)
		defer cancel()
		cmd := exec.CommandContext(ctx, program, CheckArgs(in)...)
		cmd.Env = environ
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		var r core.WatcherCheckResult
		if err == nil {
			err = json.Unmarshal(stdout.Bytes(), &r)
		}
		if err != nil {
			if line := gh.FirstLine(stderr.String()); line != "" {
				err = fmt.Errorf("%w: %s", err, line)
			}
			return core.WatcherCheckResult{Problem: core.ProblemCheckFailed, Reason: fmt.Sprintf("The CI Watcher's check didn't finish: %v", err),
				Fix: "Check the service's log, then turn CI Tracking on again."}
		}
		return r
	}
}
