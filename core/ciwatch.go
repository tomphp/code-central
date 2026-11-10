package core

import (
	"context"
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
		w.all[key] = s
	}
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
	Rules            []CIRuleView     `json:"rules"`
	GHPath           string           `json:"gh_path"`
}

// GetCIWatch is the CI Watcher's get_ci_watch.
func (c *Core) GetCIWatch(_ context.Context, key string) (CIWatch, error) {
	var out CIWatch
	err := c.store.View(func(r store.Reader) error {
		p, ok := r.Project(key)
		if !ok {
			return unknownProject(key)
		}
		v := ciView(key, p.CI, CIWatcherView{})
		out = CIWatch{Project: key, Tracking: v.Tracking, Provider: v.Provider, Repository: v.Repository,
			IgnoredWorkflows: v.IgnoredWorkflows, Rules: v.Rules, GHPath: r.Options().GHPath}
		return nil
	})
	return out, err
}

// ReportCIWatcherStatusInput is the CI Watcher's
// report_ci_watcher_status(state, problem?, message?, fix?, full?).
type ReportCIWatcherStatusInput struct {
	// State is watching or error.
	State   CIWatcherState `json:"state"`
	Problem CIProblem      `json:"problem,omitempty"`
	// Message and Fix are an error's; watching drops them.
	Message string `json:"message,omitempty"`
	Fix     string `json:"fix,omitempty"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// ReportCIWatcherStatus is report_ci_watcher_status: the CI Watcher
// reports on every poll, and a change publishes a notice. Going into an
// error is counted, whoever reports it, so notify sends one Notification.
func (c *Core) ReportCIWatcherStatus(_ context.Context, key string, in ReportCIWatcherStatusInput) (Written[ProjectView], error) {
	next := CIWatcherView{State: in.State}
	switch in.State {
	case CIWatcherWatching:
	case CIWatcherError:
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
		changed = c.ciWatchers.set(key, next, c.now())
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
