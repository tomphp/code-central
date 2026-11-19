package core

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/store"
)

// Every kind of Watcher's status: the separate processes the service runs
// per Project, each watching one external system, and what they and the
// service report about them: the CI Watcher, the Issue Watcher, the Pull
// Request Watcher and the Work Item Watcher.
//
// The status lives in memory only, like pings: a Watcher is the service's
// child, so a restarted service has no status until its Watcher reports
// again, and never shows a stale Watching.

// The Watcher status contract lives in api; these aliases keep core's own
// code naming it unqualified.
type (
	WatcherKind        = api.WatcherKind
	WatcherProblem     = api.WatcherProblem
	WatcherCheckResult = api.WatcherCheckResult
	RepositoryProblem  = api.RepositoryProblem
	WatcherState       = api.WatcherState
	WatcherCrash       = api.WatcherCrash
)

const (
	WatcherCI           = api.WatcherCI
	WatcherIssues       = api.WatcherIssues
	WatcherPullRequests = api.WatcherPullRequests
	WatcherWorkItems    = api.WatcherWorkItems

	ProblemGHMissing            = api.ProblemGHMissing
	ProblemLoggedOut            = api.ProblemLoggedOut
	ProblemRepositoryUnreadable = api.ProblemRepositoryUnreadable
	ProblemIssuesDisabled       = api.ProblemIssuesDisabled
	ProblemNoTriage             = api.ProblemNoTriage
	ProblemNoWrite              = api.ProblemNoWrite
	ProblemPATMissing           = api.ProblemPATMissing
	ProblemPATRejected          = api.ProblemPATRejected
	ProblemProjectUnreadable    = api.ProblemProjectUnreadable
	ProblemPATScope             = api.ProblemPATScope
	ProblemQueryUnusable        = api.ProblemQueryUnusable
	ProblemTooManyWorkItems     = api.ProblemTooManyWorkItems
	ProblemCheckFailed          = api.ProblemCheckFailed
	ProblemCrashed              = api.ProblemCrashed

	WatcherNotReported = api.WatcherNotReported
	WatcherWatching    = api.WatcherWatching
	WatcherError       = api.WatcherError
	WatcherElsewhere   = api.WatcherElsewhere
	WatcherWaiting     = api.WatcherWaiting

	WatcherSteadyAfter = api.WatcherSteadyAfter
)

// WatcherProblems are the problems.
func WatcherProblems() []WatcherProblem { return api.WatcherProblems() }

// WatcherStates are the states a Watcher can be in.
func WatcherStates() []WatcherState { return api.WatcherStates() }

// watcherKind is how one kind of Watcher is configured; a new kind of
// Watcher is one more entry in watcherKinds.
type watcherKind struct {
	// on is whether the Project setting that turns it on is on; the
	// setting's name and the kind's own are in api.
	on func(domain.Project) bool
	// problems are the problems an error it reports may name; crashed is
	// the service's, never the Watcher's. A problem it may not name is
	// refused as not a problem of watches, such as CI.
	problems []WatcherProblem
	watches  string
	// watched are the names of what it watches, one by one, that a
	// per-Repository problem may name: the Project's Repository Names
	// unless set, as the Issue Watcher's repositories are GitHub's
	// owner/name, and the Work Item Watcher, which runs one query, has
	// none.
	watched func(domain.Project) []string
}

// watcherKinds is every kind of Watcher.
var watcherKinds = map[WatcherKind]watcherKind{
	WatcherCI: {on: func(p domain.Project) bool { return landingPolicies(p).RunsCIWatcher() },
		problems: []WatcherProblem{ProblemGHMissing, ProblemLoggedOut, ProblemRepositoryUnreadable, ProblemPATMissing, ProblemPATRejected,
			ProblemProjectUnreadable, ProblemPATScope, ProblemCheckFailed}, watches: "CI"},
	WatcherIssues: {on: func(p domain.Project) bool { return p.GitHubIssues.On },
		problems: []WatcherProblem{ProblemGHMissing, ProblemLoggedOut, ProblemRepositoryUnreadable, ProblemIssuesDisabled, ProblemNoTriage, ProblemCheckFailed},
		watches:  "GitHub Issues", watched: func(p domain.Project) []string { return p.GitHubIssues.Repositories }},
	WatcherPullRequests: {on: func(p domain.Project) bool { return landingPolicies(p).RunsPullRequestWatcher() },
		problems: []WatcherProblem{ProblemGHMissing, ProblemLoggedOut, ProblemRepositoryUnreadable, ProblemNoWrite, ProblemPATMissing, ProblemPATRejected,
			ProblemPATScope, ProblemCheckFailed}, watches: "pull requests"},
	WatcherWorkItems: {on: func(p domain.Project) bool { return p.AzureBoards.On },
		problems: []WatcherProblem{ProblemPATMissing, ProblemPATRejected, ProblemProjectUnreadable, ProblemPATScope, ProblemQueryUnusable,
			ProblemTooManyWorkItems, ProblemCheckFailed}, watches: "Azure Boards", watched: func(domain.Project) []string { return nil }},
}

// watcherViewsEqual reports whether v and w say the same, Since aside.
func watcherViewsEqual(v, w WatcherView) bool {
	return v.State == w.State && v.Problem == w.Problem && v.Message == w.Message && v.Fix == w.Fix &&
		v.RateLimitedUntil.Equal(w.RateLimitedUntil) && slices.Equal(v.Repositories, w.Repositories)
}

// watcherWatches reports whether the Watcher v watches the Repository
// name: it is Watching, or in error only about other Repositories. One
// another service runs is taken to watch it, as this service can't tell.
func watcherWatches(v WatcherView, name string) bool {
	if v.State == WatcherWatching || v.State == WatcherElsewhere {
		return true
	}
	return len(v.Repositories) > 0 && !slices.ContainsFunc(v.Repositories, func(p RepositoryProblem) bool { return p.Repository == name })
}

// watcherMessageFor is what the Watcher v says about the Repository name
// while it doesn't watch it: its own problem's message, or the Watcher's.
func watcherMessageFor(v WatcherView, name string) string {
	if i := slices.IndexFunc(v.Repositories, func(p RepositoryProblem) bool { return p.Repository == name }); i >= 0 {
		return v.Repositories[i].Message
	}
	return v.Message
}

// watcherKey names one Project's Watcher of one kind.
type watcherKey struct {
	kind    WatcherKind
	project string
}

// watchers is every Watcher's status, by kind and Project, and each
// Project's Watcher Lease as this service last held or saw it.
type watchers struct {
	mu  sync.Mutex
	all map[watcherKey]watcherStatus
	// self is this service's ID, which holds the Leases it holds, and
	// leases are the Leases by Project, once known.
	self   string
	leases map[string]store.Lease
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

// view is the status of k's Watcher at now: its own, while this service
// holds the Project's Watcher Lease or none is known yet; else where it
// runs, or that none does.
func (w *watchers) view(k watcherKey, now time.Time) WatcherView {
	w.mu.Lock()
	defer w.mu.Unlock()
	l, leased := w.leases[k.project]
	switch {
	case leased && l.Lapsed(now):
		return WatcherView{State: WatcherWaiting, Message: "Waiting for a service to take the Watchers."}
	case leased && l.Holder != w.self:
		return WatcherView{State: WatcherElsewhere, Host: l.Host}
	}
	v := WatcherView{State: WatcherNotReported}
	if s, ok := w.all[k]; ok {
		v = s.view
	}
	v.Host = l.Host
	return v
}

// hold records l as the Project's Watcher Lease and reports whether its
// view changed: it is held by another service, or on another host, or it
// had lapsed. One this service lost takes its Watchers' statuses with it,
// so it doesn't show them as they were should it hold the Lease again.
func (w *watchers) hold(l store.Lease, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	was, known := w.leases[l.Project]
	w.leases[l.Project] = l
	if known && was.Holder == w.self && l.Holder != w.self {
		for k, s := range w.all {
			if k.project == l.Project {
				s.view, s.crashingUntil = WatcherView{State: WatcherNotReported}, time.Time{}
				w.all[k] = s
			}
		}
	}
	return !known || was.Holder != l.Holder || was.Host != l.Host || was.Lapsed(now)
}

// lease is the Project key's Watcher Lease as last known.
func (w *watchers) lease(key string) (store.Lease, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	l, ok := w.leases[key]
	return l, ok
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
	if ok && watcherViewsEqual(s.view, next) {
		return false
	}
	was := s.view
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

// repositoryProblems is ps checked as a k Watcher's: each names a
// Repository once, with a message and one of k's problems.
func repositoryProblems(k watcherKind, ps []RepositoryProblem) ([]RepositoryProblem, error) {
	var out []RepositoryProblem
	for i, p := range ps {
		field := fmt.Sprintf("repositories[%d]", i)
		p = RepositoryProblem{Repository: strings.TrimSpace(p.Repository), Problem: p.Problem, Message: strings.TrimSpace(p.Message), Fix: strings.TrimSpace(p.Fix)}
		switch {
		case p.Repository == "":
			return nil, invalid(field+".repository", "a Repository's problem names the Repository")
		case slices.ContainsFunc(out, func(o RepositoryProblem) bool { return o.Repository == p.Repository }):
			return nil, invalid(field+".repository", "%s is named twice: report one problem per Repository", p.Repository)
		case p.Problem != "" && !slices.Contains(k.problems, p.Problem):
			return nil, invalid(field+".problem", "%q isn't a %s problem", p.Problem, k.watches)
		case p.Message == "":
			return nil, invalid(field+".message", "a Repository's problem needs its message")
		}
		out = append(out, p)
	}
	return out, nil
}

// reportWatcherStatus is key's Watcher of kind reporting its status, on
// every poll; a change publishes a notice. Going into an error is
// counted, whoever reports it, so notify sends one Notification; being
// rate-limited is Watching, so it sends none.
func (c *Core) reportWatcherStatus(kind WatcherKind, key string, in ReportWatcherStatusInput) (Written[ProjectView], error) {
	k := watcherKinds[kind]
	next := WatcherView{State: in.State}
	var err error
	switch in.State {
	case WatcherWatching:
		if !in.RateLimitedUntil.IsZero() {
			next.RateLimitedUntil = in.RateLimitedUntil.UTC()
		}
		if len(in.Repositories) > 0 {
			return Written[ProjectView]{}, invalid("repositories", "a Watching %s has no failing Repositories; report them as an error", kind.Name())
		}
	case WatcherError:
		if !in.RateLimitedUntil.IsZero() {
			return Written[ProjectView]{}, invalid("rate_limited_until", "only a Watching %s is rate-limited; an error has its own fix", kind.Name())
		}
		if in.Problem != "" && !slices.Contains(k.problems, in.Problem) {
			return Written[ProjectView]{}, invalid("problem", "%q isn't a %s problem", in.Problem, k.watches)
		}
		next.Problem, next.Message, next.Fix = in.Problem, strings.TrimSpace(in.Message), strings.TrimSpace(in.Fix)
		if next.Repositories, err = repositoryProblems(k, in.Repositories); err != nil {
			return Written[ProjectView]{}, err
		}
		if next.Message == "" && len(next.Repositories) > 0 {
			// An error in some Repositories reads as the first one's.
			first := next.Repositories[0]
			next.Problem, next.Message, next.Fix = first.Problem, first.Repository+": "+first.Message, first.Fix
		}
		if next.Message == "" {
			return Written[ProjectView]{}, invalid("message", "an error needs its message")
		}
	default:
		return Written[ProjectView]{}, invalid("state", "the %s is %q or %q, not %q", kind.Name(), WatcherWatching, WatcherError, in.State)
	}
	var before, after ProjectView
	changed := false
	err = c.store.View(func(r store.Reader) error {
		if err := writable(r, key); err != nil {
			return err
		}
		p, _ := r.Project(key)
		if !k.on(p) {
			return &Error{Code: ActionNotAllowed, Message: fmt.Sprintf("%s is off on %s, so it has no %s", kind.Setting(), key, kind.Name())}
		}
		names := repositoryNames(p.RepositoryList())
		if k.watched != nil {
			names = k.watched(p)
		}
		for i, rp := range next.Repositories {
			if !slices.Contains(names, rp.Repository) {
				field := fmt.Sprintf("repositories[%d].repository", i)
				switch {
				case k.watched == nil:
					return notARepository(p, field, rp.Repository)
				case len(names) == 0:
					return invalid(field, "the %s watches no repositories: report its problem without naming one", kind.Name())
				default:
					return invalid(field, "%q is not one of %s's %s repositories, which are %s", rp.Repository, key, kind.Setting(), strings.Join(names, ", "))
				}
			}
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

// watched is whether p should have a Watcher of kind: its setting on,
// and not archived.
func watched(kind WatcherKind, p domain.Project) bool {
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
