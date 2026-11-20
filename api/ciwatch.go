package api

import (
	"github.com/tomoram/code-central/domain"
	"time"
)

// CIWatch is get_ci_watch: everything the CI Watcher needs on each poll.
// The gh path Option is read on every poll, so changing it needs no
// restart. Provider, Repository, ProjectURL, Waiting and Reruns are the
// Home Repository's; Repositories hold every other watched Repository's.
type CIWatch struct {
	Project  string `json:"project"`
	Tracking bool   `json:"tracking"`
	// NotWatched is whether the Home Repository is Not watched (Spec
	// code-central-890): the CI Watcher then leaves it alone. Provider,
	// Repository and ProjectURL are the Home Repository's CI otherwise.
	NotWatched bool              `json:"not_watched,omitempty"`
	Provider   domain.CIProvider `json:"provider"`
	Repository string            `json:"repository"`
	ProjectURL string            `json:"project_url"`
	// PATEnv is the name of the environment variable holding the Azure
	// DevOps PAT, which the CI Watcher reads from its own environment.
	PATEnv           string        `json:"pat_env"`
	IgnoredWorkflows []string      `json:"ignored_workflows"`
	Rules            []CIWatchRule `json:"rules"`
	GHPath           string        `json:"gh_path"`
	// Waiting are the pushed commits still waiting on a final CI result,
	// or newly pushed, oldest push first: the CI Watcher lists their runs.
	Waiting []CIWaitingCommit `json:"waiting"`
	// Reruns are the Re-run requests waiting on the CI Watcher: it
	// re-runs each run's failed jobs, and a request clears once it
	// reports the run's new attempt.
	Reruns []CIRerun `json:"reruns"`
	// Repositories are the watched Repositories other than the Home, in
	// landing order, each with its own CI Provider, target, waiting
	// commits and Re-run requests; Not watched ones are left out. The CI
	// Watcher reports their runs naming the Repository.
	Repositories []CIWatchRepository `json:"repositories"`
	// Pushed is every commit pushed for a Ticket, whose runs never fire a
	// CI Rule, and Landed every commit a Ticket landed at, which a fix
	// Ticket links; both by Ticket.
	Pushed []CITicketCommit `json:"pushed"`
	Landed []CITicketCommit `json:"landed"`
}

// CIWatchRepository is one watched Repository other than the Home, as
// the CI Watcher watches it.
type CIWatchRepository struct {
	Name       string            `json:"name"`
	Provider   domain.CIProvider `json:"provider"`
	Repository string            `json:"repository,omitempty"`
	ProjectURL string            `json:"project_url,omitempty"`
	Waiting    []CIWaitingCommit `json:"waiting"`
	Reruns     []CIRerun         `json:"reruns"`
}

// CIRerun is one Re-run request: a run that failed on a Ticket's pushed
// commit, at the attempt that failed, and when the human asked.
type CIRerun struct {
	Ticket    string    `json:"ticket"`
	Commit    string    `json:"commit"`
	ID        int64     `json:"id"`
	Attempt   int       `json:"attempt"`
	Requested time.Time `json:"requested"`
}

// CIWatchRule is a CI Rule as the CI Watcher reads it: with when the
// latest run tracked against it finished, so it fires the rule only for
// runs finishing after that, and after the rule was created.
type CIWatchRule struct {
	CIRuleView
	// LastRun is nil until a run is tracked against the rule.
	LastRun *time.Time `json:"last_run,omitempty"`
}

// CITicketCommit is a commit pushed for, or landed by, a Ticket on Spec,
// in the Repository named, empty for the Home Repository.
type CITicketCommit struct {
	Ticket     string `json:"ticket"`
	Spec       string `json:"spec"`
	Repository string `json:"repository,omitempty"`
	Commit     string `json:"commit"`
}
