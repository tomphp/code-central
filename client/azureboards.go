package client

import (
	"context"
	"net/url"

	"github.com/tomoram/code-central/core"
)

// Azure Boards (Spec code-central-894).

// SetAzureBoards is the human's set_azure_boards; set Actor to human.
func (c *Client) SetAzureBoards(ctx context.Context, key string, in core.SetAzureBoardsInput) (core.Written[core.ProjectView], error) {
	return write[core.ProjectView](ctx, c, "/api/p/"+url.PathEscape(key)+"/set_azure_boards", in, in.Full)
}

// GetWorkItemWatch is the Work Item Watcher's get_work_item_watch.
func (c *Client) GetWorkItemWatch(ctx context.Context, key string) (core.WorkItemWatch, error) {
	var out core.WorkItemWatch
	return out, c.do(ctx, "GET", "/api/p/"+url.PathEscape(key)+"/integration/work-item-watch", nil, &out)
}

// ReportWorkItemWatcherStatus is the Work Item Watcher's
// report_work_item_watcher_status; set Actor to integration.
func (c *Client) ReportWorkItemWatcherStatus(ctx context.Context, key string, in core.ReportWatcherStatusInput) (core.Written[core.ProjectView], error) {
	return write[core.ProjectView](ctx, c, "/api/p/"+url.PathEscape(key)+"/integration/report_work_item_watcher_status", in, in.Full)
}
