package client

import (
	"context"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/ops"
)

// Azure Boards (Spec code-central-894).

// SetAzureBoards is the human's set_azure_boards; set Actor to human.
func (c *Client) SetAzureBoards(ctx context.Context, key string, in api.SetAzureBoardsInput) (api.Written[api.ProjectView], error) {
	return written(ctx, c, ops.SetAzureBoards, key, in, in.Full)
}

// GetWorkItemWatch is the Work Item Watcher's get_work_item_watch.
func (c *Client) GetWorkItemWatch(ctx context.Context, key string) (api.WorkItemWatch, error) {
	return call(ctx, c, ops.GetWorkItemWatch, key, api.WatchInput{})
}

// ReportWorkItemWatcherStatus is the Work Item Watcher's
// report_work_item_watcher_status; set Actor to integration.
func (c *Client) ReportWorkItemWatcherStatus(ctx context.Context, key string, in api.ReportWatcherStatusInput) (api.Written[api.ProjectView], error) {
	return written(ctx, c, ops.ReportWorkItemWatcherStatus, key, in, in.Full)
}
