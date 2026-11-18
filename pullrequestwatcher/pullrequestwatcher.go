// Package pullrequestwatcher is the Pull Request Watcher (Spec
// code-central-383): the separate process, a hidden subcommand of the
// code-central binary, that opens and follows a Project's pull requests on
// its Code Host. On GitHub it goes through gh, the GitHub CLI, and its own
// login (package gh). The service runs one per Project whose Landing Method
// is Pull Request (Watch, in watch.go). The service makes no external calls
// itself (ADR 0002, ADR 0004), so it runs the Pull Request Watcher once in
// its check mode before Pull Request is turned on (Check, and Checker on the
// service's side). The check never creates a pull request.
//
// Everything one Code Host knows sits behind the Host interface, in
// github.go and azure.go.
package pullrequestwatcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/ratelimit"
	"github.com/tomoram/code-central/watchers"
)

// Command is the hidden subcommand the Pull Request Watcher runs as.
const Command = "pull-request-watcher"

// CheckArgs are the arguments that run program's Pull Request Watcher in
// its check mode for in, after the program itself.
func CheckArgs(in core.PullRequestCheckInput) []string {
	args := []string{Command, "--check", "--code-host", string(in.CodeHost), "--repository", in.Repository}
	if len(in.Repositories) > 0 {
		list, _ := json.Marshal(in.Repositories)
		args = append(args, "--repositories", string(list))
	}
	if in.GHPath != "" {
		args = append(args, "--gh-path", in.GHPath)
	}
	if in.PATEnv != "" {
		args = append(args, "--pat-env", in.PATEnv)
	}
	return args
}

// Checker is the service's Pull Request check: program, the service's own
// executable, run once in the Pull Request Watcher's check mode with
// environ, the service's environment, so gh is found on the service's PATH.
func Checker(program string, environ []string) core.PullRequestCheck {
	return func(ctx context.Context, in core.PullRequestCheckInput) core.WatcherCheckResult {
		return watchers.Check(ctx, program, environ, CheckArgs(in), core.WatcherPullRequests)
	}
}

// Check is the check mode: whether the Pull Request Watcher can open and
// follow pull requests on in's repository. On GitHub it passes when gh is
// found, is logged in, can read the repository and its login has write
// permission there. On Azure Repos it passes when the PAT variable is set,
// the repository can be read and the PAT can contribute to pull requests.
// Being rate-limited fails the check, as there is no knowing yet. Given a
// list of Repositories it checks each of them in place of in's one, and
// answers with each failing Repository's problem.
func Check(ctx context.Context, in core.PullRequestCheckInput, env gh.Env) core.WatcherCheckResult {
	if len(in.Repositories) == 0 {
		return checkOne(ctx, in, env)
	}
	var ps []core.RepositoryProblem
	for _, rv := range in.Repositories {
		one := in
		one.Repositories, one.CodeHost, one.Repository = nil, rv.CodeHost, rv.Repository
		if r := checkOne(ctx, one, env); r.Problem != "" {
			ps = append(ps, core.RepositoryProblem{Repository: rv.Name, Problem: r.Problem, Message: r.Reason, Fix: r.Fix})
		}
	}
	if len(ps) == 0 {
		return core.WatcherCheckResult{}
	}
	return core.SumRepositoryProblems(ps)
}

// checkOne is Check for in's one repository.
func checkOne(ctx context.Context, in core.PullRequestCheckInput, env gh.Env) core.WatcherCheckResult {
	_, r, err := access(ctx, in, "", env, true)
	var limit *ratelimit.Error
	if errors.As(err, &limit) {
		r = limit.CheckResult(time.Now())
	}
	return r
}

// Host is a Code Host: everything the Pull Request Watcher knows about one
// sits behind it, for one repository and one base. How it is reached is
// shared with the other Watchers (packages gh and azdo); its pull requests
// are this watcher's alone.
type Host interface {
	// CheckAccess checks the Pull Request Watcher can open and follow the
	// repository's pull requests: a result with no Problem when it can,
	// else what is wrong and its fix. Only a full check asks whether an
	// Azure Repos PAT can contribute to pull requests, which costs a
	// refused write; each poll reads the repository only. The Code Host
	// rate-limiting it is a *ratelimit.Error, as from every method: not a
	// problem, just a wait. The pull requests are reachable once it passes.
	CheckAccess(ctx context.Context, full bool) (core.WatcherCheckResult, error)
	pullRequests
}

// pullRequests is the repository's pull requests into the base.
type pullRequests interface {
	// list is every pull request from branch into the base, newest first.
	list(ctx context.Context, branch string) ([]*pullRequest, error)
	// refresh is an open pull request read singly, for what a list leaves
	// out; a host that lists it all returns p.
	refresh(ctx context.Context, p *pullRequest) (*pullRequest, error)
	// open opens a pull request ready for review, with title and body.
	open(ctx context.Context, branch, title, body string) (*pullRequest, error)
	// sameBody reports whether a listed pull request's body is body as
	// the host keeps it.
	sameBody(listed, body string) bool
	// edit sets an open pull request's body.
	edit(ctx context.Context, p *pullRequest, body string) error
	// close closes pull request number without merging it.
	close(ctx context.Context, number int) (*pullRequest, error)
}

// NewHost is the Code Host for in's repository and its pull requests into
// base, chosen by in's Code Host. It reaches nothing until CheckAccess:
// GitHub then finds gh with in's gh path Option, and Azure Repos reads the
// PAT from env by in's PAT environment variable.
func NewHost(in core.PullRequestCheckInput, base string, env gh.Env) (Host, error) {
	switch in.CodeHost {
	case core.CodeHostGitHub:
		return &github{ghPath: in.GHPath, env: env, repository: in.Repository, base: base}, nil
	case core.CodeHostAzureRepos:
		return &azureRepos{in: in, env: env, base: base}, nil
	}
	return nil, fmt.Errorf("this Pull Request Watcher can't open pull requests on %q: it works with GitHub and Azure Repos", in.CodeHost)
}

// access is Check, as each poll runs it too, with the Code Host it
// checked, for base: the Code Host's rate-limiting a call is a
// *ratelimit.Error, not a problem.
func access(ctx context.Context, in core.PullRequestCheckInput, base string, env gh.Env, full bool) (Host, core.WatcherCheckResult, error) {
	h, err := NewHost(in, base, env)
	if err != nil {
		return nil, core.WatcherCheckResult{Problem: core.ProblemCheckFailed, Reason: err.Error(), Fix: "Use Merge or Branch as the Landing Method."}, nil
	}
	r, err := h.CheckAccess(ctx, full)
	return h, r, err
}
