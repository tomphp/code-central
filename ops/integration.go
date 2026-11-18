package ops

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// The Integration contract: what an Integration, the Issue Watcher and the
// Work Item Watcher call, over HTTP only, and the human's unlink.

// CreateLinkedSpec is the integration's create_linked_spec.
var CreateLinkedSpec = &Definition[core.CreateLinkedSpecInput, core.Written[core.SpecView]]{
	Name: "create_linked_spec", Audience: IntegrationOnly, Write: true,
	HTTP: "POST /api/p/{key}/integration/create_linked_spec",
	Doc:  "Create a Spec linked to an external item.",
	Call: (*core.Core).CreateLinkedSpec,
}

// CreateLinkedTicket is the integration's create_linked_ticket.
var CreateLinkedTicket = &Definition[core.CreateLinkedTicketInput, core.Written[core.TicketView]]{
	Name: "create_linked_ticket", Audience: IntegrationOnly, Write: true,
	HTTP: "POST /api/p/{key}/integration/create_linked_ticket",
	Doc:  "Create a Ticket linked to an external item.",
	Call: (*core.Core).CreateLinkedTicket,
}

// UpdateLinkedSpec is the integration's update_linked_spec.
var UpdateLinkedSpec = &Definition[core.UpdateLinkedSpecInput, core.Written[core.SpecView]]{
	Name: "update_linked_spec", Audience: IntegrationOnly, Write: true,
	HTTP: "POST /api/p/{key}/integration/specs/{n}/update_linked_spec",
	Doc:  "Update a linked Spec whose Link owns its definition.",
	Call: (*core.Core).UpdateLinkedSpec,
}

// UpdateLinkedTicket is the integration's update_linked_ticket.
var UpdateLinkedTicket = &Definition[core.UpdateLinkedTicketInput, core.Written[core.TicketView]]{
	Name: "update_linked_ticket", Audience: IntegrationOnly, Write: true,
	HTTP: "POST /api/p/{key}/integration/tickets/{n}/update_linked_ticket",
	Doc:  "Update a linked Ticket whose Link owns its definition.",
	Call: (*core.Core).UpdateLinkedTicket,
}

// SetExternalState is the integration's set_external_state.
var SetExternalState = &Definition[core.SetExternalStateInput, core.LinkedRow]{
	Name: "set_external_state", Audience: IntegrationOnly, Write: true,
	HTTP: "POST /api/p/{key}/integration/set_external_state",
	Doc:  "Record a linked item's external status as last seen.",
	Call: (*core.Core).SetExternalState,
}

// FindLinked is find_linked, detached links included.
var FindLinked = &Definition[core.FindLinkedInput, []core.LinkedRow]{
	Name: "find_linked", Audience: IntegrationOnly,
	HTTP: "GET /api/p/{key}/integration/linked",
	Doc:  "Find the Specs and Tickets an Integration has linked, detached ones too.",
	Call: (*core.Core).FindLinked,
}

// Unlink is the human's unlink.
var Unlink = &Definition[core.UnlinkInput, core.LinkedRow]{
	Name: "unlink", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/unlink",
	Doc:  "Detach a linked Spec or Ticket from its external item.",
	Call: (*core.Core).Unlink,
}

// GetIssueWatch is the Issue Watcher's read.
var GetIssueWatch = &Definition[core.WatchInput, core.IssueWatch]{
	Name: "get_issue_watch", Audience: IntegrationOnly,
	HTTP: "GET /api/p/{key}/integration/issue-watch",
	Doc:  "Get what the Issue Watcher polls for.",
	Call: func(c *core.Core, ctx context.Context, key string, _ core.WatchInput) (core.IssueWatch, error) {
		return c.GetIssueWatch(ctx, key)
	},
}

// ReportIssueWatcherStatus is the Issue Watcher's status report.
var ReportIssueWatcherStatus = &Definition[core.ReportWatcherStatusInput, core.Written[core.ProjectView]]{
	Name: "report_issue_watcher_status", Audience: IntegrationOnly, Write: true,
	HTTP: "POST /api/p/{key}/integration/report_issue_watcher_status",
	Doc:  "Report the Issue Watcher's state.",
	Call: (*core.Core).ReportIssueWatcherStatus,
}

// ReportIssueWritten is the Watchers' report of a comment written to an
// external item.
var ReportIssueWritten = &Definition[core.ReportIssueWrittenInput, core.LinkedRow]{
	Name: "report_issue_written", Audience: IntegrationOnly, Write: true,
	HTTP: "POST /api/p/{key}/integration/report_issue_written",
	Doc:  "Report that a comment was written to a linked item's external issue, so it is never due again.",
	Call: (*core.Core).ReportIssueWritten,
}

// GetWorkItemWatch is the Work Item Watcher's read.
var GetWorkItemWatch = &Definition[core.WatchInput, core.WorkItemWatch]{
	Name: "get_work_item_watch", Audience: IntegrationOnly,
	HTTP: "GET /api/p/{key}/integration/work-item-watch",
	Doc:  "Get what the Work Item Watcher polls for.",
	Call: func(c *core.Core, ctx context.Context, key string, _ core.WatchInput) (core.WorkItemWatch, error) {
		return c.GetWorkItemWatch(ctx, key)
	},
}

// ReportWorkItemWatcherStatus is the Work Item Watcher's status report.
var ReportWorkItemWatcherStatus = &Definition[core.ReportWatcherStatusInput, core.Written[core.ProjectView]]{
	Name: "report_work_item_watcher_status", Audience: IntegrationOnly, Write: true,
	HTTP: "POST /api/p/{key}/integration/report_work_item_watcher_status",
	Doc:  "Report the Work Item Watcher's state.",
	Call: (*core.Core).ReportWorkItemWatcherStatus,
}
