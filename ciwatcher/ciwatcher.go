// Package ciwatcher is the CI Watcher (Spec code-central-247): the
// separate process, a hidden subcommand of the code-central binary, that
// watches one Project's CI through its CI Provider. The service makes no
// external calls itself (ADR 0002, ADR 0004), so it runs the CI Watcher
// once in its check mode before CI Tracking is turned on (Check, and
// Checker on the service's side).
//
// Everything GitHub-specific sits behind the Provider interface, in
// github.go; GitHub Actions is the only CI Provider.
package ciwatcher

import (
	"context"
	"errors"
	"fmt"
	"github.com/tomoram/code-central/watchers"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/store"
)

// Provider is a CI Provider: everything the CI Watcher knows about one
// CI service sits behind it.
type Provider interface {
	// CheckAccess checks the CI Watcher can watch the repository: a
	// result with no Problem when it can, else what is wrong and its fix.
	// The CI Provider rate-limiting it is a *watchers.RateLimitError, as from
	// every method: not a problem, just a wait.
	CheckAccess(ctx context.Context) (core.WatcherCheckResult, error)
	// RunsForCommit lists the workflow runs started for commit, with
	// their statuses and conclusions mapped to Code Central's and no jobs.
	RunsForCommit(ctx context.Context, commit string) ([]Run, error)
	// CompletedRunsOnBranch lists the completed workflow runs on exactly
	// branch that finished after since, mapped as RunsForCommit's are.
	CompletedRunsOnBranch(ctx context.Context, branch string, since time.Time) ([]Run, error)
	// Jobs lists the jobs of one attempt of a run.
	Jobs(ctx context.Context, id int64, attempt int) ([]core.CIJobView, error)
	// RerunFailedJobs re-runs the failed jobs of a run, as its next
	// attempt.
	RerunFailedJobs(ctx context.Context, id int64) error
}

// Run is one workflow run as a CI Provider lists it: the CI Run to
// report, with nothing tracked yet and no jobs, and when the provider
// last changed it, so the CI Watcher fetches jobs only for runs that
// changed.
type Run struct {
	core.CIRunView
	Updated time.Time
}

// NewProvider is the CI Provider provider for repository, finding gh
// with ghPath, the gh path Option.
func NewProvider(provider store.CIProvider, repository, ghPath string, env gh.Env) (Provider, error) {
	switch provider {
	case store.CIGitHubActions:
		return &GitHubActions{Repository: repository, GHPath: ghPath, Env: env}, nil
	default:
		return nil, fmt.Errorf("CI Provider %q isn't %q, the only one", provider, store.CIGitHubActions)
	}
}

// Check is the check mode: whether the CI Watcher can watch in's
// repository through its CI Provider. Being rate-limited fails the check,
// as there is no knowing yet whether the repository can be read.
func Check(ctx context.Context, in core.CICheckInput, env gh.Env) core.WatcherCheckResult {
	p, bad := provider(in, env)
	if bad != nil {
		return *bad
	}
	r, err := p.CheckAccess(ctx)
	var limit *watchers.RateLimitError
	if errors.As(err, &limit) {
		r = limit.CheckResult(time.Now())
	}
	return r
}

// provider is in's CI Provider, or the problem with it.
func provider(in core.CICheckInput, env gh.Env) (Provider, *core.WatcherCheckResult) {
	p, err := NewProvider(in.Provider, in.Repository, in.GHPath, env)
	if err != nil {
		return nil, &core.WatcherCheckResult{Problem: core.ProblemCheckFailed, Reason: err.Error(), Fix: "Choose GitHub Actions as the CI Provider."}
	}
	return p, nil
}
