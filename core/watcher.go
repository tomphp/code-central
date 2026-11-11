package core

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/store"
)

// Every kind of Watcher's status: the separate processes the service runs
// per Project, each watching one external system, and what they and the
// service report about them: the CI Watcher, and the Issue Watcher.
//
// The status lives in memory only, like pings: a Watcher is the service's
// child, so a restarted service has no status until its Watcher reports
// again, and never shows a stale Watching.

// WatcherKind names a kind of Watcher.
type WatcherKind string

// The kinds of Watcher.
const (
	// WatcherCI is the CI Watcher, run while CI Tracking is on.
	WatcherCI WatcherKind = "ci"
	// WatcherIssues is the Issue Watcher, run while GitHub Issues is on.
	WatcherIssues WatcherKind = "issues"
)

// watcherKind is how one kind of Watcher is configured; a new kind of
// Watcher is one more entry in watcherKinds.
type watcherKind struct {
	// name is the kind's glossary name, such as CI Watcher.
	name string
	// setting is the Project setting that turns it on, such as CI
	// Tracking, and on whether it is on.
	setting string
	on      func(store.Project) bool
	// problems are the problems an error it reports may name; crashed is
	// the service's, never the Watcher's. A problem it may not name is
	// refused as not a problem of watches, such as CI.
	problems []WatcherProblem
	watches  string
}

// watcherKinds is every kind of Watcher.
var watcherKinds = map[WatcherKind]watcherKind{
	WatcherCI: {name: "CI Watcher", setting: "CI Tracking", on: func(p store.Project) bool { return p.CI.Tracking },
		problems: []WatcherProblem{ProblemGHMissing, ProblemLoggedOut, ProblemRepositoryUnreadable, ProblemPATMissing, ProblemPATRejected,
			ProblemProjectUnreadable, ProblemPATScope, ProblemCheckFailed}, watches: "CI"},
	WatcherIssues: {name: "Issue Watcher", setting: "GitHub Issues", on: func(p store.Project) bool { return p.GitHubIssues.On },
		problems: []WatcherProblem{ProblemGHMissing, ProblemLoggedOut, ProblemRepositoryUnreadable, ProblemIssuesDisabled, ProblemNoTriage, ProblemCheckFailed},
		watches:  "GitHub Issues"},
}

// Name is the kind's glossary name, such as CI Watcher.
func (k WatcherKind) Name() string { return watcherKinds[k].name }

// Setting is the name of the Project setting that turns the kind on, such
// as CI Tracking.
func (k WatcherKind) Setting() string { return watcherKinds[k].setting }

// WatcherProblem names why a Watcher can't watch what it watches.
type WatcherProblem string

// The problems, each with its own fix.
const (
	// ProblemGHMissing is gh, the GitHub CLI, found nowhere Code Central looks.
	ProblemGHMissing WatcherProblem = "gh_missing"
	// ProblemLoggedOut is gh with no login: gh auth status fails.
	ProblemLoggedOut WatcherProblem = "logged_out"
	// ProblemRepositoryUnreadable is a repository gh's login can't read.
	ProblemRepositoryUnreadable WatcherProblem = "repository_unreadable"
	// ProblemIssuesDisabled is a repository with GitHub's issues turned
	// off.
	ProblemIssuesDisabled WatcherProblem = "issues_disabled"
	// ProblemNoTriage is a repository on which gh's login has less than
	// triage permission, which labelling and closing issues need.
	ProblemNoTriage WatcherProblem = "no_triage"
	// ProblemPATMissing is the PAT environment variable unset or empty in
	// the service's environment.
	ProblemPATMissing WatcherProblem = "pat_missing"
	// ProblemPATRejected is an Azure DevOps PAT that is expired, revoked or
	// wrong.
	ProblemPATRejected WatcherProblem = "pat_rejected"
	// ProblemProjectUnreadable is an Azure DevOps project the PAT can't
	// read, or that wasn't found.
	ProblemProjectUnreadable WatcherProblem = "project_unreadable"
	// ProblemPATScope is an Azure DevOps PAT without the Build scope.
	ProblemPATScope WatcherProblem = "pat_scope"
	// ProblemCheckFailed is a check that didn't run to an answer.
	ProblemCheckFailed WatcherProblem = "check_failed"
	// ProblemCrashed is a Watcher that exited without being stopped. The
	// service reports it, never the Watcher.
	ProblemCrashed WatcherProblem = "crashed"
)

// WatcherCheckResult is a Watcher's check mode's answer: no Problem when
// it can watch, else what is wrong and its fix.
type WatcherCheckResult struct {
	Problem WatcherProblem `json:"problem,omitempty"`
	Reason  string         `json:"reason,omitempty"`
	Fix     string         `json:"fix,omitempty"`
}

// WatcherState is how a Project's Watcher last said it was.
type WatcherState string

const (
	// WatcherNotReported is a Watcher that hasn't reported since the
	// service started, or since its setting was turned on.
	WatcherNotReported WatcherState = "not_reported"
	// WatcherWatching is a Watcher that can reach what it watches.
	WatcherWatching WatcherState = "watching"
	// WatcherError is a Watcher that can't, with the message and fix.
	WatcherError WatcherState = "error"
)

// WatcherView is a Project's Watcher's status, in its kind's settings.
type WatcherView struct {
	State WatcherState `json:"state"`
	// Problem, Message and Fix say what is wrong, while in error.
	Problem WatcherProblem `json:"problem,omitempty"`
	Message string         `json:"message,omitempty"`
	Fix     string         `json:"fix,omitempty"`
	// RateLimitedUntil is when GitHub lets a rate-limited Watcher call it
	// again; until then it is still Watching, not in error.
	RateLimitedUntil time.Time `json:"rate_limited_until,omitzero"`
	// Since is when the Watcher went into this state; nil until it
	// reports.
	Since *time.Time `json:"since,omitempty"`
}

// watcherKey names one Project's Watcher of one kind.
type watcherKey struct {
	kind    WatcherKind
	project string
}

// watchers is every Watcher's status, by kind and Project.
type watchers struct {
	mu  sync.Mutex
	all map[watcherKey]watcherStatus
}

// watcherStatus is one Watcher's status, with the errors notify counts.
type watcherStatus struct {
	view WatcherView
	// errors counts the times it went into an error, and lastError is the
	// latest one's message: notify sends one Notification for each, even
	// one that has cleared by the time it looks.
	errors    int
	lastError string
	// crashingUntil is when a crash streak ends: until then the status
	// stays crashed, as the service reported it, and the Watcher's own
	// reports are held back, so a Watcher that reports Watching between
	// crashes doesn't hide them or notify for each one.
	crashingUntil time.Time
}

// view is the status of k's Watcher.
func (w *watchers) view(k watcherKey) WatcherView {
	w.mu.Lock()
	defer w.mu.Unlock()
	if s, ok := w.all[k]; ok {
		return s.view
	}
	return WatcherView{State: WatcherNotReported}
}

// reset makes k's status not reported, as when its setting is turned off
// or watches another repository, keeping its count of errors so the next
// one still notifies.
func (w *watchers) reset(k watcherKey) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if s, ok := w.all[k]; ok {
		s.view = WatcherView{State: WatcherNotReported}
		s.crashingUntil = time.Time{}
		w.all[k] = s
	}
}

// report makes next, as the Watcher reported it, k's status, unless a
// crash streak holds it back, and reports whether it changed.
func (w *watchers) report(k watcherKey, next WatcherView, now time.Time) bool {
	w.mu.Lock()
	crashing := now.Before(w.all[k].crashingUntil)
	w.mu.Unlock()
	if crashing {
		return false
	}
	return w.set(k, next, now)
}

// crashed makes next, the service's crash report, k's status, holding it
// until until.
func (w *watchers) crashed(k watcherKey, next WatcherView, until, now time.Time) bool {
	changed := w.set(k, next, now)
	w.mu.Lock()
	defer w.mu.Unlock()
	s := w.all[k]
	s.crashingUntil = until
	w.all[k] = s
	return changed
}

// set makes next k's status, keeping since while the status is the same,
// and reports whether it changed.
func (w *watchers) set(k watcherKey, next WatcherView, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	s, ok := w.all[k]
	was := s.view
	was.Since = nil
	if ok && was == next {
		return false
	}
	if next.State == WatcherError && was.State != WatcherError {
		s.errors++
		s.lastError = next.Message
	}
	if !ok || was.State != next.State {
		next.Since = &now
	} else {
		next.Since = s.view.Since
	}
	s.view = next
	w.all[k] = s
	return true
}

// ReportWatcherStatusInput is a Watcher's status report: for the CI
// Watcher, report_ci_watcher_status(state, problem?, message?, fix?,
// rate_limited_until?, full?).
type ReportWatcherStatusInput struct {
	// State is watching or error.
	State   WatcherState   `json:"state"`
	Problem WatcherProblem `json:"problem,omitempty"`
	// Message and Fix are an error's; watching drops them.
	Message string `json:"message,omitempty"`
	Fix     string `json:"fix,omitempty"`
	// RateLimitedUntil is when GitHub lets a Watching Watcher call it
	// again, while it is rate-limited.
	RateLimitedUntil time.Time `json:"rate_limited_until,omitzero"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// reportWatcherStatus is key's Watcher of kind reporting its status, on
// every poll; a change publishes a notice. Going into an error is
// counted, whoever reports it, so notify sends one Notification; being
// rate-limited is Watching, so it sends none.
func (c *Core) reportWatcherStatus(kind WatcherKind, key string, in ReportWatcherStatusInput) (Written[ProjectView], error) {
	k := watcherKinds[kind]
	next := WatcherView{State: in.State}
	switch in.State {
	case WatcherWatching:
		if !in.RateLimitedUntil.IsZero() {
			next.RateLimitedUntil = in.RateLimitedUntil.UTC()
		}
	case WatcherError:
		if !in.RateLimitedUntil.IsZero() {
			return Written[ProjectView]{}, invalid("rate_limited_until", "only a Watching %s is rate-limited; an error has its own fix", k.name)
		}
		if in.Problem != "" && !slices.Contains(k.problems, in.Problem) {
			return Written[ProjectView]{}, invalid("problem", "%q isn't a %s problem", in.Problem, k.watches)
		}
		next.Problem, next.Message, next.Fix = in.Problem, strings.TrimSpace(in.Message), strings.TrimSpace(in.Fix)
		if next.Message == "" {
			return Written[ProjectView]{}, invalid("message", "an error needs its message")
		}
	default:
		return Written[ProjectView]{}, invalid("state", "the %s is %q or %q, not %q", k.name, WatcherWatching, WatcherError, in.State)
	}
	var before, after ProjectView
	changed := false
	err := c.store.View(func(r store.Reader) error {
		if err := writable(r, key); err != nil {
			return err
		}
		p, _ := r.Project(key)
		if !k.on(p) {
			return &Error{Code: ActionNotAllowed, Message: fmt.Sprintf("%s is off on %s, so it has no %s", k.setting, key, k.name)}
		}
		// The store's read lock keeps the setting from being turned off
		// between the check and the change.
		before = c.projectView(p)
		changed = c.watchers.report(watcherKey{kind, key}, next, c.now())
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

// WatcherAttention is one Project's Watcher of one kind as notify watches
// it.
type WatcherAttention struct {
	Kind    WatcherKind
	Project string
	// Errors counts the times it went into an error since the service
	// started, and LastError is the latest one's message.
	Errors    int
	LastError string
}

// WatcherAttention is every Watcher that has reported, by Project, then
// kind.
func (c *Core) WatcherAttention(context.Context) []WatcherAttention {
	return c.watchers.attention()
}

func (w *watchers) attention() []WatcherAttention {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := []WatcherAttention{}
	for k, s := range w.all {
		out = append(out, WatcherAttention{Kind: k.kind, Project: k.project, Errors: s.errors, LastError: s.lastError})
	}
	slices.SortFunc(out, func(a, b WatcherAttention) int {
		return cmp.Or(strings.Compare(a.Project, b.Project), strings.Compare(string(a.Kind), string(b.Kind)))
	})
	return out
}

// WatcherSteadyAfter is how long a restarted Watcher runs before its
// crash streak ends: its restart backoff starts again, and its own
// reports show again.
const WatcherSteadyAfter = 10 * time.Minute

// watched is whether p should have a Watcher of kind: its setting on,
// and not archived.
func watched(kind WatcherKind, p store.Project) bool {
	return watcherKinds[kind].on(p) && p.Archived.IsZero()
}

// WatchedProjects is the key of every Project the service runs a Watcher
// of kind for.
func (c *Core) WatchedProjects(_ context.Context, kind WatcherKind) []string {
	out := []string{}
	c.store.View(func(r store.Reader) error {
		for _, p := range r.Projects() {
			if watched(kind, p) {
				out = append(out, p.Key)
			}
		}
		return nil
	})
	return out
}

// WatcherCrash is the service's report of a Watcher that crashed.
type WatcherCrash struct {
	// Restarts counts the crashes in this streak, each followed by a
	// restart.
	Restarts int
	// LastLine is the last line it wrote to stderr.
	LastLine string
	// RestartAt is when the service restarts it.
	RestartAt time.Time
}

// ReportWatcherCrash is the service's own report that key's Watcher of
// kind crashed. It has no door: a Watcher can't report its own crash. The
// status stays crashed until the Watcher has run WatcherSteadyAfter from
// its restart, so like any error the streak sends one Notification
// however often it restarts. A Project no longer watched is left alone.
func (c *Core) ReportWatcherCrash(_ context.Context, kind WatcherKind, key string, crash WatcherCrash) {
	times := "times"
	if crash.Restarts == 1 {
		times = "time"
	}
	msg := fmt.Sprintf("The %s crashed, restarted %d %s", kind.Name(), crash.Restarts, times)
	if line := strings.TrimSpace(crash.LastLine); line != "" {
		msg += "; the last line it wrote: " + line
	}
	next := WatcherView{State: WatcherError, Problem: ProblemCrashed, Message: msg,
		Fix: "Code Central restarts it by itself, waiting longer after each crash. Its output is in the service's log."}
	changed := false
	c.store.View(func(r store.Reader) error {
		if p, ok := r.Project(key); ok && watched(kind, p) {
			changed = c.watchers.crashed(watcherKey{kind, key}, next, crash.RestartAt.Add(WatcherSteadyAfter), c.now())
		}
		return nil
	})
	if changed {
		c.events.Publish(events.Event{Type: events.ProjectChanged, Project: key})
	}
}
