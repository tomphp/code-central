package core

import (
	"context"

	"github.com/tomoram/code-central/store"
)

// The Pull Request Watcher's check mode and status (code-central-389).

// PullRequestCheckInput is what the Pull Request Watcher's check mode
// checks: that it can open and follow pull requests on the Code Host's
// repository. On GitHub it goes through gh, found with the gh path Option.
type PullRequestCheckInput struct {
	CodeHost   string `json:"code_host"`
	Repository string `json:"repository"`
	GHPath     string `json:"gh_path,omitempty"`
	PATEnv     string `json:"pat_env,omitempty"`
	// ProjectURL, when set, is the Azure DevOps project URL to call instead
	// of the one worked out from Repository, for tests.
	ProjectURL string `json:"project_url,omitempty"`
}

// PullRequestCheck runs the Pull Request Watcher once in its check mode.
// The service makes no external calls itself (ADR 0002, ADR 0004), so the
// check runs in the watcher's process. It never creates a pull request.
type PullRequestCheck func(ctx context.Context, in PullRequestCheckInput) WatcherCheckResult

// SetPullRequestCheck makes c run check when Pull Request is turned on.
// Call it before serving; without it, nothing is checked.
func (c *Core) SetPullRequestCheck(check PullRequestCheck) { c.pullRequestCheck = check }

// checkPullRequest runs the Pull Request check when l turns Pull Request
// on (not when it already is), and refuses with the reason and fix when it fails. It runs outside
// the store's lock, as the check calls the Code Host. GitHub is checked
// through gh, and Azure Repos with the PAT variable.
func (c *Core) checkPullRequest(ctx context.Context, key string, l store.Landing) error {
	if l.Method != store.LandingPullRequest || c.pullRequestCheck == nil {
		return nil
	}
	var url, ghPath string
	var already bool
	err := c.store.View(func(r store.Reader) error {
		if err := writable(r, key); err != nil {
			return err
		}
		p, _ := r.Project(key)
		url, ghPath, already = p.CI.PushRemoteURL, r.Options().GHPath, p.Landing.Method == store.LandingPullRequest
		return nil
	})
	if err != nil || already {
		return err
	}
	// The Push remote's own refusals come first, with their own fixes.
	if err := checkPullRequestHost(url); err != nil {
		return err
	}
	host, repository := codeHostOf(url)
	res := c.pullRequestCheck(ctx, PullRequestCheckInput{CodeHost: host, Repository: repository, GHPath: ghPath, PATEnv: patEnvOf(store.CI{PATEnv: l.PATEnv})})
	if res.Problem == "" {
		return nil
	}
	field := "method"
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
