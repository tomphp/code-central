package client

import (
	"context"
	"net/url"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ops"
)

// Azure Boards (Spec code-central-894).

// SetAzureBoards is the human's set_azure_boards; set Actor to human.
func (c *Client) SetAzureBoards(ctx context.Context, key string, in core.SetAzureBoardsInput) (core.Written[core.ProjectView], error) {
	return write[core.ProjectView](ctx, c, "/api/p/"+url.PathEscape(key)+"/set_azure_boards", in, in.Full)
}

// GetWorkItemWatch is the Work Item Watcher's get_work_item_watch.
func (c *Client) GetWorkItemWatch(ctx context.Context, key string) (core.WorkItemWatch, error) {
	return call(ctx, c, ops.GetWorkItemWatch, key, core.WatchInput{})
}

// ReportWorkItemWatcherStatus is the Work Item Watcher's
// report_work_item_watcher_status; set Actor to integration.
func (c *Client) ReportWorkItemWatcherStatus(ctx context.Context, key string, in core.ReportWatcherStatusInput) (core.Written[core.ProjectView], error) {
	return written(ctx, c, ops.ReportWorkItemWatcherStatus, key, in, in.Full)
}
