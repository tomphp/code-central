// The Work Item Watcher's check mode, and the service's side of it.

package workitemwatcher

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tomoram/code-central/azdo"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/ratelimit"
	"github.com/tomoram/code-central/watchers"
)

// CheckArgs are the arguments that run program's Work Item Watcher in its
// check mode for in, after the program itself. The PAT is never an
// argument: the check reads it from the environment the variable's name
// points at.
func CheckArgs(in core.AzureBoardsCheckInput) []string {
	args := []string{Command, "--check", "--mode", string(in.Mode), "--pat-env", in.PATEnv}
	if in.Mode == domain.AzureBoardsWIQL {
		return append(args, "--project-url", in.ProjectURL, "--where", in.Where)
	}
	return append(args, "--query-url", in.QueryURL)
}

// Checker is the service's Azure Boards check: program, the service's own
// executable, run once in the Work Item Watcher's check mode with
// environ, the service's environment, which holds the PAT.
func Checker(program string, environ []string) core.AzureBoardsCheck {
	return func(ctx context.Context, in core.AzureBoardsCheckInput) core.WatcherCheckResult {
		return watchers.Check(ctx, program, environ, CheckArgs(in), core.WatcherWorkItems)
	}
}

// Check is the check mode: whether the Work Item Watcher can work in's
// work items. In order, the PAT variable is set and not empty, the PAT is
// accepted, a Saved query exists and is a flat one, and the query runs.
// Then, given a first result, a validateOnly write of its tags proves the
// PAT can write work items without writing any; with no results, that is
// left to the first write, whose refusal is an error status with the same
// fix. Being rate-limited fails the check, as there is no knowing yet.
func Check(ctx context.Context, in core.AzureBoardsCheckInput, env gh.Env) core.WatcherCheckResult {
	q, r, err := runQuery(ctx, env, in)
	if err == nil && r.Problem == "" && len(q.ids) > 0 {
		err = q.c.CheckTagWrite(ctx, q.ids[0])
		r = writeProblem(in, err)
	}
	var limit *ratelimit.Error
	if errors.As(err, &limit) {
		return limit.CheckResult(time.Now())
	}
	return r
}

// writeProblem is the problem err, from a validateOnly write to a work
// item, is; none for a nil err.
func writeProblem(in core.AzureBoardsCheckInput, err error) core.WatcherCheckResult {
	var refusal *azdo.RefusedError
	switch {
	case err == nil:
		return core.WatcherCheckResult{}
	case errors.Is(err, azdo.ErrPATRejected):
		return access(nil, in).Rejected()
	case errors.Is(err, azdo.ErrMissingScope):
		return core.WatcherCheckResult{Problem: core.ProblemPATScope,
			Reason: fmt.Sprintf("The PAT in %s can read work items but not write them, which Code Central needs to comment on them and tag them.", in.PATEnv),
			Fix:    fmt.Sprintf("The PAT needs %s. Make a new PAT with that scope, and set it for the service.", scope)}
	case errors.As(err, &refusal):
		return core.WatcherCheckResult{Problem: core.ProblemCheckFailed,
			Reason: "Azure DevOps refused a test write of a work item's tags: " + refusal.Message,
			Fix:    "Check the PAT's account may edit the work items the query returns, then turn Azure Boards on again."}
	}
	return core.WatcherCheckResult{Problem: core.ProblemCheckFailed,
		Reason: fmt.Sprintf("Code Central can't test a write to the Azure Boards work items: %v", err),
		Fix:    "Check Azure DevOps' status, and that the service can reach it, then turn Azure Boards on again."}
}
