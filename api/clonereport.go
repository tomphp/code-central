package api

import (
	"time"
)

// CloneState is the state of a Repository's clone as the Factory reports
// it.
type CloneState string

// CloneStates are the states of a reported clone.
func CloneStates() []CloneState { return []CloneState{CloneReady, CloneMissing} }

// RepositoryReport is one Repository's clone in report_repositories.
type RepositoryReport struct {
	Name string `json:"name"`
	// Path is where the Factory looks for the clone.
	Path  string     `json:"path"`
	State CloneState `json:"state"`
	// Problem says what is wrong at Path; required for missing.
	Problem string `json:"problem,omitempty"`
}

// ReportRepositoriesInput is the Factory's report_repositories(
// repositories, full?).
type ReportRepositoriesInput struct {
	Repositories []RepositoryReport `json:"repositories"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// CloneView is a Repository's clone as the Factory last reported it;
// absent while no Factory is reporting.
type CloneView struct {
	State   CloneState `json:"state"`
	Path    string     `json:"path"`
	Problem string     `json:"problem,omitempty"`
	// Since is when the Factory first reported it in this state with
	// this problem.
	Since time.Time `json:"since"`
}

// RepositoryMissingView is a Repository missing item's detail: the
// Repository, its path and Clone URL, what is wrong and the fix.
type RepositoryMissingView struct {
	Repository string `json:"repository"`
	Path       string `json:"path"`
	CloneURL   string `json:"clone_url"`
	Problem    string `json:"problem"`
	Fix        string `json:"fix"`
}

const (
	// CloneReady is a clone the Factory made or adopted.
	CloneReady CloneState = "ready"
	// CloneMissing is Repository missing: the path holds something other
	// than a clone of the Clone URL, or cloning it failed; for a
	// local-only Repository, no git repository of its own; or the path
	// is another Repository's.
	CloneMissing CloneState = "missing"
)
