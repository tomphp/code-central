package client

import (
	"context"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/ops"
)

// GitHub Issues (Spec code-central-279).

// SetGitHubIssues is the human's set_github_issues; set Actor to human.
func (c *Client) SetGitHubIssues(ctx context.Context, key string, in api.SetGitHubIssuesInput) (api.Written[api.ProjectView], error) {
	return written(ctx, c, ops.SetGitHubIssues, key, in, in.Full)
}

// GetIssueWatch is the Issue Watcher's get_issue_watch.
func (c *Client) GetIssueWatch(ctx context.Context, key string) (api.IssueWatch, error) {
	return call(ctx, c, ops.GetIssueWatch, key, api.WatchInput{})
}

// ReportIssueWatcherStatus is the Issue Watcher's
// report_issue_watcher_status; set Actor to integration.
func (c *Client) ReportIssueWatcherStatus(ctx context.Context, key string, in api.ReportWatcherStatusInput) (api.Written[api.ProjectView], error) {
	return written(ctx, c, ops.ReportIssueWatcherStatus, key, in, in.Full)
}

// ReportIssueWritten is the Issue Watcher's report_issue_written; set
// Actor to integration.
func (c *Client) ReportIssueWritten(ctx context.Context, key string, in api.ReportIssueWrittenInput) (api.LinkedRow, error) {
	return call(ctx, c, ops.ReportIssueWritten, key, in)
}
