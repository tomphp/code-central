package core

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/store"
)

// The CI Watcher's own operations (Spec code-central-247): get_ci_watch,
// the one read it polls, and report_ci_watcher_status. It writes as an
// ordinary agent client; there is no CI Watcher Actor.
//
// Its status lives in memory only, like pings: the CI Watcher is the
// service's child, so a restarted service has no status until its CI
// Watcher reports again, and never shows a stale Watching.

// CIWatcherState is how a Project's CI Watcher last said it was.
type CIWatcherState string

const (
	// CIWatcherNotReported is a CI Watcher that hasn't reported since the
	// service started, or since CI Tracking was turned on.
	CIWatcherNotReported CIWatcherState = "not_reported"
	// CIWatcherWatching is a CI Watcher that can reach its CI Provider.
	CIWatcherWatching CIWatcherState = "watching"
	// CIWatcherError is a CI Watcher that can't, with the message and fix.
	CIWatcherError CIWatcherState = "error"
)

// CIWatcherView is a Project's CI Watcher's status, in its CI settings.
type CIWatcherView struct {
	State CIWatcherState `json:"state"`
	// Problem, Message and Fix say what is wrong, while in error.
	Problem CIProblem `json:"problem,omitempty"`
	Message string    `json:"message,omitempty"`
	Fix     string    `json:"fix,omitempty"`
	// RateLimitedUntil is when GitHub lets a rate-limited CI Watcher call
	// it again; until then it is still Watching, not in error.
	RateLimitedUntil time.Time `json:"rate_limited_until,omitzero"`
	// Since is when the CI Watcher went into this state; nil until it
	// reports.
	Since *time.Time `json:"since,omitempty"`
}

// ciWatchers is every Project's CI Watcher's status, by Project key.
type ciWatchers struct {
	mu  sync.Mutex
	all map[string]ciWatcherStatus
}

// ciWatcherStatus is one CI Watcher's status, with the errors notify
// counts.
type ciWatcherStatus struct {
	view CIWatcherView
	// errors counts the times it went into an error, and lastError is the
	// latest one's message: notify sends one Notification for each, even
	// one that has cleared by the time it looks.
	errors    int
	lastError string
	// crashingUntil is when a crash streak ends: until then the status
	// stays crashed, as the service reported it, and the CI Watcher's
	// own reports are held back, so a CI Watcher that reports Watching
	// between crashes doesn't hide them or notify for each one.
	crashingUntil time.Time
}

// view is the status of key's CI Watcher.
func (w *ciWatchers) view(key string) CIWatcherView {
	w.mu.Lock()
	defer w.mu.Unlock()
	if s, ok := w.all[key]; ok {
		return s.view
	}
	return CIWatcherView{State: CIWatcherNotReported}
}

// reset makes key's status not reported, as when CI Tracking is turned
// off or watches another repository, keeping its count of errors so the
// next one still notifies.
func (w *ciWatchers) reset(key string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if s, ok := w.all[key]; ok {
		s.view = CIWatcherView{State: CIWatcherNotReported}
		s.crashingUntil = time.Time{}
		w.all[key] = s
	}
}

// report makes next, as the CI Watcher reported it, key's status, unless
// a crash streak holds it back, and reports whether it changed.
func (w *ciWatchers) report(key string, next CIWatcherView, now time.Time) bool {
	w.mu.Lock()
	crashing := now.Before(w.all[key].crashingUntil)
	w.mu.Unlock()
	if crashing {
		return false
	}
	return w.set(key, next, now)
}

// crashed makes next, the service's crash report, key's status, holding
// it until until.
func (w *ciWatchers) crashed(key string, next CIWatcherView, until, now time.Time) bool {
	changed := w.set(key, next, now)
	w.mu.Lock()
	defer w.mu.Unlock()
	s := w.all[key]
	s.crashingUntil = until
	w.all[key] = s
	return changed
}

// set makes next key's status, keeping since while the status is the
// same, and reports whether it changed.
func (w *ciWatchers) set(key string, next CIWatcherView, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	s, ok := w.all[key]
	was := s.view
	was.Since = nil
	if ok && was == next {
		return false
	}
	if next.State == CIWatcherError && was.State != CIWatcherError {
		s.errors++
		s.lastError = next.Message
	}
	if !ok || was.State != next.State {
		next.Since = &now
	} else {
		next.Since = s.view.Since
	}
	s.view = next
	w.all[key] = s
	return true
}

// ciProblems are the CI problems an error may name.
var ciProblems = []CIProblem{CIGHMissing, CILoggedOut, CIRepositoryUnreadable, CICheckFailed}

// CIWatch is get_ci_watch: everything the CI Watcher needs on each poll.
// The gh path Option is read on every poll, so changing it needs no
// restart.
type CIWatch struct {
	Project          string           `json:"project"`
	Tracking         bool             `json:"tracking"`
	Provider         store.CIProvider `json:"provider"`
	Repository       string           `json:"repository"`
	IgnoredWorkflows []string         `json:"ignored_workflows"`
	Rules            []CIWatchRule    `json:"rules"`
	GHPath           string           `json:"gh_path"`
	// Waiting are the pushed commits still waiting on a final CI result,
	// or newly pushed, oldest push first: the CI Watcher lists their runs.
	Waiting []CIWaitingCommit `json:"waiting"`
	// Pushed is every commit pushed for a Ticket, whose runs never fire a
	// CI Rule, and Landed every commit a Ticket landed at, which a fix
	// Ticket links; both by Ticket.
	Pushed []CITicketCommit `json:"pushed"`
	Landed []CITicketCommit `json:"landed"`
}

// CIWatchRule is a CI Rule as the CI Watcher reads it: with when the
// latest run tracked against it finished, so it fires the rule only for
// runs finishing after that, and after the rule was created.
type CIWatchRule struct {
	CIRuleView
	// LastRun is nil until a run is tracked against the rule.
	LastRun *time.Time `json:"last_run,omitempty"`
}

// CITicketCommit is a commit pushed for, or landed by, a Ticket on Spec.
type CITicketCommit struct {
	Ticket string `json:"ticket"`
	Spec   string `json:"spec"`
	Commit string `json:"commit"`
}

// GetCIWatch is the CI Watcher's get_ci_watch. With CI Tracking off,
// nothing waits.
func (c *Core) GetCIWatch(_ context.Context, key string) (CIWatch, error) {
	var out CIWatch
	err := c.store.View(func(r store.Reader) error {
		p, ok := r.Project(key)
		if !ok {
			return unknownProject(key)
		}
		v := ciView(key, p.CI, CIWatcherView{})
		out = CIWatch{Project: key, Tracking: v.Tracking, Provider: v.Provider, Repository: v.Repository,
			IgnoredWorkflows: v.IgnoredWorkflows, Rules: []CIWatchRule{}, GHPath: r.Options().GHPath,
			Waiting: []CIWaitingCommit{}, Pushed: []CITicketCommit{}, Landed: []CITicketCommit{}}
		last := ciRulesLastRuns(r.CIRuns(key))
		for i, rule := range v.Rules {
			w := CIWatchRule{CIRuleView: rule}
			if at, ok := last[p.CI.Rules[i].Spec]; ok {
				w.LastRun = &at
			}
			out.Rules = append(out.Rules, w)
		}
		if p.CI.Tracking {
			out.Waiting = ciWaiting(r, key, c.now())
			out.Pushed, out.Landed = ciTicketCommits(r, key)
		}
		return nil
	})
	return out, err
}

// ciRulesLastRuns is when the latest finished run tracked against each
// CI Rule finished, by the number of the rule's Spec.
func ciRulesLastRuns(runs []store.CIRun) map[int]time.Time {
	last := map[int]time.Time{}
	for _, run := range runs {
		if run.Rule != 0 && run.Finished.After(last[run.Rule]) {
			last[run.Rule] = run.Finished
		}
	}
	return last
}

// ciTicketCommits is every commit pushed for a Ticket, and every commit a
// Ticket landed at, in Ticket order.
func ciTicketCommits(r store.Reader, key string) (pushed, landed []CITicketCommit) {
	pushed, landed = []CITicketCommit{}, []CITicketCommit{}
	for _, t := range r.Tickets(key) {
		id, spec := DisplayID(key, t.Number), DisplayID(key, t.Spec)
		for _, p := range t.PushedCommits {
			pushed = append(pushed, CITicketCommit{Ticket: id, Spec: spec, Commit: p.Commit})
		}
		for _, m := range t.Moves {
			if m.Commit != "" {
				landed = append(landed, CITicketCommit{Ticket: id, Spec: spec, Commit: m.Commit})
			}
		}
	}
	return pushed, landed
}

// ReportCIWatcherStatusInput is the CI Watcher's
// report_ci_watcher_status(state, problem?, message?, fix?,
// rate_limited_until?, full?).
type ReportCIWatcherStatusInput struct {
	// State is watching or error.
	State   CIWatcherState `json:"state"`
	Problem CIProblem      `json:"problem,omitempty"`
	// Message and Fix are an error's; watching drops them.
	Message string `json:"message,omitempty"`
	Fix     string `json:"fix,omitempty"`
	// RateLimitedUntil is when GitHub lets a Watching CI Watcher call it
	// again, while it is rate-limited.
	RateLimitedUntil time.Time `json:"rate_limited_until,omitzero"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// ReportCIWatcherStatus is report_ci_watcher_status: the CI Watcher
// reports on every poll, and a change publishes a notice. Going into an
// error is counted, whoever reports it, so notify sends one Notification;
// being rate-limited is Watching, so it sends none.
func (c *Core) ReportCIWatcherStatus(_ context.Context, key string, in ReportCIWatcherStatusInput) (Written[ProjectView], error) {
	next := CIWatcherView{State: in.State}
	switch in.State {
	case CIWatcherWatching:
		if !in.RateLimitedUntil.IsZero() {
			next.RateLimitedUntil = in.RateLimitedUntil.UTC()
		}
	case CIWatcherError:
		if !in.RateLimitedUntil.IsZero() {
			return Written[ProjectView]{}, invalid("rate_limited_until", "only a Watching CI Watcher is rate-limited; an error has its own fix")
		}
		if in.Problem != "" && !slices.Contains(ciProblems, in.Problem) {
			return Written[ProjectView]{}, invalid("problem", "%q isn't a CI problem", in.Problem)
		}
		next.Problem, next.Message, next.Fix = in.Problem, strings.TrimSpace(in.Message), strings.TrimSpace(in.Fix)
		if next.Message == "" {
			return Written[ProjectView]{}, invalid("message", "an error needs its message")
		}
	default:
		return Written[ProjectView]{}, invalid("state", "the CI Watcher is %q or %q, not %q", CIWatcherWatching, CIWatcherError, in.State)
	}
	var before, after ProjectView
	changed := false
	err := c.store.View(func(r store.Reader) error {
		if err := writable(r, key); err != nil {
			return err
		}
		p, _ := r.Project(key)
		if !p.CI.Tracking {
			return &Error{Code: ActionNotAllowed, Message: "CI Tracking is off on " + key + ", so it has no CI Watcher"}
		}
		// The store's read lock keeps set_ci_tracking from turning CI
		// Tracking off between the check and the change.
		before = c.projectView(p)
		changed = c.ciWatchers.report(key, next, c.now())
		after = c.projectView(p)
		return nil
	})
	if err != nil {
		return Written[ProjectView]{}, err
	}
	if changed {
		c.events.Publish(events.Event{Type: events.ProjectChanged, Project: key})
	}
	return acknowledgeProject(before, after, in.Full, c.now())
}

// CIWatcherAttention is one Project's CI Watcher as notify watches it.
type CIWatcherAttention struct {
	Project string
	// Errors counts the times it went into an error since the service
	// started, and LastError is the latest one's message.
	Errors    int
	LastError string
}

// CIWatcherAttention is every CI Watcher that has reported, by Project.
func (c *Core) CIWatcherAttention(context.Context) []CIWatcherAttention {
	return c.ciWatchers.attention()
}

func (w *ciWatchers) attention() []CIWatcherAttention {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := []CIWatcherAttention{}
	for key, s := range w.all {
		out = append(out, CIWatcherAttention{Project: key, Errors: s.errors, LastError: s.lastError})
	}
	slices.SortFunc(out, func(a, b CIWatcherAttention) int { return strings.Compare(a.Project, b.Project) })
	return out
}

// CIWatcherSteadyAfter is how long a restarted CI Watcher runs before
// its crash streak ends: its restart backoff starts again, and its own
// reports show again.
const CIWatcherSteadyAfter = 10 * time.Minute

// ciWatched is whether p should have a CI Watcher: CI Tracking on, and
// not archived.
func ciWatched(p store.Project) bool { return p.CI.Tracking && p.Archived.IsZero() }

// CIWatchedProjects is the key of every Project the service runs a CI
// Watcher for.
func (c *Core) CIWatchedProjects(context.Context) []string {
	out := []string{}
	c.store.View(func(r store.Reader) error {
		for _, p := range r.Projects() {
			if ciWatched(p) {
				out = append(out, p.Key)
			}
		}
		return nil
	})
	return out
}

// CIWatcherCrash is the service's report of a CI Watcher that crashed.
type CIWatcherCrash struct {
	// Restarts counts the crashes in this streak, each followed by a
	// restart.
	Restarts int
	// LastLine is the last line it wrote to stderr.
	LastLine string
	// RestartAt is when the service restarts it.
	RestartAt time.Time
}

// ReportCIWatcherCrash is the service's own report that key's CI Watcher
// crashed. It has no door: the CI Watcher can't report its own crash.
// The status stays crashed until the CI Watcher has run
// CIWatcherSteadyAfter from its restart, so like any error the streak
// sends one Notification however often it restarts. A Project no longer
// watched is left alone.
func (c *Core) ReportCIWatcherCrash(_ context.Context, key string, crash CIWatcherCrash) {
	times := "times"
	if crash.Restarts == 1 {
		times = "time"
	}
	msg := fmt.Sprintf("The CI Watcher crashed, restarted %d %s", crash.Restarts, times)
	if line := strings.TrimSpace(crash.LastLine); line != "" {
		msg += "; the last line it wrote: " + line
	}
	next := CIWatcherView{State: CIWatcherError, Problem: CICrashed, Message: msg,
		Fix: "Code Central restarts it by itself, waiting longer after each crash. Its output is in the service's log."}
	changed := false
	c.store.View(func(r store.Reader) error {
		if p, ok := r.Project(key); ok && ciWatched(p) {
			changed = c.ciWatchers.crashed(key, next, crash.RestartAt.Add(CIWatcherSteadyAfter), c.now())
		}
		return nil
	})
	if changed {
		c.events.Publish(events.Event{Type: events.ProjectChanged, Project: key})
	}
}
