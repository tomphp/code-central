package api

import "context"

// GitHubIssuesView is a Project's GitHub Issues settings as every Project
// read returns them.
type GitHubIssuesView struct {
	On bool `json:"on"`
	// Repositories are the GitHub repositories (owner/name) its issues
	// come from, in the order the human listed them.
	Repositories []string `json:"repositories"`
	// Label is the filter label: only issues carrying it come in; empty
	// lets every issue in.
	Label           string `json:"label"`
	NewIssuesNeedMe bool   `json:"new_issues_need_me"`
	// Spec is the Issues Spec's ID, left out until GitHub Issues is first
	// turned on.
	Spec string `json:"spec,omitempty"`
	// Watcher is the Issue Watcher's status, as it last reported it.
	Watcher WatcherView `json:"watcher"`
}

// SetGitHubIssuesInput is the human's set_github_issues(on, repositories,
// label, new_issues_need_me, full?).
type SetGitHubIssuesInput struct {
	On           bool     `json:"on"`
	Repositories []string `json:"repositories"`
	// Label is the filter label; empty lets every issue in.
	Label           string `json:"label"`
	NewIssuesNeedMe bool   `json:"new_issues_need_me"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// IssuesCheckInput is what the Issue Watcher's check mode checks: that it
// can work each of the repositories' issues, finding gh with the gh path
// Option.
type IssuesCheckInput struct {
	Repositories []string `json:"repositories"`
	GHPath       string   `json:"gh_path,omitempty"`
}

// IssuesCheck runs the Issue Watcher once in its check mode. The service
// makes no external calls itself (ADR 0002, ADR 0004), so the check runs
// in the Issue Watcher's process.
type IssuesCheck func(ctx context.Context, in IssuesCheckInput) WatcherCheckResult
