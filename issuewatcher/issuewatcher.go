// Package issuewatcher is the Issue Watcher (Spec code-central-279): the
// separate process, a hidden subcommand of the code-central binary, that
// links one Project's GitHub issues through gh, the GitHub CLI, and its
// own login (package gh). The service runs one per Project with GitHub
// Issues on (Watch, in watch.go). The service makes no external calls itself
// (ADR 0002, ADR 0004), so it runs the Issue Watcher once in its check
// mode before GitHub Issues is turned on (Check, and Checker on the
// service's side).
package issuewatcher

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

// Command is the hidden subcommand the Issue Watcher runs as.
const Command = "issue-watcher"

// CheckArgs are the arguments that run program's Issue Watcher in its
// check mode for in, after the program itself.
func CheckArgs(in core.IssuesCheckInput) []string {
	args := []string{Command, "--check", "--repository", in.Repository}
	if in.GHPath != "" {
		args = append(args, "--gh-path", in.GHPath)
	}
	return args
}

// Checker is the service's Issues check: program, the service's own
// executable, run once in the Issue Watcher's check mode with environ,
// the service's environment, so gh is found on the service's PATH.
func Checker(program string, environ []string) core.IssuesCheck {
	return func(ctx context.Context, in core.IssuesCheckInput) core.WatcherCheckResult {
		return watchers.Check(ctx, program, environ, CheckArgs(in), core.WatcherIssues)
	}
}

// Check is the check mode: whether the Issue Watcher can work in's
// repository's issues. It passes when gh is found, is logged in, can read
// the repository, the repository has issues on, and gh's login has at
// least triage permission there, which labelling and closing issues need.
// Being rate-limited fails the check, as there is no knowing yet.
func Check(ctx context.Context, in core.IssuesCheckInput, env gh.Env) core.WatcherCheckResult {
	r, err := checkAccess(ctx, in, env)
	var limit *gh.RateLimitError
	if errors.As(err, &limit) {
		r = limit.CheckResult(time.Now())
	}
	return r
}

// checkAccess is Check, as each poll runs it too: GitHub rate-limiting a
// call is a *gh.RateLimitError, not a problem.
func checkAccess(ctx context.Context, in core.IssuesCheckInput, env gh.Env) (core.WatcherCheckResult, error) {
	prog, missing := gh.Find(in.GHPath, env)
	if missing != nil {
		return *missing, nil
	}
	r, err := gh.CheckLogin(ctx, env, prog)
	if r.Problem == "" && err == nil {
		r, err = checkRepository(ctx, env, prog, in.Repository)
	}
	return r, err
}

// ghRepository is what the check reads of a repository from GitHub's
// REST API: whether it has issues on, and what gh's login may do there,
// which GitHub leaves out for a login with no role on it.
type ghRepository struct {
	HasIssues   bool `json:"has_issues"`
	Permissions *struct {
		Admin    bool `json:"admin"`
		Maintain bool `json:"maintain"`
		Push     bool `json:"push"`
		Triage   bool `json:"triage"`
	} `json:"permissions"`
}

// canTriage reports whether the login has triage permission or more.
func (r ghRepository) canTriage() bool {
	p := r.Permissions
	return p != nil && (p.Triage || p.Push || p.Maintain || p.Admin)
}

// checkRepository reads repository through prog, gh, and checks its issues
// are on and gh's login can triage them. GitHub rate-limiting the read is
// a *gh.RateLimitError, not a problem.
func checkRepository(ctx context.Context, env gh.Env, prog, repository string) (core.WatcherCheckResult, error) {
	body, msg, err := gh.API(ctx, env, prog, "repos/"+repository)
	if err != nil {
		var limit *gh.RateLimitError
		if errors.As(err, &limit) {
			return core.WatcherCheckResult{}, limit
		}
		if r, ran := gh.DidntRun(prog, err); !ran {
			return r, nil
		}
		return core.WatcherCheckResult{Problem: core.ProblemRepositoryUnreadable, Reason: fmt.Sprintf("gh can't read the repository %s: %s", repository, msg),
			Fix: "Check the repository is the owner/name GitHub shows, and that the account gh is logged in as can see it."}, nil
	}
	var repo ghRepository
	if err := json.Unmarshal(body, &repo); err != nil {
		return core.WatcherCheckResult{Problem: core.ProblemCheckFailed, Reason: fmt.Sprintf("GitHub's answer for the repository %s can't be read: %v", repository, err),
			Fix: "Check gh works for the user the service runs as, then turn GitHub Issues on again."}, nil
	}
	switch {
	case !repo.HasIssues:
		return core.WatcherCheckResult{Problem: core.ProblemIssuesDisabled, Reason: fmt.Sprintf("The repository %s has its issues turned off.", repository),
			Fix: "Turn Issues on in the repository's Settings on GitHub, under Features, or name the repository whose issues to bring in."}, nil
	case !repo.canTriage():
		return core.WatcherCheckResult{Problem: core.ProblemNoTriage,
			Reason: fmt.Sprintf("The account gh is logged in as can't triage the issues of %s: Code Central labels and closes them, which needs triage permission or more.", repository),
			Fix:    "Give the account gh is logged in as triage permission or more on the repository, or log gh in as an account that has it."}, nil
	}
	return core.WatcherCheckResult{}, nil
}
