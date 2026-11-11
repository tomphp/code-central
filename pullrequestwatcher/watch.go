// The Pull Request Watcher's watch mode: the loop the service runs one of
// per Project under Pull Request. It talks to Code Central over the HTTP
// API only.

package pullrequestwatcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/store"
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
	GetPullRequestWatch(ctx context.Context, key string) (core.PullRequestWatch, error)
	ReportPullRequestWatcherStatus(ctx context.Context, key string, in core.ReportWatcherStatusInput) (core.Written[core.ProjectView], error)
	ReportPullRequest(ctx context.Context, key string, in core.ReportPullRequestInput) (core.Written[core.PullRequestReport], error)
}

// Config wires the watch mode.
type Config struct {
	Client  Client
	Project string
	Env     gh.Env
	Clock   clock.Clock
	// Stdin is a pipe from the service: the Pull Request Watcher exits when
	// it closes, as a stopped Windows scheduled task doesn't kill its
	// children. Nil watches until ctx ends.
	Stdin io.Reader
	// Log receives what goes wrong reaching Code Central.
	Log io.Writer
}

// Kind is the Pull Request Watcher as the service's supervision runs it.
var Kind = watchers.Kind{Kind: core.WatcherPullRequests, Args: WatchArgs}

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
			cfg.report(ctx, core.ReportWatcherStatusInput{State: core.WatcherWatching, RateLimitedUntil: until})
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
func (cfg Config) poll(ctx context.Context) (reachable, waiting bool, limit *watchers.RateLimitError) {
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
	prog, r, err := checkAccess(ctx, core.PullRequestCheckInput{CodeHost: watch.CodeHost, Repository: watch.Repository, GHPath: watch.GHPath, PATEnv: watch.PATEnv}, cfg.Env)
	if ctx.Err() != nil {
		// Stopped mid-poll: what failed is the stop, not GitHub.
		return false, false, nil
	}
	if r.Problem != "" {
		cfg.report(ctx, core.ReportWatcherStatusInput{State: core.WatcherError, Problem: r.Problem, Message: r.Reason, Fix: r.Fix})
		return false, false, nil
	}
	if errors.As(err, &limit) {
		return true, false, limit
	}
	g := github{prog: prog, env: cfg.Env, repository: watch.Repository, base: watch.Base}
	for _, w := range watch.Work {
		pr, err := cfg.follow(ctx, g, w)
		switch {
		case errors.As(err, &limit):
			return true, waiting, limit
		case err != nil:
			if ctx.Err() == nil {
				cfg.logf("%s: %v", w.Branch, err)
			}
		}
		if pr == nil {
			waiting = true
		}
	}
	return true, waiting, nil
}

// follow brings one piece of pushed work's pull request up to date: it
// finds the pull requests from its branch into the base, adopts the open
// one, else the merged one, and opens one when none is open or merged and
// the latest push is newer than the last closed one; then it reports what
// changed. It returns the pull request as reported, nil when there is none.
func (cfg Config) follow(ctx context.Context, g github, w core.PushedWorkView) (*pullRequest, error) {
	prs, err := g.list(ctx, w.Branch)
	if err != nil {
		return nil, err
	}
	pr := choose(prs)
	switch {
	case pr == nil, pr.state() == store.PRClosed && w.Pushed.After(pr.Closed):
		opened, err := g.open(ctx, w.Branch, w.Title, w.Body)
		if err != nil {
			// Nothing is open yet, so the next poll comes sooner.
			return nil, err
		}
		pr = opened
	case pr.state() == store.PROpen:
		if pr, err = g.withMergeability(ctx, pr); err != nil {
			return pr, err
		}
	}
	state := pr.state()
	if have := w.PullRequest; have != nil && have.Number == pr.Number && have.Link == pr.Link && have.State == state && have.MergeCommit == pr.mergeCommit() {
		return pr, nil
	}
	_, err = cfg.Client.ReportPullRequest(ctx, cfg.Project, core.ReportPullRequestInput{Branch: w.Branch, Number: pr.Number, Link: pr.Link, State: state, MergeCommit: pr.mergeCommit()})
	return pr, err
}

// choose is the pull request that stands for a branch's pull requests,
// newest first: an open one, else a merged one, else the latest closed.
func choose(prs []*pullRequest) *pullRequest {
	var closed *pullRequest
	var merged *pullRequest
	for _, p := range prs {
		switch p.state() {
		case store.PROpen:
			return p
		case store.PRMerged:
			if merged == nil {
				merged = p
			}
		case store.PRClosed:
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
func (cfg Config) report(ctx context.Context, in core.ReportWatcherStatusInput) {
	if _, err := cfg.Client.ReportPullRequestWatcherStatus(ctx, cfg.Project, in); err != nil && ctx.Err() == nil {
		cfg.logf("report_pull_request_watcher_status: %v", err)
	}
}

func (cfg Config) logf(format string, args ...any) {
	if cfg.Log != nil {
		fmt.Fprintf(cfg.Log, "pull-request-watcher %s: "+format+"\n", append([]any{cfg.Project}, args...)...)
	}
}

// github is the repository's pull requests as gh reaches them.
type github struct {
	prog       string
	env        gh.Env
	repository string
	base       string
}

// pullRequest is a pull request as GitHub's REST API gives it.
type pullRequest struct {
	Number         int        `json:"number"`
	Link           string     `json:"html_url"`
	Status         string     `json:"state"`
	MergedAt       *time.Time `json:"merged_at"`
	MergeSHA       string     `json:"merge_commit_sha"`
	Closed         time.Time  `json:"closed_at"`
	Mergeable      *bool      `json:"mergeable"`
	MergeableState string     `json:"mergeable_state"`
}

// state maps GitHub's state and mergeability to Code Central's: closed
// with a merge time is merged, and an open one GitHub says can't merge
// cleanly is conflicting.
func (p *pullRequest) state() store.PullRequestState {
	switch {
	case p.MergedAt != nil:
		return store.PRMerged
	case p.Status == "closed":
		return store.PRClosed
	case p.MergeableState == "dirty" || (p.Mergeable != nil && !*p.Mergeable):
		return store.PRConflicting
	}
	return store.PROpen
}

func (p *pullRequest) mergeCommit() string {
	if p.state() == store.PRMerged {
		return p.MergeSHA
	}
	return ""
}

// list is every pull request from branch into the base, newest first.
func (g github) list(ctx context.Context, branch string) ([]*pullRequest, error) {
	owner, _, _ := strings.Cut(g.repository, "/")
	path := fmt.Sprintf("repos/%s/pulls?state=all&sort=created&direction=desc&per_page=100&head=%s&base=%s",
		g.repository, url.QueryEscape(owner+":"+branch), url.QueryEscape(g.base))
	body, _, err := gh.API(ctx, g.env, g.prog, path)
	if err != nil {
		return nil, fmt.Errorf("listing pull requests from %s: %w", branch, err)
	}
	var prs []*pullRequest
	if len(strings.TrimSpace(string(body))) > 0 {
		if err := json.Unmarshal(body, &prs); err != nil {
			return nil, fmt.Errorf("reading pull requests from %s: %w", branch, err)
		}
	}
	return prs, nil
}

// withMergeability is the open pull request read singly, as only that read
// tells whether GitHub finds it conflicting; a pull request GitHub hasn't
// worked the mergeability of yet reads as open.
func (g github) withMergeability(ctx context.Context, p *pullRequest) (*pullRequest, error) {
	body, _, err := gh.API(ctx, g.env, g.prog, fmt.Sprintf("repos/%s/pulls/%d", g.repository, p.Number))
	if err != nil {
		return p, fmt.Errorf("reading pull request %d: %w", p.Number, err)
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return p, nil
	}
	var one pullRequest
	if err := json.Unmarshal(body, &one); err != nil {
		return p, fmt.Errorf("reading pull request %d: %w", p.Number, err)
	}
	if one.Number != p.Number {
		return p, nil
	}
	return &one, nil
}

// open opens a pull request from branch into the base, ready for review,
// with title and body.
func (g github) open(ctx context.Context, branch, title, body string) (*pullRequest, error) {
	out, msg, err := gh.API(ctx, g.env, g.prog, "repos/"+g.repository+"/pulls", "--method", "POST",
		"-f", "title="+title, "-f", "body="+body, "-f", "head="+branch, "-f", "base="+g.base)
	if err != nil {
		var limit *watchers.RateLimitError
		if errors.As(err, &limit) {
			return nil, err
		}
		return nil, fmt.Errorf("opening a pull request from %s into %s: %w: %s", branch, g.base, err, msg)
	}
	var pr pullRequest
	if err := json.Unmarshal(out, &pr); err != nil || pr.Number == 0 {
		return nil, fmt.Errorf("opening a pull request from %s into %s: GitHub's answer has no pull request", branch, g.base)
	}
	return &pr, nil
}
