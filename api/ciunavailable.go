package api

import (
	"time"
)

// CIUnavailableCause is why a Ticket is CI unavailable.
type CIUnavailableCause string

// CIUnavailableCauses are the causes.
func CIUnavailableCauses() []CIUnavailableCause {
	return []CIUnavailableCause{CIWatcherNotWatching, CINoRuns}
}

// CIUnavailableView is a CI unavailable row's detail: the cause, the
// pushed commit and when it was pushed, and, while the CI Watcher is in
// error, its message.
type CIUnavailableView struct {
	Cause   CIUnavailableCause `json:"cause"`
	Commit  string             `json:"commit"`
	Pushed  time.Time          `json:"pushed"`
	Message string             `json:"message,omitempty"`
}

const (
	// CIWatcherNotWatching is the cause while the Project's CI Watcher is
	// in error or hasn't reported.
	CIWatcherNotWatching CIUnavailableCause = "ci_watcher_not_watching"
	// CINoRuns is the cause while the CI Watcher is Watching but no CI
	// Runs, ignored ones aside, are reported for the pushed commit.
	CINoRuns CIUnavailableCause = "no_ci_runs"
)
