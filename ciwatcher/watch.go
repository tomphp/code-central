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
// waits on CI.
const Interval = 2 * time.Minute

// Client is what the CI Watcher calls on Code Central; *client.Client is
// one.
type Client interface {
	GetCIWatch(ctx context.Context, key string) (core.CIWatch, error)
	ReportCIWatcherStatus(ctx context.Context, key string, in core.ReportCIWatcherStatusInput) (core.Written[core.ProjectView], error)
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
// next, so fixing the cause brings it back to Watching by itself.
func Watch(ctx context.Context, cfg Config) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if cfg.Stdin != nil {
		go func() {
			io.Copy(io.Discard, cfg.Stdin)
			cancel()
		}()
	}
	for {
		cfg.poll(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-cfg.Clock.After(Interval):
		}
	}
}

// poll is one poll.
func (cfg Config) poll(ctx context.Context) {
	w, err := cfg.Client.GetCIWatch(ctx, cfg.Project)
	if err != nil {
		cfg.logf("get_ci_watch: %v", err)
		return
	}
	if !w.Tracking {
		return
	}
	r := Check(ctx, core.CICheckInput{Provider: w.Provider, Repository: w.Repository, GHPath: w.GHPath}, cfg.Env)
	if ctx.Err() != nil {
		// Stopped mid-poll: what failed is the stop, not GitHub.
		return
	}
	in := core.ReportCIWatcherStatusInput{State: core.CIWatcherWatching}
	if r.Problem != "" {
		in = core.ReportCIWatcherStatusInput{State: core.CIWatcherError, Problem: r.Problem, Message: r.Reason, Fix: r.Fix}
	}
	if _, err := cfg.Client.ReportCIWatcherStatus(ctx, cfg.Project, in); err != nil && ctx.Err() == nil {
		cfg.logf("report_ci_watcher_status: %v", err)
	}
}

func (cfg Config) logf(format string, args ...any) {
	if cfg.Log != nil {
		fmt.Fprintf(cfg.Log, "ci-watcher %s: "+format+"\n", append([]any{cfg.Project}, args...)...)
	}
}
