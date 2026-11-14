package core

import "time"

// CI unavailable (code-central-262): a Ticket the CI gate holds shows in
// Needs Me once it has waited CIUnavailableAfter since its push while
// nothing is telling Code Central how its CI went: the CI Watcher isn't
// Watching, or no CI Runs have been reported for its pushed commit.
// Worked out on read by the Project's CI state, never stored.

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
