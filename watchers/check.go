// The service's side of every Watcher's check mode: it runs its own
// executable's hidden subcommand once and reads the answer.

package watchers

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

// checkLimit bounds a check: gh auth status and one API read.
const checkLimit = time.Minute

// Check runs program, the service's own executable, once with args, a
// Watcher's check mode, in environ, the service's environment, so gh is
// found on the service's PATH, and reads its result. A check that doesn't
// finish is the check failed problem, naming kind, with the fix of
// turning its setting on again once the service's log is checked.
func Check(ctx context.Context, program string, environ, args []string, kind core.WatcherKind) core.WatcherCheckResult {
	ctx, cancel := context.WithTimeout(ctx, checkLimit)
	defer cancel()
	cmd := exec.CommandContext(ctx, program, args...)
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
		return core.WatcherCheckResult{Problem: core.ProblemCheckFailed, Reason: fmt.Sprintf("The %s's check didn't finish: %v", kind.Name(), err),
			Fix: "Check the service's log, then turn " + kind.Setting() + " on again."}
	}
	return r
}
