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

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/ratelimit"
	"github.com/tomoram/code-central/watchers"
)

// Interval is how long the Issue Watcher waits between polls.
// Rate-limited with no reset time, it doubles its interval each poll, up
// to watchers.MaxRateLimitInterval.
const Interval = 2 * time.Minute

// Client is what the Issue Watcher calls on Code Central, as the
// integration Actor; *client.Client with its Actor set is one.
type Client interface {
	GetIssueWatch(ctx context.Context, key string) (api.IssueWatch, error)
	ReportIssueWatcherStatus(ctx context.Context, key string, in api.ReportWatcherStatusInput) (api.Written[api.ProjectView], error)
	FindLinked(ctx context.Context, key string, in api.FindLinkedInput) ([]api.LinkedRow, error)
	CreateLinkedTicket(ctx context.Context, key string, in api.CreateLinkedTicketInput) (api.Written[api.TicketView], error)
	UpdateLinkedTicket(ctx context.Context, key string, in api.UpdateLinkedTicketInput) (api.Written[api.TicketView], error)
	SetExternalState(ctx context.Context, key string, in api.SetExternalStateInput) (api.LinkedRow, error)
	ReportIssueWritten(ctx context.Context, key string, in api.ReportIssueWrittenInput) (api.LinkedRow, error)
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
var Kind = watchers.Kind{Kind: api.WatcherIssues, Args: WatchArgs}

// WatchArgs are the arguments that run program's Issue Watcher in its
// watch mode for project, after the program itself.
func WatchArgs(project string) []string { return []string{Command, "--project", project} }

// Watch polls until its stdin closes or ctx ends. Each poll reads
// get_issue_watch, checks gh can reach the repository's issues, brings in
// the open issues it hasn't yet (arrive), has the Incoming Issues follow
// their issues' edits and states (follow), carries out the writes due on
// linked issues (writeBack), and reports Watching or the error with its
// fix; a failed poll is retried
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
			cfg.report(ctx, api.ReportWatcherStatusInput{State: api.WatcherWatching, RateLimitedUntil: until})
		}
		select {
		case <-ctx.Done():
			return nil
		case <-cfg.Clock.After(every):
		}
	}
}

// polled is a repository whose check passed this poll: the access it
// found and its open issues, which stay nil until they are listed.
type polled struct {
	repository string
	a          access
	issues     []ghIssue
}

// poll is one poll: whether GitHub could be reached, so the loop reports
// Watching, and its rate limit, if it refused a call for it. A poll that
// finds an error reports it itself. Each listed repository is checked on
// its own: one that fails is reported with its own problem and the others
// are still watched (Spec code-central-848).
func (cfg Config) poll(ctx context.Context) (reachable bool, limit *ratelimit.Error) {
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
	var passed []polled
	var problems []api.RepositoryProblem
	for _, repository := range watch.Repositories {
		a, r, err := checkAccess(ctx, repository, watch.GHPath, cfg.Env)
		if ctx.Err() != nil {
			// Stopped mid-poll: what failed is the stop, not GitHub.
			return false, nil
		}
		switch {
		case r.Problem == api.ProblemGHMissing || r.Problem == api.ProblemLoggedOut:
			// gh itself is the problem, for every repository alike.
			cfg.report(ctx, api.ReportWatcherStatusInput{State: api.WatcherError, Problem: r.Problem, Message: r.Reason, Fix: r.Fix})
			return false, nil
		case r.Problem != "":
			problems = append(problems, api.RepositoryProblem{Repository: repository, Problem: r.Problem, Message: r.Reason, Fix: r.Fix})
		case errors.As(err, &limit):
			// A repository that already failed is still reported.
			if len(problems) > 0 {
				cfg.reportProblems(ctx, problems, len(watch.Repositories) > 1)
			}
			return len(problems) == 0, limit
		default:
			passed = append(passed, polled{repository: repository, a: a})
		}
	}
	if len(problems) > 0 {
		cfg.reportProblems(ctx, problems, len(watch.Repositories) > 1)
	}
	reachable = len(problems) == 0
	if watch.Spec == "" || len(passed) == 0 {
		return reachable, nil
	}
	open := map[string]ghIssue{}
	var skipped []string
	for i := range passed {
		p := &passed[i]
		p.issues, err = openIssues(ctx, cfg.Env, p.a.prog, p.repository, watch.Label)
		if err != nil {
			if errors.As(err, &limit) {
				return reachable, limit
			}
			if ctx.Err() == nil {
				cfg.logf("%v", err)
			}
			skipped = append(skipped, p.repository)
			continue
		}
		cfg.arrive(ctx, watch, p.repository, p.issues)
		if limit := cfg.follow(ctx, watch, p.repository, p.a, p.issues); limit != nil {
			return reachable, limit
		}
		for _, i := range p.issues {
			open[LinkID(p.repository, i.Number)] = i
		}
	}
	// Writes on an issue of a repository that failed its check or its
	// listing wait for the next poll.
	for _, p := range problems {
		skipped = append(skipped, p.Repository)
	}
	return reachable, cfg.writeBack(ctx, watch, passed[0].a.prog, open, skipped)
}

// reportProblems reports an error for problems, the repositories that
// failed their checks. One repository alone is reported as it always
// was; several are named, each with its own fix.
func (cfg Config) reportProblems(ctx context.Context, problems []api.RepositoryProblem, several bool) {
	in := api.ReportWatcherStatusInput{State: api.WatcherError, Problem: problems[0].Problem, Message: problems[0].Message, Fix: problems[0].Fix}
	if several {
		sum := core.SumRepositoryProblems(problems)
		in.Message, in.Fix, in.Repositories = sum.Reason, sum.Fix, problems
	}
	cfg.report(ctx, in)
}

// report reports the Issue Watcher's status.
func (cfg Config) report(ctx context.Context, in api.ReportWatcherStatusInput) {
	if _, err := cfg.Client.ReportIssueWatcherStatus(ctx, cfg.Project, in); err != nil && ctx.Err() == nil {
		cfg.logf("report_issue_watcher_status: %v", err)
	}
}

func (cfg Config) logf(format string, args ...any) {
	if cfg.Log != nil {
		fmt.Fprintf(cfg.Log, "issue-watcher %s: "+format+"\n", append([]any{cfg.Project}, args...)...)
	}
}
