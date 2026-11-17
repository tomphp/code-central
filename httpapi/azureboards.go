package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// Azure Boards (Spec code-central-894): the human's Azure Boards settings.
func init() {
	Routes["set_azure_boards"] = "POST /api/p/{key}/set_azure_boards"

	handlers["set_azure_boards"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.SetAzureBoardsInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.SetAzureBoards(r.Context(), r.PathValue("key"), in)
	}
}

// The Work Item Watcher's own: the read it polls and its status. It
// reports each comment it writes to a work item with
// report_issue_written.
func init() {
	Routes["get_work_item_watch"] = "GET /api/p/{key}/integration/work-item-watch"
	Routes["report_work_item_watcher_status"] = "POST /api/p/{key}/integration/report_work_item_watcher_status"

	handlers["get_work_item_watch"] = func(c *core.Core, r *http.Request) (any, error) {
		return c.GetWorkItemWatch(r.Context(), r.PathValue("key"))
	}
	handlers["report_work_item_watcher_status"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.ReportWatcherStatusInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.ReportWorkItemWatcherStatus(r.Context(), r.PathValue("key"), in)
	}
}
