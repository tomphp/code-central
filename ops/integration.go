package ops

import (
	"github.com/tomoram/code-central/api"
)

// The Integration contract: what an Integration, the Issue Watcher and the
// Work Item Watcher call, over HTTP only, and the human's unlink.

// CreateLinkedSpec is the integration's create_linked_spec.
var CreateLinkedSpec = &Definition[api.CreateLinkedSpecInput, api.Written[api.SpecView]]{
	Name: "create_linked_spec", Audience: IntegrationOnly, Write: true,
	HTTP: "POST /api/p/{key}/integration/create_linked_spec",
	Doc:  "Create a Spec linked to an external item.",
}

// CreateLinkedTicket is the integration's create_linked_ticket.
var CreateLinkedTicket = &Definition[api.CreateLinkedTicketInput, api.Written[api.TicketView]]{
	Name: "create_linked_ticket", Audience: IntegrationOnly, Write: true,
	HTTP: "POST /api/p/{key}/integration/create_linked_ticket",
	Doc:  "Create a Ticket linked to an external item.",
}

// UpdateLinkedSpec is the integration's update_linked_spec.
var UpdateLinkedSpec = &Definition[api.UpdateLinkedSpecInput, api.Written[api.SpecView]]{
	Name: "update_linked_spec", Audience: IntegrationOnly, Write: true,
	HTTP: "POST /api/p/{key}/integration/specs/{n}/update_linked_spec",
	Doc:  "Update a linked Spec whose Link owns its definition.",
}

// UpdateLinkedTicket is the integration's update_linked_ticket.
var UpdateLinkedTicket = &Definition[api.UpdateLinkedTicketInput, api.Written[api.TicketView]]{
	Name: "update_linked_ticket", Audience: IntegrationOnly, Write: true,
	HTTP: "POST /api/p/{key}/integration/tickets/{n}/update_linked_ticket",
	Doc:  "Update a linked Ticket whose Link owns its definition.",
}

// SetExternalState is the integration's set_external_state.
var SetExternalState = &Definition[api.SetExternalStateInput, api.LinkedRow]{
	Name: "set_external_state", Audience: IntegrationOnly, Write: true,
	HTTP: "POST /api/p/{key}/integration/set_external_state",
	Doc:  "Record a linked item's external status as last seen.",
}

// FindLinked is find_linked, detached links included.
var FindLinked = &Definition[api.FindLinkedInput, []api.LinkedRow]{
	Name: "find_linked", Audience: IntegrationOnly,
	HTTP: "GET /api/p/{key}/integration/linked",
	Doc:  "Find the Specs and Tickets an Integration has linked, detached ones too.",
}

// Unlink is the human's unlink.
var Unlink = &Definition[api.UnlinkInput, api.LinkedRow]{
	Name: "unlink", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/unlink",
	Doc:  "Detach a linked Spec or Ticket from its external item.",
}

// GetIssueWatch is the Issue Watcher's read.
var GetIssueWatch = &Definition[api.WatchInput, api.IssueWatch]{
	Name: "get_issue_watch", Audience: IntegrationOnly,
	HTTP: "GET /api/p/{key}/integration/issue-watch",
	Doc:  "Get what the Issue Watcher polls for.",
}

// ReportIssueWatcherStatus is the Issue Watcher's status report.
var ReportIssueWatcherStatus = &Definition[api.ReportWatcherStatusInput, api.Written[api.ProjectView]]{
	Name: "report_issue_watcher_status", Audience: IntegrationOnly, Write: true,
	HTTP: "POST /api/p/{key}/integration/report_issue_watcher_status",
	Doc:  "Report the Issue Watcher's state.",
}

// ReportIssueWritten is the Watchers' report of a comment written to an
// external item.
var ReportIssueWritten = &Definition[api.ReportIssueWrittenInput, api.LinkedRow]{
	Name: "report_issue_written", Audience: IntegrationOnly, Write: true,
	HTTP: "POST /api/p/{key}/integration/report_issue_written",
	Doc:  "Report that a comment was written to a linked item's external issue, so it is never due again.",
}

// GetWorkItemWatch is the Work Item Watcher's read.
var GetWorkItemWatch = &Definition[api.WatchInput, api.WorkItemWatch]{
	Name: "get_work_item_watch", Audience: IntegrationOnly,
	HTTP: "GET /api/p/{key}/integration/work-item-watch",
	Doc:  "Get what the Work Item Watcher polls for.",
}

// ReportWorkItemWatcherStatus is the Work Item Watcher's status report.
var ReportWorkItemWatcherStatus = &Definition[api.ReportWatcherStatusInput, api.Written[api.ProjectView]]{
	Name: "report_work_item_watcher_status", Audience: IntegrationOnly, Write: true,
	HTTP: "POST /api/p/{key}/integration/report_work_item_watcher_status",
	Doc:  "Report the Work Item Watcher's state.",
}
