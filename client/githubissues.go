package client

import (
	"context"
	"net/url"

	"github.com/tomoram/code-central/core"
)

// GitHub Issues (Spec code-central-279).

// SetGitHubIssues is the human's set_github_issues; set Actor to human.
func (c *Client) SetGitHubIssues(ctx context.Context, key string, in core.SetGitHubIssuesInput) (core.Written[core.ProjectView], error) {
	return write[core.ProjectView](ctx, c, "/api/p/"+url.PathEscape(key)+"/set_github_issues", in, in.Full)
}

// GetIssueWatch is the Issue Watcher's get_issue_watch.
func (c *Client) GetIssueWatch(ctx context.Context, key string) (core.IssueWatch, error) {
	var out core.IssueWatch
	return out, c.do(ctx, "GET", "/api/p/"+url.PathEscape(key)+"/integration/issue-watch", nil, &out)
}

// ReportIssueWatcherStatus is the Issue Watcher's
// report_issue_watcher_status; set Actor to integration.
func (c *Client) ReportIssueWatcherStatus(ctx context.Context, key string, in core.ReportWatcherStatusInput) (core.Written[core.ProjectView], error) {
	return write[core.ProjectView](ctx, c, "/api/p/"+url.PathEscape(key)+"/integration/report_issue_watcher_status", in, in.Full)
}

// ReportIssueWritten is the Issue Watcher's report_issue_written; set
// Actor to integration.
func (c *Client) ReportIssueWritten(ctx context.Context, key string, in core.ReportIssueWrittenInput) (core.LinkedRow, error) {
	var out core.LinkedRow
	return out, c.do(ctx, "POST", "/api/p/"+url.PathEscape(key)+"/integration/report_issue_written", in, &out)
}
