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

// watcherKind is how one kind of Watcher is configured; a new kind of
// Watcher is one more entry in watcherKinds.
type watcherKind struct {
	// on is whether the Project setting that turns it on is on; the
	// setting's name and the kind's own are in api.
	on func(domain.Project) bool
	// problems are the problems an error it reports may name; crashed is
	// the service's, never the Watcher's. A problem it may not name is
	// refused as not a problem of watches, such as CI.
	problems []api.WatcherProblem
	watches  string
	// watched are the names of what it watches, one by one, that a
	// per-Repository problem may name: the Project's Repository Names
	// unless set, as the Issue Watcher's repositories are GitHub's
	// owner/name, and the Work Item Watcher, which runs one query, has
	// none.
	watched func(domain.Project) []string
}

// watcherKinds is every kind of Watcher.
var watcherKinds = map[api.WatcherKind]watcherKind{
	api.WatcherCI: {on: func(p domain.Project) bool { return landingPolicies(p).RunsCIWatcher() },
		problems: []api.WatcherProblem{api.ProblemGHMissing, api.ProblemLoggedOut, api.ProblemRepositoryUnreadable, api.ProblemPATMissing, api.ProblemPATRejected,
			api.ProblemProjectUnreadable, api.ProblemPATScope, api.ProblemCheckFailed}, watches: "CI"},
	api.WatcherIssues: {on: func(p domain.Project) bool { return p.GitHubIssues.On },
		problems: []api.WatcherProblem{api.ProblemGHMissing, api.ProblemLoggedOut, api.ProblemRepositoryUnreadable, api.ProblemIssuesDisabled, api.ProblemNoTriage, api.ProblemCheckFailed},
		watches:  "GitHub Issues", watched: func(p domain.Project) []string { return p.GitHubIssues.Repositories }},
	api.WatcherPullRequests: {on: func(p domain.Project) bool { return landingPolicies(p).RunsPullRequestWatcher() },
		problems: []api.WatcherProblem{api.ProblemGHMissing, api.ProblemLoggedOut, api.ProblemRepositoryUnreadable, api.ProblemNoWrite, api.ProblemPATMissing, api.ProblemPATRejected,
			api.ProblemPATScope, api.ProblemCheckFailed}, watches: "pull requests"},
	api.WatcherWorkItems: {on: func(p domain.Project) bool { return p.AzureBoards.On },
		problems: []api.WatcherProblem{api.ProblemPATMissing, api.ProblemPATRejected, api.ProblemProjectUnreadable, api.ProblemPATScope, api.ProblemQueryUnusable,
			api.ProblemTooManyWorkItems, api.ProblemCheckFailed}, watches: "Azure Boards", watched: func(domain.Project) []string { return nil }},
}

// watcherViewsEqual reports whether v and w say the same, Since aside.
func watcherViewsEqual(v, w api.WatcherView) bool {
	return v.State == w.State && v.Problem == w.Problem && v.Message == w.Message && v.Fix == w.Fix &&
		v.RateLimitedUntil.Equal(w.RateLimitedUntil) && slices.Equal(v.Repositories, w.Repositories)
}

// watcherWatches reports whether the Watcher v watches the Repository
// name: it is Watching, or in error only about other Repositories. One
// another service runs is taken to watch it, as this service can't tell.
func watcherWatches(v api.WatcherView, name string) bool {
	if v.State == api.WatcherWatching || v.State == api.WatcherElsewhere {
		return true
	}
	return len(v.Repositories) > 0 && !slices.ContainsFunc(v.Repositories, func(p api.RepositoryProblem) bool { return p.Repository == name })
}

// watcherMessageFor is what the Watcher v says about the Repository name
// while it doesn't watch it: its own problem's message, or the Watcher's.
func watcherMessageFor(v api.WatcherView, name string) string {
	if i := slices.IndexFunc(v.Repositories, func(p api.RepositoryProblem) bool { return p.Repository == name }); i >= 0 {
		return v.Repositories[i].Message
	}
	return v.Message
}

// watcherKey names one Project's Watcher of one kind.
type watcherKey struct {
	kind    api.WatcherKind
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
	view api.WatcherView
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
func (w *watchers) view(k watcherKey, now time.Time) api.WatcherView {
	w.mu.Lock()
	defer w.mu.Unlock()
	l, leased := w.leases[k.project]
	switch {
	case leased && l.Lapsed(now):
		return api.WatcherView{State: api.WatcherWaiting, Message: "Waiting for a service to take the Watchers."}
	case leased && l.Holder != w.self:
		return api.WatcherView{State: api.WatcherElsewhere, Host: l.Host}
	}
	v := api.WatcherView{State: api.WatcherNotReported}
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
				s.view, s.crashingUntil = api.WatcherView{State: api.WatcherNotReported}, time.Time{}
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
		s.view = api.WatcherView{State: api.WatcherNotReported}
		s.crashingUntil = time.Time{}
		w.all[k] = s
	}
}

// report makes next, as the Watcher reported it, k's status, unless a
// crash streak holds it back, and reports whether it changed.
func (w *watchers) report(k watcherKey, next api.WatcherView, now time.Time) bool {
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
func (w *watchers) crashed(k watcherKey, next api.WatcherView, until, now time.Time) bool {
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
func (w *watchers) set(k watcherKey, next api.WatcherView, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	s, ok := w.all[k]
	if ok && watcherViewsEqual(s.view, next) {
		return false
	}
	was := s.view
	if next.State == api.WatcherError && was.State != api.WatcherError {
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
func repositoryProblems(k watcherKind, ps []api.RepositoryProblem) ([]api.RepositoryProblem, error) {
	var out []api.RepositoryProblem
	for i, p := range ps {
		field := fmt.Sprintf("repositories[%d]", i)
		p = api.RepositoryProblem{Repository: strings.TrimSpace(p.Repository), Problem: p.Problem, Message: strings.TrimSpace(p.Message), Fix: strings.TrimSpace(p.Fix)}
		switch {
		case p.Repository == "":
			return nil, invalid(field+".repository", "a Repository's problem names the Repository")
		case slices.ContainsFunc(out, func(o api.RepositoryProblem) bool { return o.Repository == p.Repository }):
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
func (c *Core) reportWatcherStatus(kind api.WatcherKind, key string, in api.ReportWatcherStatusInput) (api.Written[api.ProjectView], error) {
	k := watcherKinds[kind]
	next := api.WatcherView{State: in.State}
	var err error
	switch in.State {
	case api.WatcherWatching:
		if !in.RateLimitedUntil.IsZero() {
			next.RateLimitedUntil = in.RateLimitedUntil.UTC()
		}
		if len(in.Repositories) > 0 {
			return api.Written[api.ProjectView]{}, invalid("repositories", "a Watching %s has no failing Repositories; report them as an error", kind.Name())
		}
	case api.WatcherError:
		if !in.RateLimitedUntil.IsZero() {
			return api.Written[api.ProjectView]{}, invalid("rate_limited_until", "only a Watching %s is rate-limited; an error has its own fix", kind.Name())
		}
		if in.Problem != "" && !slices.Contains(k.problems, in.Problem) {
			return api.Written[api.ProjectView]{}, invalid("problem", "%q isn't a %s problem", in.Problem, k.watches)
		}
		next.Problem, next.Message, next.Fix = in.Problem, strings.TrimSpace(in.Message), strings.TrimSpace(in.Fix)
		if next.Repositories, err = repositoryProblems(k, in.Repositories); err != nil {
			return api.Written[api.ProjectView]{}, err
		}
		if next.Message == "" && len(next.Repositories) > 0 {
			// An error in some Repositories reads as the first one's.
			first := next.Repositories[0]
			next.Problem, next.Message, next.Fix = first.Problem, first.Repository+": "+first.Message, first.Fix
		}
		if next.Message == "" {
			return api.Written[api.ProjectView]{}, invalid("message", "an error needs its message")
		}
	default:
		return api.Written[api.ProjectView]{}, invalid("state", "the %s is %q or %q, not %q", kind.Name(), api.WatcherWatching, api.WatcherError, in.State)
	}
	var before, after api.ProjectView
	changed := false
	err = c.store.View(func(r store.Reader) error {
		if err := writable(r, key); err != nil {
			return err
		}
		p, _ := r.Project(key)
		if !k.on(p) {
			return &api.Error{Code: api.ActionNotAllowed, Message: fmt.Sprintf("%s is off on %s, so it has no %s", kind.Setting(), key, kind.Name())}
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
		return api.Written[api.ProjectView]{}, err
	}
	if changed {
		c.events.Publish(events.Event{Type: events.ProjectChanged, Project: key})
	}
	return acknowledgeProject(before, after, in.Full, c.now())
}

// WatcherAttention is every Watcher that has reported, by Project, then
// kind.
func (c *Core) WatcherAttention(context.Context) []api.WatcherAttention {
	return c.watchers.attention()
}

func (w *watchers) attention() []api.WatcherAttention {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := []api.WatcherAttention{}
	for k, s := range w.all {
		out = append(out, api.WatcherAttention{Kind: k.kind, Project: k.project, Errors: s.errors, LastError: s.lastError})
	}
	slices.SortFunc(out, func(a, b api.WatcherAttention) int {
		return cmp.Or(strings.Compare(a.Project, b.Project), strings.Compare(string(a.Kind), string(b.Kind)))
	})
	return out
}

// watched is whether p should have a Watcher of kind: its setting on,
// and not archived.
func watched(kind api.WatcherKind, p domain.Project) bool {
	return watcherKinds[kind].on(p) && p.Archived.IsZero()
}

// WatchedProjects is the key of every Project the service runs a Watcher
// of kind for.
func (c *Core) WatchedProjects(_ context.Context, kind api.WatcherKind) []string {
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
func (c *Core) ReportWatcherCrash(_ context.Context, kind api.WatcherKind, key string, crash api.WatcherCrash) {
	times := "times"
	if crash.Restarts == 1 {
		times = "time"
	}
	msg := fmt.Sprintf("The %s crashed, restarted %d %s", kind.Name(), crash.Restarts, times)
	if line := strings.TrimSpace(crash.LastLine); line != "" {
		msg += "; the last line it wrote: " + line
	}
	next := api.WatcherView{State: api.WatcherError, Problem: api.ProblemCrashed, Message: msg,
		Fix: "Code Central restarts it by itself, waiting longer after each crash. Its output is in the service's log."}
	changed := false
	c.store.View(func(r store.Reader) error {
		if p, ok := r.Project(key); ok && watched(kind, p) {
			changed = c.watchers.crashed(watcherKey{kind, key}, next, crash.RestartAt.Add(api.WatcherSteadyAfter), c.now())
		}
		return nil
	})
	if changed {
		c.events.Publish(events.Event{Type: events.ProjectChanged, Project: key})
	}
}
