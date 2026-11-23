package core

import (
	"context"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core/foundation"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// The Pull Request Watcher's check mode and status (code-central-389).

// SetPullRequestCheck makes c run check when Pull Request is turned on.
// Call it before serving; without it, nothing is checked.
func (c *Core) SetPullRequestCheck(check api.PullRequestCheck) { c.pullRequestCheck = check }

// checkPullRequest runs the Pull Request check on each Repository that
// change, a change of the Project's Default Landing, brings under Pull
// Request (not one already there), and refuses with the reason and fix
// when it fails. It runs outside the store's lock, as the check calls the
// Code Host. GitHub is checked through gh, and Azure Repos with the PAT
// variable. With several Repositories the refusal names each failing
// Repository with its reason and fix. A local-only Repository is refused
// before the check runs, with or without one.
func (c *Core) checkPullRequest(ctx context.Context, key string, change func(domain.Project) domain.Project) error {
	var url, ghPath, patEnv string
	var repositories []api.RepositoryView
	var homeUnder, single bool
	var localOnly error
	err := c.store.View(func(r store.Reader) error {
		if err := foundation.Writable(r, key); err != nil {
			return err
		}
		p, _ := r.Project(key)
		next := change(p)
		localOnly = refuseLocalOnlyFollowers(p, next)
		was, now := landingPolicies(p), landingPolicies(next)
		all := repositoriesView(next)
		for _, rv := range all {
			if now.In(rv.Name).LandsByPullRequest() && !was.In(rv.Name).LandsByPullRequest() {
				repositories = append(repositories, rv)
				// A local-only Home has no Push remote to check: it is
				// refused as local-only.
				homeUnder = homeUnder || rv.Home && !rv.LocalOnly
			}
		}
		url, ghPath, patEnv, single = next.CI.PushRemoteURL, r.Options().GHPath, patEnvOf(domain.CI{PATEnv: next.Landing.PATEnv}), len(all) == 1
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
	if localOnly != nil {
		return localOnly
	}
	if c.pullRequestCheck == nil {
		return nil
	}
	in := api.PullRequestCheckInput{GHPath: ghPath, PATEnv: patEnv}
	if single {
		in.CodeHost, in.Repository = codeHostOf(url)
		return checkRefusal(c.pullRequestCheck(ctx, in), "method")
	}
	var unsupported []api.RepositoryProblem
	for _, rv := range repositories {
		host, repository := codeHostOf(rv.CloneURL)
		if host == api.CodeHostNone {
			unsupported = append(unsupported, api.RepositoryProblem{Repository: rv.Name, Problem: api.ProblemCheckFailed,
				Message: rv.CloneURL + " is on a host Pull Request doesn't support; it supports GitHub and Azure Repos", Fix: "point " + rv.Name + "'s Clone URL at GitHub or Azure Repos, or use Merge or Branch"})
			continue
		}
		in.Repositories = append(in.Repositories, api.PullRequestCheckRepository{Name: rv.Name, CodeHost: host, Repository: repository})
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
	return checkRefusal(api.SumRepositoryProblems(ps), "method")
}

// localOnlyUnderPullRequestFix is how to put right the local-only
// Repository rv under Pull Request: another Landing, or a Clone URL.
func localOnlyUnderPullRequestFix(rv api.RepositoryView) string {
	if rv.OwnLanding {
		return "give " + rv.Name + " a Clone URL, or another Landing such as Merge"
	}
	return "give " + rv.Name + " an Own Landing such as Merge, or a Clone URL"
}

// refuseLocalOnlyUnderPullRequest refuses p, as an add or edit of its
// Repository name would leave it, while name is local-only under Pull
// Request: Pull Request is refused as a local-only Repository's Own
// Landing, and a Clone URL can't be cleared or left out while the
// Repository would be under Pull Request. It is refused on landing when
// the human chose the Landing, else on clone_url.
func refuseLocalOnlyUnderPullRequest(p domain.Project, name string, landing bool) error {
	for _, rv := range repositoriesView(p) {
		if rv.Name == name && rv.LocalOnlyUnderPullRequest {
			field := "clone_url"
			if landing {
				field = "landing"
			}
			return &api.Error{Code: api.Invalid, Field: field, Message: name + " is local-only, with no Clone URL, so it can't land by Pull Request", Fix: localOnlyUnderPullRequestFix(rv)}
		}
	}
	return nil
}

// refuseLocalOnlyFollowers refuses next, a change of p's Default Landing,
// on method while it brings a local-only Repository following it under
// Pull Request, naming each one with its fix. One already there, as only
// a Home that lost its remote can be, isn't refused again.
func refuseLocalOnlyFollowers(p, next domain.Project) error {
	was := landingPolicies(p)
	var ps []api.RepositoryProblem
	for _, rv := range repositoriesView(next) {
		if rv.LocalOnlyUnderPullRequest && !was.In(rv.Name).NeedsCloneURL() {
			ps = append(ps, api.RepositoryProblem{Repository: rv.Name, Problem: api.ProblemCheckFailed,
				Message: "local-only, with no Clone URL, and following the Default Landing, so it can't land by Pull Request", Fix: localOnlyUnderPullRequestFix(rv)})
		}
	}
	return checkRefusal(api.SumRepositoryProblems(ps), "method")
}

// checkRefusal is a failed check's refusal, with its reason and fix, on
// field, or on pat_env for a problem with the PAT; nil for a check that
// passed.
func checkRefusal(res api.WatcherCheckResult, field string) error {
	if res.Problem == "" {
		return nil
	}
	switch res.Problem {
	case api.ProblemPATMissing, api.ProblemPATRejected, api.ProblemPATScope:
		field = "pat_env"
	}
	return &api.Error{Code: api.Invalid, Field: field, Message: res.Reason, Fix: res.Fix}
}

// ReportPullRequestWatcherStatus is report_pull_request_watcher_status:
// the Pull Request Watcher reports its status on every poll.
func (c *Core) ReportPullRequestWatcherStatus(_ context.Context, key string, in api.ReportWatcherStatusInput) (api.Written[api.ProjectView], error) {
	return c.reportWatcherStatus(api.WatcherPullRequests, key, in)
}
