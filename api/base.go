package api

import (
	"time"
)

// BaseState is the state of the local base a Factory reports.
type BaseState string

// BaseStates are the states of a reported base.
func BaseStates() []BaseState { return []BaseState{BaseCurrent, BaseDiverged, BaseMissing} }

// BaseView is a Project's reported base, as its Factory Status carries it;
// absent while no Factory is reporting.
type BaseView struct {
	State BaseState `json:"state"`
	// Local and Remote are the local and remote base commits.
	Local  string `json:"local"`
	Remote string `json:"remote"`
	// Branch is the base branch's name, when the Factory reported it.
	Branch string    `json:"branch,omitempty"`
	At     time.Time `json:"at"`
}

// BaseDivergedView is a Base diverged item's detail: both commits and the
// base branch, and the Repository whose base it is while the Project has
// several.
type BaseDivergedView struct {
	Repository string `json:"repository,omitempty"`
	Base       string `json:"base,omitempty"`
	Local      string `json:"local"`
	Remote     string `json:"remote"`
}

// BaseMissingView is a Base missing item's detail: the branch and the fix,
// and the Repository whose base it is while the Project has several.
type BaseMissingView struct {
	Repository string `json:"repository,omitempty"`
	Base       string `json:"base"`
	Fix        string `json:"fix"`
}

// ReportBaseInput is the Factory's report_base(state, local, remote,
// repository?, full?).
type ReportBaseInput struct {
	State BaseState `json:"state"`
	// Local and Remote are the local and remote base commits.
	// They are required for current and diverged, and ignored for missing.
	Local  string `json:"local"`
	Remote string `json:"remote"`
	// Repository is the Repository Name whose base this is; blank for the
	// Home Repository, as a Factory from before Repositories reports it
	// (Spec code-central-848).
	Repository string `json:"repository,omitempty"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

const (
	BaseCurrent  BaseState = "current"
	BaseDiverged BaseState = "diverged"
	// BaseMissing is a base that exists neither locally nor on the Push
	// remote, or in a local-only Repository, never looked for on a
	// remote, doesn't exist locally; it carries no commits.
	BaseMissing BaseState = "missing"
)
