// The Pull Request Watcher's watch mode: the loop the service runs one of
// per Project under Pull Request. It talks to Code Central over the HTTP
// API only.

package pullrequestwatcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/ratelimit"
	"github.com/tomoram/code-central/watchers"
)

// Interval is how long the Pull Request Watcher waits between polls, and
// WaitingInterval how long while pushed work has no pull request yet,
// matching the CI Watcher's. Rate-limited with no reset time, it doubles
// its interval each poll, up to watchers.MaxRateLimitInterval.
const (
	Interval        = 2 * time.Minute
	WaitingInterval = 30 * time.Second
)

// Client is what the Pull Request Watcher calls on Code Central;
// *client.Client is one.
type Client interface {
	GetPullRequestWatch(ctx context.Context, key string) (api.PullRequestWatch, error)
	ReportPullRequestWatcherStatus(ctx context.Context, key string, in api.ReportWatcherStatusInput) (api.Written[api.ProjectView], error)
	ReportPullRequest(ctx context.Context, key string, in api.ReportPullRequestInput) (api.Written[api.PullRequestReport], error)
}

// Config wires the watch mode.
type Config struct {
	Client  Client
	Project string
	Env     gh.Env
	// AzureProjectURL, when set, is the Azure DevOps project URL to call
	// instead of the one worked out from the repository, for tests.
	AzureProjectURL string
	Clock           clock.Clock
	// Stdin is a pipe from the service: the Pull Request Watcher exits when
	// it closes, as a stopped Windows scheduled task doesn't kill its
	// children. Nil watches until ctx ends.
	Stdin io.Reader
	// Log receives what goes wrong reaching Code Central.
	Log io.Writer
}

// Kind is the Pull Request Watcher as the service's supervision runs it.
var Kind = watchers.Kind{Kind: api.WatcherPullRequests, Args: WatchArgs}

// WatchArgs are the arguments that run program's Pull Request Watcher in
// its watch mode for project, after the program itself.
func WatchArgs(project string) []string { return []string{Command, "--project", project} }

// Watch polls until its stdin closes or ctx ends. Each poll reads
// get_pull_request_watch, checks gh can reach the repository with write
// access, and for each piece of pushed work adopts or opens its pull
// request and reports its state; it then reports Watching or the error
// with its fix. A failed poll is retried on the next, so fixing the cause
// brings it back to Watching by itself. It polls every Interval, and every
// WaitingInterval while pushed work has no pull request yet.
//
// When GitHub rate-limits it, it makes no more calls until the reset time
// GitHub gave, or with none, doubles its interval up to
// watchers.MaxRateLimitInterval; it reports Watching with when it calls
// again, as being rate-limited isn't an error.
func Watch(ctx context.Context, cfg Config) error {
	ctx, cancel := watchers.UntilStdinCloses(ctx, cfg.Stdin)
	defer cancel()
	var pace watchers.Pace
	for {
		reachable, waiting, limit := cfg.poll(ctx)
		every := Interval
		if waiting {
			every = WaitingInterval
		}
		wait, until := pace.Next(cfg.Clock.Now(), every, limit)
		if reachable {
			cfg.report(ctx, api.ReportWatcherStatusInput{State: api.WatcherWatching, RateLimitedUntil: until})
		}
		select {
		case <-ctx.Done():
			return nil
		case <-cfg.Clock.After(wait):
		}
	}
}

// poll is one poll: whether the Code Host could be reached, so the loop
// reports Watching; whether pushed work is still without a pull request, so
// the loop polls sooner; and the rate limit, if it refused a call for it. A
// poll that finds an error reports it itself.
func (cfg Config) poll(ctx context.Context) (reachable, waiting bool, limit *ratelimit.Error) {
	watch, err := cfg.Client.GetPullRequestWatch(ctx, cfg.Project)
	if err != nil {
		if ctx.Err() == nil {
			cfg.logf("get_pull_request_watch: %v", err)
		}
		return false, false, nil
	}
	if !watch.On {
		return false, false, nil
	}
	repositories := watch.Repositories
	if len(repositories) == 0 {
		// A service from before Repositories: the Home Repository alone.
		repositories = []api.PullRequestRepositoryView{{CodeHost: watch.CodeHost, Repository: watch.Repository, Base: watch.Base}}
	}
	several := len(repositories) > 1
	hosts := map[string]Host{}
	var problems []api.RepositoryProblem
	for _, rv := range repositories {
		in := api.PullRequestCheckInput{CodeHost: rv.CodeHost, Repository: rv.Repository, GHPath: watch.GHPath, PATEnv: watch.PATEnv, ProjectURL: cfg.AzureProjectURL}
		h, r, err := access(ctx, in, rv.Base, cfg.Env, false)
		if ctx.Err() != nil {
			// Stopped mid-poll: what failed is the stop, not the Code Host.
			return false, false, nil
		}
		if r.Problem != "" {
			problems = append(problems, api.RepositoryProblem{Repository: rv.Name, Problem: r.Problem, Message: r.Reason, Fix: r.Fix})
			continue
		}
		if errors.As(err, &limit) {
			return true, false, limit
		}
		hosts[rv.Name] = h
	}
	if len(problems) > 0 {
		// A failing Repository is named, and the others are still watched
		// (ADR 0050); one Repository alone reports as it always did.
		in := api.ReportWatcherStatusInput{State: api.WatcherError, Problem: problems[0].Problem, Message: problems[0].Message, Fix: problems[0].Fix}
		if several {
			sum := core.SumRepositoryProblems(problems)
			in.Message, in.Fix, in.Repositories = sum.Reason, sum.Fix, problems
		}
		cfg.report(ctx, in)
	}
	for _, c := range watch.Close {
		g, ok := hosts[c.Repository]
		if !ok {
			continue
		}
		if err := cfg.close(ctx, g, c); errors.As(err, &limit) {
			return len(problems) == 0, waiting, limit
		} else if err != nil && ctx.Err() == nil {
			cfg.logf("%s: %v", c.Link, err)
		}
	}
	for _, w := range watch.Work {
		g, ok := hosts[w.Repository]
		if !ok {
			continue
		}
		pr, err := cfg.follow(ctx, g, w, several)
		switch {
		case errors.As(err, &limit):
			return len(problems) == 0, waiting, limit
		case err != nil:
			if ctx.Err() == nil {
				cfg.logf("%s: %v", w.Branch, err)
			}
		}
		if pr == nil {
			waiting = true
		}
	}
	return len(problems) == 0, waiting, nil
}

// follow brings one piece of pushed work's pull request up to date: it
// finds the pull requests from its branch into the base, adopts the open
// one, else the merged one, and opens one when none is open or merged and
// the latest push is newer than the last closed one; then it reports what
// changed. With several Repositories, a push newer than the merge opens
// one again, as a Ticket sent back after its sibling merged brings new
// work, and an open pull request's body is edited to list its siblings as
// they stand. It returns the pull request as reported, nil when there is
// none.
func (cfg Config) follow(ctx context.Context, g pullRequests, w api.PushedWorkView, several bool) (*pullRequest, error) {
	prs, err := g.list(ctx, w.Branch)
	if err != nil {
		return nil, err
	}
	pr := choose(prs)
	switch {
	case pr == nil, pr.state() == domain.PRClosed && w.Pushed.After(pr.Closed),
		several && pr.state() == domain.PRMerged && !pr.Closed.IsZero() && w.Pushed.After(pr.Closed):
		opened, err := g.open(ctx, w.Branch, w.Title, w.Body)
		if err != nil {
			// Nothing is open yet, so the next poll comes sooner.
			return nil, err
		}
		pr = opened
		pr.Body = w.Body
	case pr.state() == domain.PROpen:
		if pr, err = g.refresh(ctx, pr); err != nil {
			return pr, err
		}
	}
	state := pr.state()
	if several && (state == domain.PROpen || state == domain.PRConflicting) && !g.sameBody(pr.Body, w.Body) {
		if err := g.edit(ctx, pr, w.Body); err != nil {
			return pr, err
		}
	}
	if have := w.PullRequest; have != nil && have.Number == pr.Number && have.Link == pr.Link && have.State == state && have.MergeCommit == pr.mergeCommit() {
		return pr, nil
	}
	_, err = cfg.Client.ReportPullRequest(ctx, cfg.Project, api.ReportPullRequestInput{Repository: w.Repository, Branch: w.Branch, Number: pr.Number, Link: pr.Link, State: state, MergeCommit: pr.mergeCommit()})
	return pr, err
}

// close closes c, a pull request marked to close, unless it is no longer
// open, and reports how it stands, which clears the mark.
func (cfg Config) close(ctx context.Context, g pullRequests, c api.PullRequestToClose) error {
	prs, err := g.list(ctx, c.Branch)
	if err != nil {
		return err
	}
	var pr *pullRequest
	for _, p := range prs {
		if p.Number == c.Number {
			pr = p
		}
	}
	if pr == nil || pr.state() == domain.PROpen || pr.state() == domain.PRConflicting {
		if pr, err = g.close(ctx, c.Number); err != nil {
			return err
		}
	}
	_, err = cfg.Client.ReportPullRequest(ctx, cfg.Project, api.ReportPullRequestInput{Repository: c.Repository, Branch: c.Branch, Number: pr.Number, Link: pr.Link, State: pr.state(), MergeCommit: pr.mergeCommit()})
	return err
}

// choose is the pull request that stands for a branch's pull requests,
// newest first: an open one, else a merged one, else the latest closed.
func choose(prs []*pullRequest) *pullRequest {
	var closed *pullRequest
	var merged *pullRequest
	for _, p := range prs {
		switch p.state() {
		case domain.PROpen, domain.PRConflicting:
			return p
		case domain.PRMerged:
			if merged == nil {
				merged = p
			}
		case domain.PRClosed:
			if closed == nil {
				closed = p
			}
		}
	}
	if merged != nil {
		return merged
	}
	return closed
}

// report reports the Pull Request Watcher's status.
func (cfg Config) report(ctx context.Context, in api.ReportWatcherStatusInput) {
	if _, err := cfg.Client.ReportPullRequestWatcherStatus(ctx, cfg.Project, in); err != nil && ctx.Err() == nil {
		cfg.logf("report_pull_request_watcher_status: %v", err)
	}
}

func (cfg Config) logf(format string, args ...any) {
	if cfg.Log != nil {
		fmt.Fprintf(cfg.Log, "pull-request-watcher %s: "+format+"\n", append([]any{cfg.Project}, args...)...)
	}
}

// pullRequest is a pull request as GitHub's REST API gives it; Azure
// Repos' are mapped into the same fields.
type pullRequest struct {
	Number         int        `json:"number"`
	Link           string     `json:"html_url"`
	Status         string     `json:"state"`
	MergedAt       *time.Time `json:"merged_at"`
	MergeSHA       string     `json:"merge_commit_sha"`
	Closed         time.Time  `json:"closed_at"`
	Body           string     `json:"body"`
	Mergeable      *bool      `json:"mergeable"`
	MergeableState string     `json:"mergeable_state"`
}

// state maps GitHub's state and mergeability to Code Central's: closed
// with a merge time is merged, and an open one GitHub says can't merge
// cleanly is conflicting.
func (p *pullRequest) state() domain.PullRequestState {
	switch {
	case p.MergedAt != nil:
		return domain.PRMerged
	case p.Status == "closed":
		return domain.PRClosed
	case p.MergeableState == "dirty" || (p.Mergeable != nil && !*p.Mergeable):
		return domain.PRConflicting
	}
	return domain.PROpen
}

func (p *pullRequest) mergeCommit() string {
	if p.state() == domain.PRMerged {
		return p.MergeSHA
	}
	return ""
}
