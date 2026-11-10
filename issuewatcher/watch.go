// The Issue Watcher's watch mode: the loop the service runs one of per
// Project with GitHub Issues on. It talks to Code Central over the HTTP
// API only, as the integration Actor.

package issuewatcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/watchers"
)

// Interval is how long the Issue Watcher waits between polls.
// Rate-limited with no reset time, it doubles its interval each poll, up
// to watchers.MaxRateLimitInterval.
const Interval = 2 * time.Minute

// Client is what the Issue Watcher calls on Code Central, as the
// integration Actor; *client.Client with its Actor set is one.
type Client interface {
	GetIssueWatch(ctx context.Context, key string) (core.IssueWatch, error)
	ReportIssueWatcherStatus(ctx context.Context, key string, in core.ReportWatcherStatusInput) (core.Written[core.ProjectView], error)
}

// Config wires the watch mode.
type Config struct {
	Client  Client
	Project string
	Env     gh.Env
	Clock   clock.Clock
	// Stdin is a pipe from the service: the Issue Watcher exits when it
	// closes, as a stopped Windows scheduled task doesn't kill its
	// children. Nil watches until ctx ends.
	Stdin io.Reader
	// Log receives what goes wrong reaching Code Central.
	Log io.Writer
}

// Kind is the Issue Watcher as the service's supervision runs it.
var Kind = watchers.Kind{Kind: core.WatcherIssues, Args: WatchArgs}

// WatchArgs are the arguments that run program's Issue Watcher in its
// watch mode for project, after the program itself.
func WatchArgs(project string) []string { return []string{Command, "--project", project} }

// Watch polls until its stdin closes or ctx ends. Each poll reads
// get_issue_watch, checks gh can reach the repository's issues, and
// reports Watching or the error with its fix; a failed poll is retried
// on the next, so fixing the cause brings it back to Watching by itself.
// It polls every Interval.
//
// When GitHub rate-limits it, it makes no more calls until the reset
// time GitHub gave, or with none, doubles its interval up to
// watchers.MaxRateLimitInterval; it reports Watching with when it calls
// again, as being rate-limited isn't an error.
func Watch(ctx context.Context, cfg Config) error {
	ctx, cancel := watchers.UntilStdinCloses(ctx, cfg.Stdin)
	defer cancel()
	var pace watchers.Pace
	for {
		reachable, limit := cfg.poll(ctx)
		every, until := pace.Next(cfg.Clock.Now(), Interval, limit)
		if reachable {
			cfg.report(ctx, core.ReportWatcherStatusInput{State: core.WatcherWatching, RateLimitedUntil: until})
		}
		select {
		case <-ctx.Done():
			return nil
		case <-cfg.Clock.After(every):
		}
	}
}

// poll is one poll: whether GitHub could be reached, so the loop reports
// Watching, and its rate limit, if it refused a call for it. A poll that
// finds an error reports it itself.
func (cfg Config) poll(ctx context.Context) (reachable bool, limit *gh.RateLimitError) {
	watch, err := cfg.Client.GetIssueWatch(ctx, cfg.Project)
	if err != nil {
		if ctx.Err() == nil {
			cfg.logf("get_issue_watch: %v", err)
		}
		return false, nil
	}
	if !watch.On {
		return false, nil
	}
	r, err := checkAccess(ctx, core.IssuesCheckInput{Repository: watch.Repository, GHPath: watch.GHPath}, cfg.Env)
	if ctx.Err() != nil {
		// Stopped mid-poll: what failed is the stop, not GitHub.
		return false, nil
	}
	if r.Problem != "" {
		cfg.report(ctx, core.ReportWatcherStatusInput{State: core.WatcherError, Problem: r.Problem, Message: r.Reason, Fix: r.Fix})
		return false, nil
	}
	errors.As(err, &limit)
	return true, limit
}

// report reports the Issue Watcher's status.
func (cfg Config) report(ctx context.Context, in core.ReportWatcherStatusInput) {
	if _, err := cfg.Client.ReportIssueWatcherStatus(ctx, cfg.Project, in); err != nil && ctx.Err() == nil {
		cfg.logf("report_issue_watcher_status: %v", err)
	}
}

func (cfg Config) logf(format string, args ...any) {
	if cfg.Log != nil {
		fmt.Fprintf(cfg.Log, "issue-watcher %s: "+format+"\n", append([]any{cfg.Project}, args...)...)
	}
}
