// Package api is the wire contract: what crosses the HTTP boundary between
// the service and its clients. It imports only domain and internal/fieldwalk,
// so a client that imports it doesn't link the Core or the Store (ADR 0072).
package api

import "time"

// The Watcher status contract: what the Watchers and the service say to each
// other about a Watcher's kind, check result, problem, state and crash.

// WatcherKind names a kind of Watcher.
type WatcherKind string

// The kinds of Watcher.
const (
	// WatcherCI is the CI Watcher, run while CI Tracking is on.
	WatcherCI WatcherKind = "ci"
	// WatcherIssues is the Issue Watcher, run while GitHub Issues is on.
	WatcherIssues WatcherKind = "issues"
	// WatcherPullRequests is the Pull Request Watcher, run while the
	// Landing Method is Pull Request.
	WatcherPullRequests WatcherKind = "pull_requests"
	// WatcherWorkItems is the Work Item Watcher, run while Azure Boards is
	// on.
	WatcherWorkItems WatcherKind = "work_items"
)

// kindNames are each kind's glossary name and the Project setting that
// turns it on.
var kindNames = map[WatcherKind]struct{ name, setting string }{
	WatcherCI:           {"CI Watcher", "CI Tracking"},
	WatcherIssues:       {"Issue Watcher", "GitHub Issues"},
	WatcherPullRequests: {"Pull Request Watcher", "Pull Request"},
	WatcherWorkItems:    {"Work Item Watcher", "Azure Boards"},
}

// Name is the kind's glossary name, such as CI Watcher.
func (k WatcherKind) Name() string { return kindNames[k].name }

// Setting is the name of the Project setting that turns the kind on, such
// as CI Tracking.
func (k WatcherKind) Setting() string { return kindNames[k].setting }

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
	// ProblemNoWrite is a repository on which gh's login can't write, as
	// opening a pull request needs.
	ProblemNoWrite WatcherProblem = "no_write"
	// ProblemPATMissing is the PAT environment variable unset or empty in
	// the service's environment.
	ProblemPATMissing WatcherProblem = "pat_missing"
	// ProblemPATRejected is an Azure DevOps PAT that is expired, revoked or
	// wrong.
	ProblemPATRejected WatcherProblem = "pat_rejected"
	// ProblemProjectUnreadable is an Azure DevOps project the PAT can't
	// read, or that wasn't found.
	ProblemProjectUnreadable WatcherProblem = "project_unreadable"
	// ProblemPATScope is an Azure DevOps PAT without a scope its Watcher
	// needs: Build for the CI Watcher, Work Items Read & Write for the
	// Work Item Watcher.
	ProblemPATScope WatcherProblem = "pat_scope"
	// ProblemQueryUnusable is an Azure Boards query that can't be run: a
	// Saved query not found, or a tree or links one, or a WIQL filter
	// Azure DevOps refuses.
	ProblemQueryUnusable WatcherProblem = "query_unusable"
	// ProblemTooManyWorkItems is an Azure Boards query matching more work
	// items than the Work Item Watcher takes in.
	ProblemTooManyWorkItems WatcherProblem = "too_many_work_items"
	// ProblemCheckFailed is a check that didn't run to an answer.
	ProblemCheckFailed WatcherProblem = "check_failed"
	// ProblemCrashed is a Watcher that exited without being stopped. The
	// service reports it, never the Watcher.
	ProblemCrashed WatcherProblem = "crashed"
)

// WatcherProblems are the problems.
func WatcherProblems() []WatcherProblem {
	return []WatcherProblem{ProblemGHMissing, ProblemLoggedOut, ProblemRepositoryUnreadable,
		ProblemIssuesDisabled, ProblemNoTriage, ProblemNoWrite, ProblemPATMissing, ProblemPATRejected,
		ProblemProjectUnreadable, ProblemPATScope, ProblemQueryUnusable, ProblemTooManyWorkItems, ProblemCheckFailed, ProblemCrashed}
}

// WatcherCheckResult is a Watcher's check mode's answer: no Problem when
// it can watch, else what is wrong and its fix.
type WatcherCheckResult struct {
	Problem WatcherProblem `json:"problem,omitempty"`
	Reason  string         `json:"reason,omitempty"`
	Fix     string         `json:"fix,omitempty"`
	// Repositories are the failing Repositories of a check that was given
	// a list of them, each with its own problem; Problem, Reason and Fix
	// then sum them up.
	Repositories []RepositoryProblem `json:"repositories,omitempty"`
}

// RepositoryProblem is one Repository a Watcher can't watch, by
// Repository Name, with what is wrong and its fix (Spec
// code-central-848): the Watcher carries on with the others.
type RepositoryProblem struct {
	Repository string         `json:"repository"`
	Problem    WatcherProblem `json:"problem,omitempty"`
	Message    string         `json:"message"`
	Fix        string         `json:"fix,omitempty"`
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
	// WatcherElsewhere is a Watcher another service runs, on the host
	// holding the Project's Watcher Lease (ADR 0068), whose status this
	// service doesn't know.
	WatcherElsewhere WatcherState = "elsewhere"
	// WatcherWaiting is a Watcher no service runs: the Project's Watcher
	// Lease lapsed, and no live service has taken it yet.
	WatcherWaiting WatcherState = "waiting"
)

// WatcherStates are the states a Watcher can be in.
func WatcherStates() []WatcherState {
	return []WatcherState{WatcherNotReported, WatcherWatching, WatcherError, WatcherElsewhere, WatcherWaiting}
}

// WatcherSteadyAfter is how long a restarted Watcher runs before its
// crash streak ends: its restart backoff starts again, and its own
// reports show again.
const WatcherSteadyAfter = 10 * time.Minute

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
