package core

import (
	"slices"
	"time"

	"github.com/tomoram/code-central/store"
)

// CI unavailable (code-central-262): a Ticket the CI gate holds shows in
// Needs Me once it has waited CIUnavailableAfter since its push while
// nothing is telling Code Central how its CI went: the CI Watcher isn't
// Watching, or no CI Runs have been reported for its pushed commit.
// Worked out on read, never stored.

// CIUnavailableAfter is how long after its push a Ticket the CI gate
// holds may go without word of its CI before it is CI unavailable.
const CIUnavailableAfter = 10 * time.Minute

// CIUnavailableCause is why a Ticket is CI unavailable.
type CIUnavailableCause string

const (
	// CIWatcherNotWatching is the cause while the Project's CI Watcher is
	// in error or hasn't reported.
	CIWatcherNotWatching CIUnavailableCause = "ci_watcher_not_watching"
	// CINoRuns is the cause while the CI Watcher is Watching but no CI
	// Runs, ignored ones aside, are reported for the pushed commit.
	CINoRuns CIUnavailableCause = "no_ci_runs"
)

// CIUnavailableView is a CI unavailable row's detail: the cause, the
// pushed commit and when it was pushed, and, while the CI Watcher is in
// error, its message.
type CIUnavailableView struct {
	Cause   CIUnavailableCause `json:"cause"`
	Commit  string             `json:"commit"`
	Pushed  time.Time          `json:"pushed"`
	Message string             `json:"message,omitempty"`
}

// ciGated is t's latest pushed commit and the runs tracked against t and
// kept at now,
// when the CI gate holds t: Accepted with a pushed commit in a Project
// under Merge + Push with CI Tracking on.
func ciGated(r store.Reader, p store.Project, t store.Ticket, now time.Time) (store.PushedCommit, []store.CIRun, bool) {
	if t.State != store.Accepted || !p.LandingPolicy().CIGateHolds() {
		return store.PushedCommit{}, nil, false
	}
	runs, ok := ticketRuns(r, p.Key, t, now)
	if !ok {
		return store.PushedCommit{}, nil, false
	}
	return t.PushedCommits[len(t.PushedCommits)-1], runs, true
}

// ciWaitingOn reports whether the CI gate holds t on a CI result still
// running, which time passing doesn't make Accepted but not landed, and,
// when it has also gone CIUnavailableAfter since its push with no word
// of its CI, why it is CI unavailable.
func (c *Core) ciWaitingOn(r store.Reader, p store.Project, t store.Ticket) (waiting bool, unavailable *CIUnavailableView) {
	pushed, runs, ok := ciGated(r, p, t, c.now())
	if !ok || latestCIResult(pushed, runs, c.now()) != CIRunning {
		return false, nil
	}
	if c.now().Sub(pushed.Pushed) < CIUnavailableAfter {
		return true, nil
	}
	if !isCILead(r, p.Key, t) {
		return true, nil // the lead of its Spec's group carries the reason
	}
	view := &CIUnavailableView{Commit: pushed.Commit, Pushed: pushed.Pushed}
	if w := c.watchers.view(watcherKey{WatcherCI, p.Key}); w.State != WatcherWatching {
		view.Cause, view.Message = CIWatcherNotWatching, w.Message
		return true, view
	}
	if slices.ContainsFunc(runs, func(run store.CIRun) bool { return countsFor(run, pushed.Commit) }) {
		return true, nil
	}
	view.Cause = CINoRuns
	return true, view
}
