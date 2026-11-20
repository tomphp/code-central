package api

import (
	"time"
)

// SpecMergeView is the Spec Branch commit a Ticket's work was merged
// into and when Code Central heard of it; Counts is false for a merge no
// later than the Ticket's last move back to Ready.
type SpecMergeView struct {
	// Repository is the Repository Name of the Spec Branch, left out for
	// the Home Repository.
	Repository string    `json:"repository,omitempty"`
	Commit     string    `json:"commit"`
	Merged     time.Time `json:"merged"`
	Counts     bool      `json:"counts"`
}

// LandPerSpecView is a Spec's Land per Spec status on a Spec read,
// present only while the Project is under Merge with Land per Spec on.
type LandPerSpecView struct {
	// Waiting are the Accepted Tickets waiting for the Spec to land.
	Waiting []string `json:"waiting"`
	// Ready is whether the Spec is ready to land: the Factory merges it
	// into the base.
	Ready bool `json:"ready"`
	// Pushed are the Repositories, by Repository Name, where one of the
	// Spec's Tickets, finished or not, has a pushed commit reported on
	// its Spec Branch: the Factory pushed it there, so a branch of that
	// name on the Push remote is its own. Left out while there are none.
	Pushed []string `json:"pushed,omitempty"`
}

// ReportSpecMergeInput is the Factory's report_spec_merge(ticket, commit,
// full?).
type ReportSpecMergeInput struct {
	Ticket Ref    `json:"ticket" path:"n"`
	Commit string `json:"commit"`
	// Repository is the Repository Name of the Spec Branch the commit is
	// on; left out, the Home Repository.
	Repository string `json:"repository,omitempty"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}
