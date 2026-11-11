// Azure Pipelines: the CI Watcher reaches Azure DevOps over its REST API
// (package azdo), with a PAT it reads from its own environment by the
// variable's name, so Code Central stores no PAT (ADR 0027).

package ciwatcher

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"

	"github.com/tomoram/code-central/azdo"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/watchers"
)

// AzurePipelines is the Azure Pipelines CI Provider for one Azure DevOps
// project.
type AzurePipelines struct {
	// ProjectURL is the project URL, as the Project stores it.
	ProjectURL string
	// PATEnv names the environment variable holding the PAT.
	PATEnv string
	// Env is the CI Watcher's environment, which holds the PAT.
	Env gh.Env
}

// CheckAccess checks the PAT variable is set, reads the project, and
// lists one build. Azure DevOps throttling it is a *watchers.RateLimitError,
// not a problem. The execute scope can't be confirmed without retrying a
// run, so it isn't checked.
func (a *AzurePipelines) CheckAccess(ctx context.Context) (core.WatcherCheckResult, error) {
	c, err := azdo.New(a.Env.Environ, a.PATEnv, a.ProjectURL, nil)
	switch {
	case errors.Is(err, azdo.ErrNoPAT):
		return core.WatcherCheckResult{Problem: core.ProblemPATMissing,
			Reason: fmt.Sprintf("The environment variable %s is not set, or is empty, in the CI Watcher's environment.", a.PATEnv),
			Fix:    patMissingFix(runtime.GOOS, a.PATEnv)}, nil
	case err != nil:
		return unreadable(a.ProjectURL, err), nil
	}
	if _, err := c.Project(ctx); err != nil {
		return a.refused(err)
	}
	if _, err := c.Builds(ctx, azdo.BuildQuery{Top: 1}); err != nil {
		return a.refused(err)
	}
	return core.WatcherCheckResult{}, nil
}

// refused is the check's answer to err from a call.
func (a *AzurePipelines) refused(err error) (core.WatcherCheckResult, error) {
	var limit *watchers.RateLimitError
	switch {
	case errors.As(err, &limit):
		return core.WatcherCheckResult{}, limit
	case errors.Is(err, azdo.ErrPATRejected):
		return core.WatcherCheckResult{Problem: core.ProblemPATRejected,
			Reason: fmt.Sprintf("Azure DevOps rejected the PAT in %s: it may have expired or been revoked.", a.PATEnv),
			Fix:    fmt.Sprintf("Make a new PAT with Build: Read & execute, and set it in %s for the service.", a.PATEnv)}, nil
	case errors.Is(err, azdo.ErrMissingScope):
		return core.WatcherCheckResult{Problem: core.ProblemPATScope,
			Reason: fmt.Sprintf("The PAT in %s can't read builds in %s.", a.PATEnv, a.ProjectURL),
			Fix:    "The PAT needs Build: Read & execute. Make a new PAT with that scope, and set it for the service."}, nil
	}
	return unreadable(a.ProjectURL, err), nil
}

// unreadable is the project not found, or its host not reached.
func unreadable(projectURL string, err error) core.WatcherCheckResult {
	return core.WatcherCheckResult{Problem: core.ProblemProjectUnreadable,
		Reason: fmt.Sprintf("Code Central can't read the Azure DevOps project %s: %v", projectURL, err),
		Fix:    "Check the project URL is the one the browser shows, such as https://dev.azure.com/acme/web, and that the PAT's organization can see it."}
}

// patMissingFix is how to give the service the variable on goos, then
// restart it.
func patMissingFix(goos, name string) string {
	var how string
	switch goos {
	case "darwin":
		how = fmt.Sprintf("run `launchctl setenv %s <PAT>`", name)
	case "windows":
		how = fmt.Sprintf("set %s as a user environment variable", name)
	default:
		how = fmt.Sprintf("run `systemctl --user set-environment %s=<PAT>`, or put %s=<PAT> in a file under ~/.config/environment.d/", name, name)
	}
	return fmt.Sprintf("Make an Azure DevOps PAT with Build: Read & execute, %s, then restart the Code Central service.", how)
}

var errAzureRuns = errors.New("Azure Pipelines runs aren't watched yet")

// RunsForCommit is not implemented yet.
func (a *AzurePipelines) RunsForCommit(context.Context, string) ([]Run, error) {
	return nil, errAzureRuns
}

// CompletedRunsOnBranch is not implemented yet.
func (a *AzurePipelines) CompletedRunsOnBranch(context.Context, string, time.Time) ([]Run, error) {
	return nil, errAzureRuns
}

// Jobs is not implemented yet.
func (a *AzurePipelines) Jobs(context.Context, int64, int) ([]core.CIJobView, error) {
	return nil, errAzureRuns
}

// RerunFailedJobs is not implemented yet.
func (a *AzurePipelines) RerunFailedJobs(context.Context, int64) error { return errAzureRuns }
