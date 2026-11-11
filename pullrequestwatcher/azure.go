// Azure Repos: the Pull Request Watcher reaches it over its REST API
// (package azdo), with a PAT it reads from its own environment by the
// variable's name, so Code Central stores no PAT (ADR 0027).

package pullrequestwatcher

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/tomoram/code-central/azdo"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/store"
	"github.com/tomoram/code-central/watchers"
)

// azureRepos is one Azure Repos repository's pull requests.
type azureRepos struct {
	c          *azdo.Client
	repository string
	base       string
}

// projectURLOf is the Azure DevOps project URL of a repository written
// <organization>/<project>/<repository>, and the repository's name. in's
// project URL, when set, is used instead.
func projectURLOf(in core.PullRequestCheckInput) (projectURL, repository string, ok bool) {
	org, project, repository, ok := azdo.SplitRepository(in.Repository)
	if !ok {
		return "", "", false
	}
	if in.ProjectURL != "" {
		return in.ProjectURL, repository, true
	}
	return "https://dev.azure.com/" + org + "/" + project, repository, true
}

// checkAzureRepos is the Azure Repos check: the PAT variable is set, the
// repository can be read, and the PAT can contribute to pull requests.
func checkAzureRepos(ctx context.Context, in core.PullRequestCheckInput, env gh.Env, full bool) (func(string) host, core.WatcherCheckResult, error) {
	patEnv := in.PATEnv
	if patEnv == "" {
		patEnv = store.DefaultPATEnv
	}
	projectURL, repository, ok := projectURLOf(in)
	if !ok {
		return nil, unreadableRepository(in.Repository, errors.New("it isn't <organization>/<project>/<repository>")), nil
	}
	c, err := azdo.New(env.Environ, patEnv, projectURL, nil)
	switch {
	case errors.Is(err, azdo.ErrNoPAT):
		return nil, core.WatcherCheckResult{Problem: core.ProblemPATMissing,
			Reason: fmt.Sprintf("The environment variable %s is not set, or is empty, in the Pull Request Watcher's environment.", patEnv),
			Fix:    patMissingFix(runtime.GOOS, patEnv)}, nil
	case err != nil:
		return nil, unreadableRepository(in.Repository, err), nil
	}
	if _, err := c.Repository(ctx, repository); err != nil {
		r, err := azureRefused(patEnv, in.Repository, err)
		return nil, r, err
	}
	if !full {
		return func(base string) host { return azureRepos{c: c, repository: repository, base: base} }, core.WatcherCheckResult{}, nil
	}
	if err := c.CanContribute(ctx, repository); err != nil {
		r, err := azureRefused(patEnv, in.Repository, err)
		return nil, r, err
	}
	return func(base string) host { return azureRepos{c: c, repository: repository, base: base} }, core.WatcherCheckResult{}, nil
}

// azureRefused is the check's answer to err from a call. A rate limit is
// returned as the error, not a problem.
func azureRefused(patEnv, repository string, err error) (core.WatcherCheckResult, error) {
	var limit *watchers.RateLimitError
	switch {
	case errors.As(err, &limit):
		return core.WatcherCheckResult{}, limit
	case errors.Is(err, azdo.ErrPATRejected):
		return core.WatcherCheckResult{Problem: core.ProblemPATRejected,
			Reason: fmt.Sprintf("Azure DevOps rejected the PAT in %s: it may have expired or been revoked.", patEnv),
			Fix:    fmt.Sprintf("Make a new PAT with Code: Read & write, and set it in %s for the service.", patEnv)}, nil
	case errors.Is(err, azdo.ErrMissingScope):
		return core.WatcherCheckResult{Problem: core.ProblemPATScope,
			Reason: fmt.Sprintf("The PAT in %s can't contribute to pull requests in %s.", patEnv, repository),
			Fix:    "The PAT needs Code: Read & write. Make a new PAT with that scope, and set it for the service. It may share the variable with Azure Pipelines if it has both scopes."}, nil
	}
	return unreadableRepository(repository, err), nil
}

func unreadableRepository(repository string, err error) core.WatcherCheckResult {
	return core.WatcherCheckResult{Problem: core.ProblemRepositoryUnreadable,
		Reason: fmt.Sprintf("Code Central can't read the Azure Repos repository %s: %v", repository, err),
		Fix:    "Check the Push remote is the repository's URL as Azure Repos shows it, and that the PAT's organization can see it."}
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
	return fmt.Sprintf("Make an Azure DevOps PAT with Code: Read & write, %s, then restart the Code Central service.", how)
}

func ref(branch string) string { return "refs/heads/" + strings.TrimPrefix(branch, "refs/heads/") }

func (a azureRepos) list(ctx context.Context, branch string) ([]*pullRequest, error) {
	prs, err := a.c.PullRequests(ctx, a.repository, ref(branch), ref(a.base))
	if err != nil {
		return nil, fmt.Errorf("listing pull requests from %s: %w", branch, err)
	}
	sort.SliceStable(prs, func(i, j int) bool { return prs[i].Created.After(prs[j].Created) })
	out := make([]*pullRequest, len(prs))
	for i, p := range prs {
		out[i] = a.mapped(p)
	}
	return out, nil
}

// refresh needs no second read: Azure Repos' list has the merge status.
func (a azureRepos) refresh(_ context.Context, p *pullRequest) (*pullRequest, error) { return p, nil }

func (a azureRepos) open(ctx context.Context, branch, title, body string) (*pullRequest, error) {
	p, err := a.c.OpenPullRequest(ctx, a.repository, ref(branch), ref(a.base), title, body)
	if err != nil {
		var limit *watchers.RateLimitError
		if errors.As(err, &limit) {
			return nil, err
		}
		return nil, fmt.Errorf("opening a pull request from %s into %s: %w", branch, a.base, err)
	}
	if p.ID == 0 {
		return nil, fmt.Errorf("opening a pull request from %s into %s: Azure DevOps' answer has no pull request", branch, a.base)
	}
	return a.mapped(p), nil
}

// mapped is p in the watcher's terms: active is open, or conflicting while
// its merge status says conflicts; abandoned is closed; completed is
// merged, at the merge commit Azure Repos reports.
func (a azureRepos) mapped(p azdo.PullRequest) *pullRequest {
	pr := &pullRequest{Number: p.ID, Link: a.c.WebURL(a.repository, p.ID), Status: "open", Closed: p.Closed}
	switch p.Status {
	case "abandoned":
		pr.Status = "closed"
	case "completed":
		pr.Status = "closed"
		closed := p.Closed
		if closed.IsZero() {
			closed = time.Unix(1, 0)
		}
		pr.MergedAt = &closed
		pr.MergeSHA = p.LastMergeCommit.CommitID
	default:
		if p.MergeStatus == "conflicts" {
			no := false
			pr.Mergeable = &no
		}
	}
	return pr
}
