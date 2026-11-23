// Package workitemwatcher is the Work Item Watcher (Spec code-central-894):
// the separate process, a hidden subcommand of the code-central binary,
// that links one Project's Azure Boards work items, those its Saved query
// or WIQL filter matches, as Incoming Issues on the Azure Boards Spec. It
// reaches Azure DevOps over its REST API (package azdo), with a PAT it
// reads from its own environment by the variable's name, so Code Central
// stores no PAT (ADR 0027). The service runs one per Project with Azure
// Boards on (Watch). It talks to Code Central over the HTTP API only, as
// the integration Actor, and keeps no state of its own: each poll reads
// get_work_item_watch.
package workitemwatcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/ratelimit"
	"github.com/tomoram/code-central/watchers"
)

// Command is the hidden subcommand the Work Item Watcher runs as.
const Command = "work-item-watcher"

// Interval is how long the Work Item Watcher waits between polls.
// Rate-limited, it waits as long as Azure DevOps says, or with no time
// given, doubles its interval each poll, up to
// watchers.MaxRateLimitInterval.
const Interval = 2 * time.Minute

// Client is what the Work Item Watcher calls on Code Central, as the
// integration Actor; *client.Client with its Actor set is one.
type Client interface {
	GetWorkItemWatch(ctx context.Context, key string) (api.WorkItemWatch, error)
	ReportWorkItemWatcherStatus(ctx context.Context, key string, in api.ReportWatcherStatusInput) (api.Written[api.ProjectView], error)
	CreateLinkedTicket(ctx context.Context, key string, in api.CreateLinkedTicketInput) (api.Written[api.TicketView], error)
	UpdateLinkedTicket(ctx context.Context, key string, in api.UpdateLinkedTicketInput) (api.Written[api.TicketView], error)
	SetExternalState(ctx context.Context, key string, in api.SetExternalStateInput) (api.LinkedRow, error)
	ReportIssueWritten(ctx context.Context, key string, in api.ReportIssueWrittenInput) (api.LinkedRow, error)
}

// Config wires the watch mode.
type Config struct {
	Client  Client
	Project string
	// Env is the Work Item Watcher's environment, which holds the PAT.
	Env   gh.Env
	Clock clock.Clock
	// Stdin is a pipe from the service: the Work Item Watcher exits when
	// it closes, as a stopped Windows scheduled task doesn't kill its
	// children. Nil watches until ctx ends.
	Stdin io.Reader
	// Log receives what goes wrong reaching Code Central or a work item.
	Log io.Writer
}

// Kind is the Work Item Watcher as the service's supervision runs it.
var Kind = watchers.Kind{Kind: api.WatcherWorkItems, Args: WatchArgs}

// WatchArgs are the arguments that run program's Work Item Watcher in its
// watch mode for project, after the program itself.
func WatchArgs(project string) []string { return []string{Command, "--project", project} }

// Watch polls until its stdin closes or ctx ends. Each poll reads
// get_work_item_watch, runs the query, and mirrors its work items as
// Incoming Issues (mirror): new matches arrive, Incoming Issues follow
// their work items' edits, and those that left the results are out of
// scope or deleted. It reports Watching, or the error with its fix; a
// failed poll is retried on the next, so fixing the cause brings it back
// to Watching by itself. It polls every Interval.
//
// When Azure DevOps rate-limits it, it makes no more calls until the time
// Azure DevOps gave, or with none, doubles its interval up to
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
			cfg.report(ctx, api.ReportWatcherStatusInput{State: api.WatcherWatching, RateLimitedUntil: until})
		}
		select {
		case <-ctx.Done():
			return nil
		case <-cfg.Clock.After(every):
		}
	}
}

// poll is one poll: whether Azure DevOps ran the query, or rate-limited
// it, so the loop reports Watching, and the rate limit, if it refused a
// call for it. A poll that finds an error reports it itself.
func (cfg Config) poll(ctx context.Context) (reachable bool, limit *ratelimit.Error) {
	watch, err := cfg.Client.GetWorkItemWatch(ctx, cfg.Project)
	if err != nil {
		if ctx.Err() == nil {
			cfg.logf("get_work_item_watch: %v", err)
		}
		return false, nil
	}
	if !watch.On || watch.Spec == "" {
		return false, nil
	}
	q, r, err := runQuery(ctx, cfg.Env, checkInput(watch))
	switch {
	case ctx.Err() != nil:
		// Stopped mid-poll: what failed is the stop, not Azure DevOps.
		return false, nil
	case errors.As(err, &limit):
		return true, limit
	case r.Problem != "":
		cfg.reportProblem(ctx, r)
		return false, nil
	}
	limit = cfg.mirror(ctx, watch, q)
	var refused []*refusedState
	if limit == nil && ctx.Err() == nil {
		refused, err = cfg.writeBack(ctx, q, watch)
	}
	switch {
	case ctx.Err() != nil:
		return false, nil
	case limit != nil:
		return true, limit
	case errors.As(err, &limit):
		return true, limit
	case err != nil:
		// Only the PAT's scope stops write-back this way.
		cfg.reportProblem(ctx, writeScopeProblem(watch, q))
		return false, nil
	case len(refused) > 0:
		cfg.reportProblem(ctx, stateRefusedProblem(refused))
		return false, nil
	}
	return true, nil
}

// checkInput is what watch runs, as the Azure Boards check takes it.
func checkInput(watch api.WorkItemWatch) api.AzureBoardsCheckInput {
	return api.AzureBoardsCheckInput{Mode: watch.Mode, QueryURL: watch.QueryURL, ProjectURL: watch.ProjectURL, Where: watch.Where, PATEnv: watch.PATEnv}
}

// report reports the Work Item Watcher's status.
func (cfg Config) report(ctx context.Context, in api.ReportWatcherStatusInput) {
	if _, err := cfg.Client.ReportWorkItemWatcherStatus(ctx, cfg.Project, in); err != nil && ctx.Err() == nil {
		cfg.logf("report_work_item_watcher_status: %v", err)
	}
}

// reportProblem reports the Work Item Watcher in error with r.
func (cfg Config) reportProblem(ctx context.Context, r api.WatcherCheckResult) {
	cfg.report(ctx, api.ReportWatcherStatusInput{State: api.WatcherError, Problem: r.Problem, Message: r.Reason, Fix: r.Fix})
}

func (cfg Config) logf(format string, args ...any) {
	if cfg.Log != nil {
		fmt.Fprintf(cfg.Log, "work-item-watcher %s: "+format+"\n", append([]any{cfg.Project}, args...)...)
	}
}
