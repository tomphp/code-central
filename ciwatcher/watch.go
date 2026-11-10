// The CI Watcher's watch mode: the loop the service runs one of per
// Project with CI Tracking on. It talks to Code Central over the HTTP API
// only, as an ordinary agent client.

package ciwatcher

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
)

// Interval is how long the CI Watcher waits between polls while nothing
// waits on CI, and WaitingInterval while a pushed commit does.
const (
	Interval        = 2 * time.Minute
	WaitingInterval = 30 * time.Second
)

// Client is what the CI Watcher calls on Code Central; *client.Client is
// one.
type Client interface {
	GetCIWatch(ctx context.Context, key string) (core.CIWatch, error)
	ReportCIWatcherStatus(ctx context.Context, key string, in core.ReportCIWatcherStatusInput) (core.Written[core.ProjectView], error)
	ReportCIRuns(ctx context.Context, key string, in core.ReportCIRunsInput) (core.Written[core.CIRunsView], error)
	FindTickets(ctx context.Context, key string, in core.FindTicketsInput) ([]core.TicketRow, error)
	CreateTicket(ctx context.Context, key string, in core.CreateTicketInput) (core.Written[core.TicketView], error)
	Comment(ctx context.Context, key string, in core.CommentInput) (core.Written[core.TicketView], error)
}

// Config wires the watch mode.
type Config struct {
	Client  Client
	Project string
	Env     Env
	Clock   clock.Clock
	// Stdin is a pipe from the service: the CI Watcher exits when it
	// closes, as a stopped Windows scheduled task doesn't kill its
	// children. Nil watches until ctx ends.
	Stdin io.Reader
	// Log receives what goes wrong reaching Code Central.
	Log io.Writer
}

// WatchArgs are the arguments that run program's CI Watcher in its watch
// mode for project, after the program itself.
func WatchArgs(project string) []string { return []string{Command, "--project", project} }

// Watch polls until its stdin closes or ctx ends. Each poll reads
// get_ci_watch, checks the CI Provider can be reached, and reports
// Watching or the error with its fix; a failed poll is retried on the
// next, so fixing the cause brings it back to Watching by itself. It then
// lists the runs of each pushed commit still waiting, and reports those
// that changed, and fires the CI Rules for failures on their branches.
// It polls every WaitingInterval while a pushed commit waits, and every
// Interval otherwise.
func Watch(ctx context.Context, cfg Config) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if cfg.Stdin != nil {
		go func() {
			io.Copy(io.Discard, cfg.Stdin)
			cancel()
		}()
	}
	w := &watcher{Config: cfg, reported: map[runKey]time.Time{}, fired: map[runKey]bool{}}
	for {
		every := Interval
		if w.poll(ctx) {
			every = WaitingInterval
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
	// fired are the runs a CI Rule fired on, so a run whose report
	// failed is reported again without firing again.
	fired map[runKey]bool
}

// runKey names one attempt of one run.
type runKey struct {
	id      int64
	attempt int
}

// poll is one poll; it reports whether a pushed commit waits on CI.
func (w *watcher) poll(ctx context.Context) (waiting bool) {
	watch, err := w.Client.GetCIWatch(ctx, w.Project)
	if err != nil {
		w.logf("get_ci_watch: %v", err)
		return false
	}
	if !watch.Tracking {
		return false
	}
	waiting = len(watch.Waiting) > 0
	r := Check(ctx, core.CICheckInput{Provider: watch.Provider, Repository: watch.Repository, GHPath: watch.GHPath}, w.Env)
	if ctx.Err() != nil {
		// Stopped mid-poll: what failed is the stop, not GitHub.
		return waiting
	}
	in := core.ReportCIWatcherStatusInput{State: core.CIWatcherWatching}
	if r.Problem != "" {
		in = core.ReportCIWatcherStatusInput{State: core.CIWatcherError, Problem: r.Problem, Message: r.Reason, Fix: r.Fix}
	}
	if _, err := w.Client.ReportCIWatcherStatus(ctx, w.Project, in); err != nil && ctx.Err() == nil {
		w.logf("report_ci_watcher_status: %v", err)
	}
	if r.Problem != "" {
		return waiting
	}
	p, err := NewProvider(watch.Provider, watch.Repository, watch.GHPath, w.Env)
	if err != nil {
		w.logf("%v", err)
		return waiting
	}
	w.reportRuns(ctx, p, watch.Waiting)
	w.fireRules(ctx, p, watch)
	return waiting
}

// reportRuns lists the runs of each waiting pushed commit, fetches the
// jobs of those that changed, and reports them tracked against the
// commit's Ticket.
func (w *watcher) reportRuns(ctx context.Context, p Provider, waiting []core.CIWaitingCommit) {
	var changed []Run
	for _, c := range waiting {
		runs, err := p.RunsForCommit(ctx, c.Commit)
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
