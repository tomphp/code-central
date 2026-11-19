package client

import (
	"context"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/ops"
)

// CI Tracking (Spec code-central-247).

// SetCITracking is the human's set_ci_tracking; set Actor to human.
func (c *Client) SetCITracking(ctx context.Context, key string, in api.SetCITrackingInput) (api.Written[api.ProjectView], error) {
	return written(ctx, c, ops.SetCITracking, key, in, in.Full)
}

// SetRepositoryCI is the human's set_repository_ci: one Repository's CI
// entry.
func (c *Client) SetRepositoryCI(ctx context.Context, key string, in api.SetRepositoryCIInput) (api.Written[api.ProjectView], error) {
	return written(ctx, c, ops.SetRepositoryCI, key, in, in.Full)
}

// ReportPushRemote is the Factory's report_push_remote.
func (c *Client) ReportPushRemote(ctx context.Context, key string, in api.ReportPushRemoteInput) (api.Written[api.ProjectView], error) {
	return written(ctx, c, ops.ReportPushRemote, key, in, in.Full)
}

// AddCIRule is the human's add_ci_rule; set Actor to human.
func (c *Client) AddCIRule(ctx context.Context, key string, in api.CIRuleInput) (api.Written[api.ProjectView], error) {
	return written(ctx, c, ops.AddCIRule, key, in, in.Full)
}

// EditCIRule is the human's edit_ci_rule; set Actor to human.
func (c *Client) EditCIRule(ctx context.Context, key string, in api.CIRuleInput) (api.Written[api.ProjectView], error) {
	return written(ctx, c, ops.EditCIRule, key, in, in.Full)
}

// RemoveCIRule is the human's remove_ci_rule; set Actor to human.
func (c *Client) RemoveCIRule(ctx context.Context, key string, in api.RemoveCIRuleInput) (api.Written[api.ProjectView], error) {
	return written(ctx, c, ops.RemoveCIRule, key, in, in.Full)
}

// GetCIWatch is the CI Watcher's get_ci_watch.
func (c *Client) GetCIWatch(ctx context.Context, key string) (api.CIWatch, error) {
	return call(ctx, c, ops.GetCIWatch, key, ops.WatchRead{})
}

// ReportCIWatcherStatus is the CI Watcher's report_ci_watcher_status.
func (c *Client) ReportCIWatcherStatus(ctx context.Context, key string, in api.ReportWatcherStatusInput) (api.Written[api.ProjectView], error) {
	return written(ctx, c, ops.ReportCIWatcherStatus, key, in, in.Full)
}

// ReportPullRequestWatcherStatus is the Pull Request Watcher's
// report_pull_request_watcher_status.
func (c *Client) ReportPullRequestWatcherStatus(ctx context.Context, key string, in api.ReportWatcherStatusInput) (api.Written[api.ProjectView], error) {
	return written(ctx, c, ops.ReportPullRequestWatcherStatus, key, in, in.Full)
}

// ReportPushedCommit is the Factory's report_pushed_commit.
func (c *Client) ReportPushedCommit(ctx context.Context, key string, in api.ReportPushedCommitInput) (api.Written[api.TicketView], error) {
	return written(ctx, c, ops.ReportPushedCommit, key, in, in.Full)
}

// ReportSpecMerge is the Factory's report_spec_merge.
func (c *Client) ReportSpecMerge(ctx context.Context, key string, in api.ReportSpecMergeInput) (api.Written[api.TicketView], error) {
	return written(ctx, c, ops.ReportSpecMerge, key, in, in.Full)
}

// ReportCIRuns is the CI Watcher's report_ci_runs.
func (c *Client) ReportCIRuns(ctx context.Context, key string, in api.ReportCIRunsInput) (api.Written[api.CIRunsView], error) {
	return written(ctx, c, ops.ReportCIRuns, key, in, in.Full)
}

// GetPullRequestWatch is the Pull Request Watcher's get_pull_request_watch.
func (c *Client) GetPullRequestWatch(ctx context.Context, key string) (api.PullRequestWatch, error) {
	return call(ctx, c, ops.GetPullRequestWatch, key, ops.WatchRead{})
}

// ReportPullRequest is the Pull Request Watcher's report_pull_request.
func (c *Client) ReportPullRequest(ctx context.Context, key string, in api.ReportPullRequestInput) (api.Written[api.PullRequestReport], error) {
	return written(ctx, c, ops.ReportPullRequest, key, in, in.Full)
}
