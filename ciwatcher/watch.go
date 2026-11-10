// The CI Watcher's watch mode: the loop the service runs one of per
// Project with CI Tracking on. It talks to Code Central over the HTTP API
// only, as an ordinary agent client.

package ciwatcher

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

// Interval is how long the CI Watcher waits between polls while nothing
// waits on CI, and WaitingInterval while a pushed commit does. Rate-limited
// with no reset time, it doubles its interval each poll, up to
// MaxRateLimitInterval.
const (
	Interval             = 2 * time.Minute
	WaitingInterval      = 30 * time.Second
	MaxRateLimitInterval = watchers.MaxRateLimitInterval
)

// Client is what the CI Watcher calls on Code Central; *client.Client is
// one.
type Client interface {
	GetCIWatch(ctx context.Context, key string) (core.CIWatch, error)
	ReportCIWatcherStatus(ctx context.Context, key string, in core.ReportWatcherStatusInput) (core.Written[core.ProjectView], error)
	ReportCIRuns(ctx context.Context, key string, in core.ReportCIRunsInput) (core.Written[core.CIRunsView], error)
	FindTickets(ctx context.Context, key string, in core.FindTicketsInput) ([]core.TicketRow, error)
	CreateTicket(ctx context.Context, key string, in core.CreateTicketInput) (core.Written[core.TicketView], error)
	Comment(ctx context.Context, key string, in core.CommentInput) (core.Written[core.TicketView], error)
}

// Config wires the watch mode.
type Config struct {
	Client  Client
	Project string
	Env     gh.Env
	Clock   clock.Clock
	// Stdin is a pipe from the service: the CI Watcher exits when it
	// closes, as a stopped Windows scheduled task doesn't kill its
	// children. Nil watches until ctx ends.
	Stdin io.Reader
	// Log receives what goes wrong reaching Code Central.
	Log io.Writer
}

// Kind is the CI Watcher as the service's supervision runs it.
var Kind = watchers.Kind{Kind: core.WatcherCI, Args: WatchArgs}

// WatchArgs are the arguments that run program's CI Watcher in its watch
// mode for project, after the program itself.
func WatchArgs(project string) []string { return []string{Command, "--project", project} }

// Watch polls until its stdin closes or ctx ends. Each poll reads
// get_ci_watch, checks the CI Provider can be reached, and reports
// Watching or the error with its fix; a failed poll is retried on the
// next, so fixing the cause brings it back to Watching by itself. It
// then carries out each Re-run request, re-running the run's failed
// jobs, lists the runs of each pushed commit still waiting, and reports
// those that changed, a re-run's new attempt among them, which answers
// its request, and fires the CI Rules for failures on their branches.
// It polls every WaitingInterval while a pushed commit waits, and every
// Interval otherwise.
//
// When the CI Provider rate-limits it, it makes no more calls until the
// reset time the provider gave, or with none, doubles its interval up to
// MaxRateLimitInterval; it reports Watching with when it calls again, as
// being rate-limited isn't an error.
func Watch(ctx context.Context, cfg Config) error {
	ctx, cancel := watchers.UntilStdinCloses(ctx, cfg.Stdin)
	defer cancel()
	w := &watcher{Config: cfg, reported: map[runKey]time.Time{}, rerunAsked: map[rerunKey]bool{}, fired: map[runKey]bool{}}
	var pace watchers.Pace
	for {
		res := w.poll(ctx)
		every := Interval
		if res.waiting {
			every = WaitingInterval
		}
		every, until := pace.Next(cfg.Clock.Now(), every, res.limit)
		if res.reachable {
			w.report(ctx, core.ReportWatcherStatusInput{State: core.WatcherWatching, RateLimitedUntil: until})
		}
		select {
		case <-ctx.Done():
			return nil
		case <-cfg.Clock.After(every):
		}
	}
}

// watcher is the watch mode's state between polls.
type watcher struct {
	Config
	// reported is when the CI Provider had last changed each run the CI
	// Watcher reported, so an unchanged run isn't fetched or reported
	// again. A restarted CI Watcher reports each once more, which
	// changes nothing.
	reported map[runKey]time.Time
	// rerunAsked is each Re-run request carried out, so it isn't asked
	// for again while GitHub starts the next attempt. A restarted CI
	// Watcher asks once more, which GitHub refuses for a run already
	// re-running.
	rerunAsked map[rerunKey]bool
	// fired are the runs a CI Rule fired on, so a run whose report
	// failed is reported again without firing again.
	fired map[runKey]bool
}

// rerunKey names one Re-run request: the attempt that failed, and when
// the human asked, so asking again after a request lapsed is carried out.
type rerunKey struct {
	runKey
	requested time.Time
}

// runKey names one attempt of one run.
type runKey struct {
	id      int64
	attempt int
}

// pollResult is how a poll went.
type pollResult struct {
	// waiting is whether a pushed commit waits on CI.
	waiting bool
	// reachable is whether the CI Provider could be reached, so the loop
	// reports Watching; a poll that finds an error reports it itself.
	reachable bool
	// limit is the CI Provider's rate limit, if it refused a call for it.
	limit *gh.RateLimitError
}

// poll is one poll.
func (w *watcher) poll(ctx context.Context) (res pollResult) {
	watch, err := w.Client.GetCIWatch(ctx, w.Project)
	if err != nil {
		w.logf("get_ci_watch: %v", err)
		return res
	}
	if !watch.Tracking {
		return res
	}
	res.waiting = len(watch.Waiting) > 0
	p, bad := provider(core.CICheckInput{Provider: watch.Provider, Repository: watch.Repository, GHPath: watch.GHPath}, w.Env)
	var r core.WatcherCheckResult
	if bad != nil {
		r = *bad
	} else {
		r, err = p.CheckAccess(ctx)
	}
	if ctx.Err() != nil {
		// Stopped mid-poll: what failed is the stop, not GitHub.
		return res
	}
	if r.Problem != "" {
		w.report(ctx, core.ReportWatcherStatusInput{State: core.WatcherError, Problem: r.Problem, Message: r.Reason, Fix: r.Fix})
		return res
	}
	if err == nil {
		err = w.rerunFailed(ctx, p, watch.Reruns)
	}
	if err == nil {
		err = w.reportRuns(ctx, p, watch.Waiting)
	}
	if err == nil {
		err = w.fireRules(ctx, p, watch)
	}
	if ctx.Err() != nil {
		return res
	}
	res.reachable = true
	errors.As(err, &res.limit)
	return res
}

// report reports the CI Watcher's status.
func (w *watcher) report(ctx context.Context, in core.ReportWatcherStatusInput) {
	if _, err := w.Client.ReportCIWatcherStatus(ctx, w.Project, in); err != nil && ctx.Err() == nil {
		w.logf("report_ci_watcher_status: %v", err)
	}
}

// rerunFailed carries out each Re-run request not yet carried out. One
// GitHub refuses is logged and tried again next poll, until the request
// lapses; a rate limit stops it and is returned.
func (w *watcher) rerunFailed(ctx context.Context, p Provider, reruns []core.CIRerun) error {
	for _, r := range reruns {
		k := rerunKey{runKey{r.ID, r.Attempt}, r.Requested}
		if w.rerunAsked[k] {
			continue
		}
		err := p.RerunFailedJobs(ctx, r.ID)
		var limit *gh.RateLimitError
		if errors.As(err, &limit) {
			return limit
		}
		if err != nil {
			if ctx.Err() == nil {
				w.logf("%s: re-running the failed jobs of run %d: %v", r.Ticket, r.ID, err)
			}
			continue
		}
		w.rerunAsked[k] = true
	}
	return nil
}

// reportRuns lists the runs of each waiting pushed commit, fetches the
// jobs of those that changed, and reports them tracked against the
// commit's Ticket. A rate limit stops it calling the CI Provider: it
// reports what changed so far and returns the *gh.RateLimitError.
func (w *watcher) reportRuns(ctx context.Context, p Provider, waiting []core.CIWaitingCommit) error {
	var changed []Run
	var limit *gh.RateLimitError
list:
	for _, c := range waiting {
		runs, err := p.RunsForCommit(ctx, c.Commit)
		if errors.As(err, &limit) {
			break
		}
		if err != nil {
			if ctx.Err() == nil {
				w.logf("%s: listing the runs of %s: %v", c.Ticket, c.Commit, err)
			}
			continue
		}
		for _, run := range runs {
			if at, ok := w.reported[runKey{run.ID, run.Attempt}]; ok && at.Equal(run.Updated) {
				continue
			}
			jobs, err := p.Jobs(ctx, run.ID, run.Attempt)
			if errors.As(err, &limit) {
				break list
			}
			if err != nil {
				if ctx.Err() == nil {
					w.logf("%s: listing the jobs of run %d: %v", c.Ticket, run.ID, err)
				}
				continue
			}
			run.Jobs, run.Ticket = jobs, c.Ticket
			changed = append(changed, run)
		}
	}
	w.reportChanged(ctx, changed)
	if limit != nil {
		return limit
	}
	return nil
}

// reportChanged reports the runs that changed, remembering each Code
// Central took.
func (w *watcher) reportChanged(ctx context.Context, changed []Run) {
	if len(changed) == 0 {
		return
	}
	in := core.ReportCIRunsInput{}
	for _, run := range changed {
		in.Runs = append(in.Runs, run.CIRunView)
	}
	if _, err := w.Client.ReportCIRuns(ctx, w.Project, in); err == nil {
		for _, run := range changed {
			w.reported[runKey{run.ID, run.Attempt}] = run.Updated
		}
		return
	}
	// One run Code Central refuses mustn't hold up the rest: report each
	// on its own, so only the refused one is tried again next poll.
	for _, run := range changed {
		if _, err := w.Client.ReportCIRuns(ctx, w.Project, core.ReportCIRunsInput{Runs: []core.CIRunView{run.CIRunView}}); err != nil {
			if ctx.Err() == nil {
				w.logf("report_ci_runs: run %d: %v", run.ID, err)
			}
			continue
		}
		w.reported[runKey{run.ID, run.Attempt}] = run.Updated
	}
}

func (cfg Config) logf(format string, args ...any) {
	if cfg.Log != nil {
		fmt.Fprintf(cfg.Log, "ci-watcher %s: "+format+"\n", append([]any{cfg.Project}, args...)...)
	}
}
