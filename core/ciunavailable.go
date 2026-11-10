package core

import (
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
	// CIWatcherNotWatching is a Project whose CI Watcher is in error or
	// hasn't reported.
	CIWatcherNotWatching CIUnavailableCause = "ci_watcher_not_watching"
	// CINoRuns is a pushed commit with no CI Runs reported, ignored ones
	// aside, while the CI Watcher is Watching.
	CINoRuns CIUnavailableCause = "no_ci_runs"
)

// CIUnavailableView is a CI unavailable row's detail: the cause, the
// pushed commit, and, while the CI Watcher is in error, its message.
type CIUnavailableView struct {
	Cause   CIUnavailableCause `json:"cause"`
	Commit  string             `json:"commit"`
	Message string             `json:"message,omitempty"`
}

// ciGated is t's latest pushed commit and the runs tracked against t,
// when the CI gate holds t: Accepted with a pushed commit in a Project
// under Merge + Push with CI Tracking on.
func ciGated(r store.Reader, p store.Project, t store.Ticket) (store.PushedCommit, []store.CIRun, bool) {
	if t.State != store.Accepted || p.Landing.Method != store.LandingMerge || !p.Landing.Push || !p.CI.Tracking {
		return store.PushedCommit{}, nil, false
	}
	runs, ok := ticketRuns(r, p.Key, t)
	if !ok {
		return store.PushedCommit{}, nil, false
	}
	return t.PushedCommits[len(t.PushedCommits)-1], runs, true
}

// ciWaitingOn reports whether the CI gate holds t on a CI result still
// running, which time passing doesn't make Accepted but not landed. With
// unavailable set, it is also CI unavailable: its detail, and its push as
// since when.
func (c *Core) ciWaitingOn(r store.Reader, p store.Project, t store.Ticket) (waiting bool, unavailable *CIUnavailableView, since time.Time) {
	pushed, runs, ok := ciGated(r, p, t)
	if !ok || ciResultOf(pushed.Commit, runs) != CIRunning {
		return false, nil, time.Time{}
	}
	if c.now().Sub(pushed.Pushed) < CIUnavailableAfter {
		return true, nil, time.Time{}
	}
	if w := c.ciWatchers.view(p.Key); w.State != CIWatcherWatching {
		return true, &CIUnavailableView{Cause: CIWatcherNotWatching, Commit: pushed.Commit, Message: w.Message}, pushed.Pushed
	}
	for _, run := range runs {
		if run.Commit == pushed.Commit && !run.Ignored {
			return true, nil, time.Time{}
		}
	}
	return true, &CIUnavailableView{Cause: CINoRuns, Commit: pushed.Commit}, pushed.Pushed
}
