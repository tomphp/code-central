// Package pullrequestwatcher is the Pull Request Watcher (Spec
// code-central-383): the separate process, a hidden subcommand of the
// code-central binary, that opens and follows a Project's pull requests on
// its Code Host. On GitHub it goes through gh, the GitHub CLI, and its own
// login (package gh). The service runs one per Project whose Landing Method
// is Pull Request (Watch, in watch.go). The service makes no external calls
// itself (ADR 0002, ADR 0004), so it runs the Pull Request Watcher once in
// its check mode before Pull Request is turned on (Check, and Checker on the
// service's side). The check never creates a pull request.
package pullrequestwatcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/watchers"
)

// Command is the hidden subcommand the Pull Request Watcher runs as.
const Command = "pull-request-watcher"

// CheckArgs are the arguments that run program's Pull Request Watcher in
// its check mode for in, after the program itself.
func CheckArgs(in core.PullRequestCheckInput) []string {
	args := []string{Command, "--check", "--code-host", in.CodeHost, "--repository", in.Repository}
	if in.GHPath != "" {
		args = append(args, "--gh-path", in.GHPath)
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
// permission there. Being rate-limited fails the check, as there is no
// knowing yet.
func Check(ctx context.Context, in core.PullRequestCheckInput, env gh.Env) core.WatcherCheckResult {
	_, r, err := checkAccess(ctx, in, env)
	var limit *watchers.RateLimitError
	if errors.As(err, &limit) {
		r = limit.CheckResult(time.Now())
	}
	return r
}

// checkAccess is Check, as each poll runs it too, with the gh it found:
// GitHub rate-limiting a call is a *watchers.RateLimitError, not a problem.
func checkAccess(ctx context.Context, in core.PullRequestCheckInput, env gh.Env) (prog string, r core.WatcherCheckResult, err error) {
	if in.CodeHost != core.CodeHostGitHub {
		return "", core.WatcherCheckResult{Problem: core.ProblemCheckFailed,
			Reason: fmt.Sprintf("this Pull Request Watcher can't open pull requests on %q yet: it works with GitHub", in.CodeHost),
			Fix:    "Use Merge or Branch as the Landing Method until it does."}, nil
	}
	prog, missing := gh.Find(in.GHPath, env)
	if missing != nil {
		return "", *missing, nil
	}
	r, err = gh.CheckLogin(ctx, env, prog)
	if r.Problem != "" || err != nil {
		return prog, r, err
	}
	body, msg, err := gh.API(ctx, env, prog, "repos/"+in.Repository)
	if err != nil {
		var limit *watchers.RateLimitError
		if errors.As(err, &limit) {
			return prog, core.WatcherCheckResult{}, limit
		}
		if r, ran := gh.DidntRun(prog, err); !ran {
			return prog, r, nil
		}
		return prog, core.WatcherCheckResult{Problem: core.ProblemRepositoryUnreadable, Reason: fmt.Sprintf("gh can't read the repository %s: %s", in.Repository, msg),
			Fix: "Check the repository is the owner/name GitHub shows, and that the account gh is logged in as can see it."}, nil
	}
	var repo struct {
		Permissions struct {
			Push bool `json:"push"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(body, &repo); err != nil || !repo.Permissions.Push {
		return prog, core.WatcherCheckResult{Problem: core.ProblemNoWrite,
			Reason: fmt.Sprintf("the account gh is logged in as can't write to %s, which opening a pull request needs", in.Repository),
			Fix:    "Log gh in as an account with write access to the repository: `gh auth login`, or ask for write access."}, nil
	}
	return prog, core.WatcherCheckResult{}, nil
}
