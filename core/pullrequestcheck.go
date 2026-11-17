package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/tomoram/code-central/store"
)

// The Pull Request Watcher's check mode and status (code-central-389).

// PullRequestCheckInput is what the Pull Request Watcher's check mode
// checks: that it can open and follow pull requests on the Code Host's
// repository, or with Repositories, on each of theirs. On GitHub it goes
// through gh, found with the gh path Option.
type PullRequestCheckInput struct {
	CodeHost   CodeHost `json:"code_host"`
	Repository string   `json:"repository"`
	// Repositories, when given, are the Repositories to check in place
	// of CodeHost and Repository, each answered on its own (Spec
	// code-central-848).
	Repositories []PullRequestCheckRepository `json:"repositories,omitempty"`
	GHPath       string                       `json:"gh_path,omitempty"`
	PATEnv       string                       `json:"pat_env,omitempty"`
	// ProjectURL, when set, is the Azure DevOps project URL to call instead
	// of the one worked out from Repository, for tests.
	ProjectURL string `json:"project_url,omitempty"`
}

// PullRequestCheckRepository is one Repository for the check: its
// Repository Name, and the Code Host and repository there its Clone URL
// names.
type PullRequestCheckRepository struct {
	Name       string   `json:"name"`
	CodeHost   CodeHost `json:"code_host"`
	Repository string   `json:"repository"`
}

// PullRequestCheck runs the Pull Request Watcher once in its check mode.
// The service makes no external calls itself (ADR 0002, ADR 0004), so the
// check runs in the watcher's process. It never creates a pull request.
type PullRequestCheck func(ctx context.Context, in PullRequestCheckInput) WatcherCheckResult

// SetPullRequestCheck makes c run check when Pull Request is turned on.
// Call it before serving; without it, nothing is checked.
func (c *Core) SetPullRequestCheck(check PullRequestCheck) { c.pullRequestCheck = check }

// checkPullRequest runs the Pull Request check on each Repository that
// change, a change of the Project's Default Landing, brings under Pull
// Request (not one already there), and refuses with the reason and fix
// when it fails. It runs outside the store's lock, as the check calls the
// Code Host. GitHub is checked through gh, and Azure Repos with the PAT
// variable. With several Repositories the refusal names each failing
// Repository with its reason and fix. A local-only Repository is refused
// before the check runs, with or without one.
func (c *Core) checkPullRequest(ctx context.Context, key string, change func(store.Project) store.Project) error {
	var url, ghPath, patEnv string
	var repositories []RepositoryView
	var homeUnder, single bool
	var cloneless error
	err := c.store.View(func(r store.Reader) error {
		if err := writable(r, key); err != nil {
			return err
		}
		p, _ := r.Project(key)
		next := change(p)
		cloneless = refuseClonelessFollowers(p, next)
		was, now := landingPolicies(p), landingPolicies(next)
		all := repositoriesView(next)
		for _, rv := range all {
			if now.In(rv.Name).LandsByPullRequest() && !was.In(rv.Name).LandsByPullRequest() {
				repositories = append(repositories, rv)
				homeUnder = homeUnder || rv.Home
			}
		}
		url, ghPath, patEnv, single = next.CI.PushRemoteURL, r.Options().GHPath, patEnvOf(store.CI{PATEnv: next.Landing.PATEnv}), len(all) == 1
		return nil
	})
	if err != nil || len(repositories) == 0 {
		return err
	}
	// The Push remote's own refusals come first, with their own fixes.
	if homeUnder {
		if err := checkPullRequestHost(url); err != nil {
			return err
		}
	}
	if cloneless != nil {
		return cloneless
	}
	if c.pullRequestCheck == nil {
		return nil
	}
	in := PullRequestCheckInput{GHPath: ghPath, PATEnv: patEnv}
	if single {
		in.CodeHost, in.Repository = codeHostOf(url)
		return checkRefusal(c.pullRequestCheck(ctx, in), "method")
	}
	var unsupported []RepositoryProblem
	for _, rv := range repositories {
		host, repository := codeHostOf(rv.CloneURL)
		if host == CodeHostNone {
			unsupported = append(unsupported, RepositoryProblem{Repository: rv.Name, Problem: ProblemCheckFailed,
				Message: rv.CloneURL + " is on a host Pull Request doesn't support; it supports GitHub and Azure Repos", Fix: "point " + rv.Name + "'s Clone URL at GitHub or Azure Repos, or use Merge or Branch"})
			continue
		}
		in.Repositories = append(in.Repositories, PullRequestCheckRepository{Name: rv.Name, CodeHost: host, Repository: repository})
	}
	res := c.pullRequestCheck(ctx, in)
	if res.Problem != "" && len(res.Repositories) == 0 {
		// A problem with no Repository named, such as gh missing, is
		// every Repository's.
		return checkRefusal(res, "method")
	}
	ps := append(unsupported, res.Repositories...)
	if len(ps) == 0 {
		return nil
	}
	return checkRefusal(SumRepositoryProblems(ps), "method")
}

// clonelessFix is how to put right the local-only Repository rv under
// Pull Request, called who: another Landing, or a Clone URL.
func clonelessFix(rv RepositoryView, who string) string {
	if rv.OwnLanding {
		return "give " + who + " a Clone URL, or another Landing such as Merge"
	}
	return "give " + who + " an Own Landing such as Merge, or a Clone URL"
}

// refuseCloneless refuses p, as an add or edit of its Repository name
// would leave it, while name is local-only under Pull Request: Pull
// Request is refused as a local-only Repository's Own Landing, and a
// Clone URL can't be cleared or left out while the Repository would be
// under Pull Request. It is refused on landing when the human chose the
// Landing, else on clone_url.
func refuseCloneless(p store.Project, name string, landing bool) error {
	for _, r := range p.RepositoryList() {
		if rv := repositoryView(p, r); rv.Name == name && rv.LocalOnly() && landingPolicyIn(p, name).NeedsCloneURL() {
			field := "clone_url"
			if landing {
				field = "landing"
			}
			return &Error{Code: Invalid, Field: field, Message: name + " is local-only, with no Clone URL, so it can't land by Pull Request", Fix: clonelessFix(rv, name)}
		}
	}
	return nil
}

// refuseClonelessFollowers refuses next, a change of p's Default Landing,
// on method while it brings a local-only Repository following it under
// Pull Request, naming each one with its fix. One already there, as only
// a Home that lost its remote can be, isn't refused again.
func refuseClonelessFollowers(p, next store.Project) error {
	was, now := landingPolicies(p), landingPolicies(next)
	var ps []RepositoryProblem
	for _, rv := range repositoriesView(next) {
		if rv.LocalOnly() && now.In(rv.Name).NeedsCloneURL() && !was.In(rv.Name).NeedsCloneURL() {
			ps = append(ps, RepositoryProblem{Repository: rv.Name, Problem: ProblemCheckFailed,
				Message: "local-only, with no Clone URL, and following the Default Landing, so it can't land by Pull Request", Fix: clonelessFix(rv, "it")})
		}
	}
	return checkRefusal(SumRepositoryProblems(ps), "method")
}

// SumRepositoryProblems is one check result naming each of ps, the
// failing Repositories, with its reason and its fix: its Problem is the
// first one's.
func SumRepositoryProblems(ps []RepositoryProblem) WatcherCheckResult {
	var reasons, fixes []string
	for _, p := range ps {
		reasons = append(reasons, p.Repository+": "+p.Message)
		if p.Fix != "" {
			fixes = append(fixes, p.Repository+": "+p.Fix)
		}
	}
	r := WatcherCheckResult{Reason: strings.Join(reasons, "; "), Fix: strings.Join(fixes, "; "), Repositories: ps}
	if len(ps) > 0 {
		r.Problem = ps[0].Problem
	}
	if len(ps) > 1 {
		r.Reason = fmt.Sprintf("%d Repositories fail: %s", len(ps), r.Reason)
	}
	return r
}

// checkRefusal is a failed check's refusal, with its reason and fix, on
// field, or on pat_env for a problem with the PAT; nil for a check that
// passed.
func checkRefusal(res WatcherCheckResult, field string) error {
	if res.Problem == "" {
		return nil
	}
	switch res.Problem {
	case ProblemPATMissing, ProblemPATRejected, ProblemPATScope:
		field = "pat_env"
	}
	return &Error{Code: Invalid, Field: field, Message: res.Reason, Fix: res.Fix}
}

// ReportPullRequestWatcherStatus is report_pull_request_watcher_status:
// the Pull Request Watcher reports its status on every poll.
func (c *Core) ReportPullRequestWatcherStatus(_ context.Context, key string, in ReportWatcherStatusInput) (Written[ProjectView], error) {
	return c.reportWatcherStatus(WatcherPullRequests, key, in)
}
