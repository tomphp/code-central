// Azure Repos: the Pull Request Watcher reaches it over its REST API
// (package azdo), with a PAT it reads from its own environment by the
// variable's name, so Code Central stores no PAT (ADR 0027).

package pullrequestwatcher

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tomoram/code-central/azdo"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/ratelimit"
)

// azureRepos is one Azure Repos repository's pull requests into one base.
type azureRepos struct {
	in   core.PullRequestCheckInput
	env  gh.Env
	base string
	// c is the client CheckAccess made, and repository the repository's
	// name in its project.
	c          *azdo.Client
	repository string
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

// codeScope is the PAT scope the Pull Request Watcher needs, as Azure
// DevOps names it.
const codeScope = "Code: Read & write"

// access is how the Pull Request Watcher reaches Azure DevOps.
func (a *azureRepos) access() azdo.Access {
	return azdo.Access{Environ: a.env.Environ, PATEnv: a.in.PATEnv, Watcher: "Pull Request Watcher", Scope: codeScope}
}

// CheckAccess checks the PAT variable is set, the repository can be read,
// and, on a full check, the PAT can contribute to pull requests.
func (a *azureRepos) CheckAccess(ctx context.Context, full bool) (core.WatcherCheckResult, error) {
	projectURL, repository, ok := projectURLOf(a.in)
	if !ok {
		return unreadableRepository(a.in.Repository, errors.New("it isn't <organization>/<project>/<repository>")), nil
	}
	c, r, err := a.access().Ready(projectURL)
	switch {
	case err != nil:
		return unreadableRepository(a.in.Repository, err), nil
	case r.Problem != "":
		return r, nil
	}
	if _, err := c.Repository(ctx, repository); err != nil {
		return a.refused(err)
	}
	if full {
		if err := c.CanContribute(ctx, repository); err != nil {
			return a.refused(err)
		}
	}
	a.c, a.repository = c, repository
	return core.WatcherCheckResult{}, nil
}

// refused is the check's answer to err from a call. A rate limit is
// returned as the error, not a problem.
func (a *azureRepos) refused(err error) (core.WatcherCheckResult, error) {
	var limit *ratelimit.Error
	switch {
	case errors.As(err, &limit):
		return core.WatcherCheckResult{}, limit
	case errors.Is(err, azdo.ErrPATRejected):
		return a.access().Rejected(), nil
	case errors.Is(err, azdo.ErrMissingScope):
		return core.WatcherCheckResult{Problem: core.ProblemPATScope,
			Reason: fmt.Sprintf("The PAT in %s can't contribute to pull requests in %s.", a.access().PATVariable(), a.in.Repository),
			Fix:    "The PAT needs " + codeScope + ". Make a new PAT with that scope, and set it for the service. It may share the variable with Azure Pipelines if it has both scopes."}, nil
	}
	return unreadableRepository(a.in.Repository, err), nil
}

func unreadableRepository(repository string, err error) core.WatcherCheckResult {
	return core.WatcherCheckResult{Problem: core.ProblemRepositoryUnreadable,
		Reason: fmt.Sprintf("Code Central can't read the Azure Repos repository %s: %v", repository, err),
		Fix:    "Check the Push remote is the repository's URL as Azure Repos shows it, and that the PAT's organization can see it."}
}

func ref(branch string) string { return "refs/heads/" + strings.TrimPrefix(branch, "refs/heads/") }

func (a *azureRepos) list(ctx context.Context, branch string) ([]*pullRequest, error) {
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
func (a *azureRepos) refresh(_ context.Context, p *pullRequest) (*pullRequest, error) { return p, nil }

func (a *azureRepos) open(ctx context.Context, branch, title, body string) (*pullRequest, error) {
	p, err := a.c.OpenPullRequest(ctx, a.repository, ref(branch), ref(a.base), title, body)
	if err != nil {
		var limit *ratelimit.Error
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

// edit sets p's description to body, as Azure Repos cuts it. A list may
// give a long description cut short, so it reads p singly first and
// writes only when that differs.
func (a *azureRepos) edit(ctx context.Context, p *pullRequest, body string) error {
	want := azdo.CutDescription(body)
	one, err := a.c.PullRequest(ctx, a.repository, p.Number)
	if err == nil && one.Description == want {
		return nil
	}
	if _, err := a.c.EditPullRequestDescription(ctx, a.repository, p.Number, body); err != nil {
		return fmt.Errorf("editing pull request %d's description: %w", p.Number, err)
	}
	return nil
}

// sameBody reports whether a listed description is body as Azure Repos
// keeps it.
func (a *azureRepos) sameBody(listed, body string) bool { return listed == azdo.CutDescription(body) }

// close abandons pull request number: Azure Repos' close without merging.
func (a *azureRepos) close(ctx context.Context, number int) (*pullRequest, error) {
	p, err := a.c.AbandonPullRequest(ctx, a.repository, number)
	if err != nil {
		return nil, fmt.Errorf("abandoning pull request %d: %w", number, err)
	}
	return a.mapped(p), nil
}

// mapped is p in the watcher's terms: active is open, or conflicting while
// its merge status says conflicts; abandoned is closed; completed is
// merged, at the merge commit Azure Repos reports.
func (a *azureRepos) mapped(p azdo.PullRequest) *pullRequest {
	pr := &pullRequest{Number: p.ID, Link: a.c.WebURL(a.repository, p.ID), Status: "open", Closed: p.Closed, Body: p.Description}
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
