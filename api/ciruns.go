package api

import (
	"github.com/tomoram/code-central/domain"
	"time"
)

// CIResult is a Ticket's CI result, from the runs of its pushed commits
// that aren't ignored: under the CI gate passed once any pushed since it
// last moved back to Ready passed, otherwise its latest pushed commit's.
type CIResult string

// CIResults are the CI results.
func CIResults() []CIResult { return []CIResult{CIRunning, CIPassed, CIFailed, CICancelled} }

// PushedCommitView is a commit the Factory pushed for a Ticket, and when
// Code Central heard of it.
type PushedCommitView struct {
	Commit string    `json:"commit"`
	Pushed time.Time `json:"pushed"`
}

// CIJobView is one job of a CI Run; its conclusion is empty until it
// ends.
type CIJobView struct {
	Name       string              `json:"name"`
	Conclusion domain.CIConclusion `json:"conclusion,omitempty"`
	URL        string              `json:"url"`
}

// CIRunView is one CI Run as reads return it, and as report_ci_runs
// takes it: the CI Watcher names what it is tracked against, a Ticket or
// a CI Rule's Spec; Code Central marks it ignored.
type CIRunView struct {
	// Repository is the Repository Name of the Repository the run is for,
	// empty for the Home Repository. With the CI Provider's run ID and
	// attempt, it keys the run.
	Repository string `json:"repository,omitempty"`
	ID         int64  `json:"id"`
	Attempt    int    `json:"attempt"`
	// Workflow is the workflow file's base name, such as ci.yml; Name is
	// the name it shows.
	Workflow   string              `json:"workflow"`
	Name       string              `json:"name"`
	Branch     string              `json:"branch"`
	Commit     string              `json:"commit"`
	Status     domain.CIRunStatus  `json:"status"`
	Conclusion domain.CIConclusion `json:"conclusion,omitempty"`
	Started    *time.Time          `json:"started,omitempty"`
	Finished   *time.Time          `json:"finished,omitempty"`
	URL        string              `json:"url"`
	Jobs       []CIJobView         `json:"jobs"`
	// Ticket or Rule is what the run is tracked against: a Ticket's ID,
	// or the ID of a CI Rule's Spec.
	Ticket string `json:"ticket,omitempty"`
	Rule   string `json:"rule,omitempty"`
	// Ignored marks a run of an Ignored workflow, which never counts
	// towards a CI result; Code Central sets it.
	Ignored bool `json:"ignored"`
}

// ReportPushedCommitInput is the Factory's report_pushed_commit(ticket,
// commit, repository?, full?).
type ReportPushedCommitInput struct {
	Ticket Ref    `json:"ticket" path:"n"`
	Commit string `json:"commit"`
	// Branch is the branch the Factory pushed the commit as, so that the
	// Pull Request Watcher knows the pull request's head branch.
	Branch string `json:"branch,omitempty"`
	// Repository is the Repository Name the Factory pushed the commit in;
	// left out, the Home Repository.
	Repository string `json:"repository,omitempty"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// ReportCIRunsInput is the CI Watcher's report_ci_runs(runs, full?).
type ReportCIRunsInput struct {
	Runs []CIRunView `json:"runs"`
	// Full asks for the runs as stored in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// CIRunsView is report_ci_runs' record: the runs it reported, as stored.
type CIRunsView struct {
	Runs []CIRunView `json:"runs"`
}

// CIWaitingCommit is a pushed commit the CI Watcher watches: one still
// waiting on a final CI result, or newly pushed.
type CIWaitingCommit struct {
	Ticket string    `json:"ticket"`
	Commit string    `json:"commit"`
	Pushed time.Time `json:"pushed"`
	// PullRequest is the number of the Ticket's pull request, when it has
	// one: Azure Pipelines builds on refs/pull/<n>/merge count towards
	// the commit's CI result.
	PullRequest int `json:"pull_request,omitempty"`
}

// The CI results.
const (
	// CIRunning is a pushed commit with a run still going, or none
	// reported yet.
	CIRunning CIResult = "running"
	// CIPassed is a pushed commit whose runs all finished, none failing.
	CIPassed CIResult = "passed"
	// CIFailed is a pushed commit with a run that concluded failure or
	// timed out.
	CIFailed CIResult = "failed"
	// CICancelled is a pushed commit with a cancelled run and none
	// failed.
	CICancelled CIResult = "cancelled"
)
