package api

import (
	"time"
)

// RepositoryWork is a Ticket's branch and its head commit in one
// Repository, as finish and report_changed_repositories take them.
type RepositoryWork struct {
	Name   string `json:"name" desc:"The Repository Name." required:"true"`
	Branch string `json:"branch" desc:"The branch the work is on there." required:"true"`
	Commit string `json:"commit" desc:"The branch's head commit there." required:"true"`
}

// ChangedRepositoryView is one of a Ticket's Changed Repositories as
// reads show it, in landing order.
type ChangedRepositoryView struct {
	Name   string `json:"name"`
	Branch string `json:"branch,omitempty"`
	Commit string `json:"commit,omitempty"`
	// Landed is the commit that put the work into the Repository's base;
	// left out until it lands.
	Landed *LandedView `json:"landed,omitempty"`
	// NotLanded is why an Accepted Ticket hasn't landed in the Repository
	// yet: its broken base, or what the Factory last said; left out once
	// it lands, or while there's none to give.
	NotLanded string `json:"not_landed,omitempty"`
	// PushedCommit is the latest commit the Factory pushed for the Ticket
	// in the Repository; left out while it has pushed none.
	PushedCommit *PushedCommitView `json:"pushed_commit,omitempty"`
	// PullRequest is the Ticket's pull request in the Repository; left
	// out until the Pull Request Watcher reports one.
	PullRequest *PullRequestView `json:"pull_request,omitempty"`
	// CIResult is the Repository's CI result, from the pushed commit
	// CICommit; both are left out while it has none. CINotWatched says CI
	// Tracking never watches the Repository: it has no CI result.
	CIResult     CIResult `json:"ci_result,omitempty"`
	CICommit     string   `json:"ci_commit,omitempty"`
	CINotWatched bool     `json:"ci_not_watched,omitempty"`
	// LandAnyway says the human accepted the failed CI result of the
	// latest pushed commit there, so the Factory lands it there.
	LandAnyway bool `json:"land_anyway,omitempty"`
	// CIAutoFix is what Auto-fix does about the Ticket's CI failure
	// there; left out while it does nothing.
	CIAutoFix *CIAutoFixView `json:"ci_auto_fix,omitempty"`
}

// LandedView is the commit that put a Ticket's work into a Repository's
// base, and when Code Central heard of it.
type LandedView struct {
	Commit string    `json:"commit"`
	Time   time.Time `json:"time"`
}

// ReportChangedRepositoriesInput is the Factory's
// report_changed_repositories(ticket, claim_id, repositories, full?): the
// Repositories whose branch holds the Ticket's work so far, each with its
// branch and head commit.
type ReportChangedRepositoriesInput struct {
	Ticket       Ref              `json:"ticket" path:"n"`
	ClaimID      string           `json:"claim_id"`
	Repositories []RepositoryWork `json:"repositories"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}
