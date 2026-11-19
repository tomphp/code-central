package api

import (
	"github.com/tomoram/code-central/domain"
	"time"
)

// PullRequestView is a pull request as reads show it: on a Ticket, on the
// Spec page and in the Pull Request Watcher's list.
type PullRequestView struct {
	// Repository is the Repository Name the pull request is in; left out
	// while the Project has one Repository.
	Repository string                  `json:"repository,omitempty"`
	Number     int                     `json:"number"`
	Link       string                  `json:"link"`
	State      domain.PullRequestState `json:"state"`
	// MergeCommit is the commit the Code Host merged it at, once merged.
	MergeCommit string `json:"merge_commit,omitempty"`
}

// PullRequestWatch is get_pull_request_watch: everything the Pull Request
// Watcher needs on each poll. Work and Close are empty unless the Landing
// Method is Pull Request.
type PullRequestWatch struct {
	Project string `json:"project"`
	// On says the Landing Method is Pull Request.
	On bool `json:"on"`
	// CodeHost, Repository and Base are the Home Repository's.
	CodeHost   CodeHost `json:"code_host"`
	Repository string   `json:"repository"`
	Base       string   `json:"base"`
	// Repositories are every Repository the watcher watches, in landing
	// order, the Home Repository's included.
	Repositories []PullRequestRepositoryView `json:"repositories"`
	// PATEnv names the environment variable holding the Azure Repos PAT,
	// for every Repository on Azure Repos.
	PATEnv string `json:"pat_env"`
	GHPath string `json:"gh_path"`
	// Work is every piece of pushed work to watch, by when it was pushed.
	Work []PushedWorkView `json:"work"`
	// Close is every open pull request marked to close.
	Close []PullRequestToClose `json:"close"`
}

// PullRequestRepositoryView is one Repository as the Pull Request Watcher
// works it: its Code Host and the repository there, worked out from its
// Clone URL, and its Base.
type PullRequestRepositoryView struct {
	Name     string   `json:"name"`
	CodeHost CodeHost `json:"code_host"`
	// Repository is owner/name on GitHub and
	// organization/project/repository on Azure Repos.
	Repository string `json:"repository"`
	Base       string `json:"base"`
}

// PushedWorkView is one piece of pushed work: a Ticket's branch in one
// Repository, with its latest pushed commit there, what to open its pull
// request with, and its current pull request, if any.
type PushedWorkView struct {
	// Repository is the Repository Name the work is in.
	Repository string    `json:"repository"`
	Branch     string    `json:"branch"`
	Commit     string    `json:"commit"`
	Pushed     time.Time `json:"pushed"`
	// Tickets are the Tickets the work covers.
	Tickets []string `json:"tickets"`
	Title   string   `json:"title"`
	// Body is what the pull request's body should be: the watcher opens
	// it with that, and with several Repositories edits it to match.
	Body string `json:"body"`
	// PullRequest is nil until the Watcher has reported one.
	PullRequest *PullRequestView `json:"pull_request,omitempty"`
}

// PullRequestToClose is an open pull request marked to close, as sending
// its Ticket back marks its open pull requests while none has merged.
type PullRequestToClose struct {
	Repository string `json:"repository"`
	Branch     string `json:"branch"`
	Number     int    `json:"number"`
	Link       string `json:"link"`
}

// ReportPullRequestInput is the Pull Request Watcher's
// report_pull_request(repository?, branch, number, link, state,
// merge_commit?, full?).
type ReportPullRequestInput struct {
	// Repository is the Repository Name the pull request is in; empty for
	// the Home Repository.
	Repository string `json:"repository,omitempty"`
	// Branch is the pushed work's branch.
	Branch string                  `json:"branch"`
	Number int                     `json:"number"`
	Link   string                  `json:"link"`
	State  domain.PullRequestState `json:"state"`
	// MergeCommit is the commit the Code Host merged the pull request at;
	// needed with the merged state.
	MergeCommit string `json:"merge_commit,omitempty"`
	// Full asks for the Tickets covered in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// PullRequestReport is report_pull_request's record: the Tickets it
// recorded the pull request on.
type PullRequestReport struct {
	Branch  string          `json:"branch"`
	Tickets []string        `json:"tickets"`
	Record  PullRequestView `json:"pull_request"`
}
