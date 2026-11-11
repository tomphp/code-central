package client

import (
	"context"
	"net/url"

	"github.com/tomoram/code-central/core"
)

// CI Tracking (Spec code-central-247).

// SetCITracking is the human's set_ci_tracking; set Actor to human.
func (c *Client) SetCITracking(ctx context.Context, key string, in core.SetCITrackingInput) (core.Written[core.ProjectView], error) {
	return write[core.ProjectView](ctx, c, "/api/p/"+url.PathEscape(key)+"/set_ci_tracking", in, in.Full)
}

// ReportPushRemote is the Factory's report_push_remote.
func (c *Client) ReportPushRemote(ctx context.Context, key string, in core.ReportPushRemoteInput) (core.Written[core.ProjectView], error) {
	return write[core.ProjectView](ctx, c, "/api/p/"+url.PathEscape(key)+"/report_push_remote", in, in.Full)
}

// AddCIRule is the human's add_ci_rule; set Actor to human.
func (c *Client) AddCIRule(ctx context.Context, key string, in core.CIRuleInput) (core.Written[core.ProjectView], error) {
	return write[core.ProjectView](ctx, c, "/api/p/"+url.PathEscape(key)+"/add_ci_rule", in, in.Full)
}

// EditCIRule is the human's edit_ci_rule; set Actor to human.
func (c *Client) EditCIRule(ctx context.Context, key string, in core.CIRuleInput) (core.Written[core.ProjectView], error) {
	return write[core.ProjectView](ctx, c, "/api/p/"+url.PathEscape(key)+"/edit_ci_rule", in, in.Full)
}

// RemoveCIRule is the human's remove_ci_rule; set Actor to human.
func (c *Client) RemoveCIRule(ctx context.Context, key string, in core.RemoveCIRuleInput) (core.Written[core.ProjectView], error) {
	return write[core.ProjectView](ctx, c, "/api/p/"+url.PathEscape(key)+"/remove_ci_rule", in, in.Full)
}

// GetCIWatch is the CI Watcher's get_ci_watch.
func (c *Client) GetCIWatch(ctx context.Context, key string) (core.CIWatch, error) {
	var out core.CIWatch
	return out, c.do(ctx, "GET", "/api/p/"+url.PathEscape(key)+"/ci-watch", nil, &out)
}

// ReportCIWatcherStatus is the CI Watcher's report_ci_watcher_status.
func (c *Client) ReportCIWatcherStatus(ctx context.Context, key string, in core.ReportWatcherStatusInput) (core.Written[core.ProjectView], error) {
	return write[core.ProjectView](ctx, c, "/api/p/"+url.PathEscape(key)+"/report_ci_watcher_status", in, in.Full)
}

// ReportPushedCommit is the Factory's report_pushed_commit.
func (c *Client) ReportPushedCommit(ctx context.Context, key string, in core.ReportPushedCommitInput) (core.Written[core.TicketView], error) {
	return write[core.TicketView](ctx, c, item(key, "tickets", in.Ticket, "report_pushed_commit"), in, in.Full)
}

// ReportSpecMerge is the Factory's report_spec_merge.
func (c *Client) ReportSpecMerge(ctx context.Context, key string, in core.ReportSpecMergeInput) (core.Written[core.TicketView], error) {
	return write[core.TicketView](ctx, c, item(key, "tickets", in.Ticket, "report_spec_merge"), in, in.Full)
}

// ReportCIRuns is the CI Watcher's report_ci_runs.
func (c *Client) ReportCIRuns(ctx context.Context, key string, in core.ReportCIRunsInput) (core.Written[core.CIRunsView], error) {
	return write[core.CIRunsView](ctx, c, "/api/p/"+url.PathEscape(key)+"/report_ci_runs", in, in.Full)
}

// GetPullRequestWatch is the Pull Request Watcher's get_pull_request_watch.
func (c *Client) GetPullRequestWatch(ctx context.Context, key string) (core.PullRequestWatch, error) {
	var out core.PullRequestWatch
	return out, c.do(ctx, "GET", "/api/p/"+url.PathEscape(key)+"/pull-request-watch", nil, &out)
}

// ReportPullRequest is the Pull Request Watcher's report_pull_request.
func (c *Client) ReportPullRequest(ctx context.Context, key string, in core.ReportPullRequestInput) (core.Written[core.PullRequestReport], error) {
	return write[core.PullRequestReport](ctx, c, "/api/p/"+url.PathEscape(key)+"/report_pull_request", in, in.Full)
}
