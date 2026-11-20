package ops

import (
	"github.com/tomoram/code-central/api"
)

// CI Tracking (Spec code-central-247), the CI Watcher's and the Pull
// Request Watcher's own, and CI failed's failure options.

// WatchRead is the input of a Watcher's read: it takes only the Project.
type WatchRead struct{}

// SetCITracking is set_ci_tracking.
var SetCITracking = &Definition[api.SetCITrackingInput, api.Written[api.ProjectView]]{
	Name: "set_ci_tracking", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/set_ci_tracking",
	Doc:  "Set whether the Project tracks CI, and the Home Repository's CI entry.",
}

// SetRepositoryCI is set_repository_ci.
var SetRepositoryCI = &Definition[api.SetRepositoryCIInput, api.Written[api.ProjectView]]{
	Name: "set_repository_ci", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/set_repository_ci",
	Doc:  "Set one Repository's CI entry.",
}

// ReportPushRemote is report_push_remote.
var ReportPushRemote = &Definition[api.ReportPushRemoteInput, api.Written[api.ProjectView]]{
	Name: "report_push_remote", Audience: FactoryOnly, Write: true,
	HTTP: "POST /api/p/{key}/report_push_remote",
	Doc:  "The Factory's report of its Push remote.",
}

// AddCIRule is add_ci_rule.
var AddCIRule = &Definition[api.CIRuleInput, api.Written[api.ProjectView]]{
	Name: "add_ci_rule", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/add_ci_rule",
	Doc:  "Add a CI Rule: when the workflow fails on the branch, open a Ticket to fix it.",
}

// EditCIRule is edit_ci_rule.
var EditCIRule = &Definition[api.CIRuleInput, api.Written[api.ProjectView]]{
	Name: "edit_ci_rule", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/edit_ci_rule",
	Doc:  "Edit a CI Rule.",
}

// RemoveCIRule is remove_ci_rule.
var RemoveCIRule = &Definition[api.RemoveCIRuleInput, api.Written[api.ProjectView]]{
	Name: "remove_ci_rule", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/remove_ci_rule",
	Doc:  "Remove a CI Rule.",
}

// GetCIWatch is get_ci_watch.
var GetCIWatch = &Definition[WatchRead, api.CIWatch]{
	Name: "get_ci_watch", Audience: FactoryOnly,
	HTTP: "GET /api/p/{key}/ci-watch",
	Doc:  "What the CI Watcher polls: the CI settings, Rules and the pushed commits still waiting.",
}

// ReportCIWatcherStatus is report_ci_watcher_status.
var ReportCIWatcherStatus = &Definition[api.ReportWatcherStatusInput, api.Written[api.ProjectView]]{
	Name: "report_ci_watcher_status", Audience: FactoryOnly, Write: true,
	HTTP: "POST /api/p/{key}/report_ci_watcher_status",
	Doc:  "The CI Watcher's report of its status.",
}

// ReportCIRuns is report_ci_runs.
var ReportCIRuns = &Definition[api.ReportCIRunsInput, api.Written[api.CIRunsView]]{
	Name: "report_ci_runs", Audience: FactoryOnly, Write: true,
	HTTP: "POST /api/p/{key}/report_ci_runs",
	Doc:  "The CI Watcher's report of CI Runs.",
}

// ReportPushedCommit is report_pushed_commit.
var ReportPushedCommit = &Definition[api.ReportPushedCommitInput, api.Written[api.TicketView]]{
	Name: "report_pushed_commit", Audience: FactoryOnly, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/report_pushed_commit",
	Doc:  "The Factory's report of the commit it pushed for a Ticket.",
}

// ReportSpecMerge is report_spec_merge.
var ReportSpecMerge = &Definition[api.ReportSpecMergeInput, api.Written[api.TicketView]]{
	Name: "report_spec_merge", Audience: FactoryOnly, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/report_spec_merge",
	Doc:  "The Factory's report of the Spec Branch commit it merged a Ticket's work into.",
}

// LandAnyway is land_anyway.
var LandAnyway = &Definition[api.CIResponseInput, api.Written[api.TicketView]]{
	Name: "land_anyway", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/land_anyway",
	Doc:  "Land a Ticket whose CI failed anyway.",
}

// RerunCI is rerun_ci.
var RerunCI = &Definition[api.CIResponseInput, api.Written[api.TicketView]]{
	Name: "rerun_ci", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/rerun_ci",
	Doc:  "Rerun a Ticket's failed CI.",
}

// NewFixTicket is new_fix_ticket.
var NewFixTicket = &Definition[api.NewFixTicketInput, api.Written[api.TicketView]]{
	Name: "new_fix_ticket", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/new_fix_ticket",
	Doc:  "Open a Ticket to fix a Ticket's failed CI.",
}

// The Pull Request Watcher's own (code-central-388).

// GetPullRequestWatch is get_pull_request_watch.
var GetPullRequestWatch = &Definition[WatchRead, api.PullRequestWatch]{
	Name: "get_pull_request_watch", Audience: FactoryOnly,
	HTTP: "GET /api/p/{key}/pull-request-watch",
	Doc:  "What the Pull Request Watcher polls.",
}

// ReportPullRequest is report_pull_request.
var ReportPullRequest = &Definition[api.ReportPullRequestInput, api.Written[api.PullRequestReport]]{
	Name: "report_pull_request", Audience: FactoryOnly, Write: true,
	HTTP: "POST /api/p/{key}/report_pull_request",
	Doc:  "The Pull Request Watcher's report of a pull request.",
}

// ReportPullRequestWatcherStatus is report_pull_request_watcher_status.
var ReportPullRequestWatcherStatus = &Definition[api.ReportWatcherStatusInput, api.Written[api.ProjectView]]{
	Name: "report_pull_request_watcher_status", Audience: FactoryOnly, Write: true,
	HTTP: "POST /api/p/{key}/report_pull_request_watcher_status",
	Doc:  "The Pull Request Watcher's report of its status.",
}

// SetAutoFix is set_auto_fix (Spec code-central-1022): the Project's own
// setting and the human's alone, over HTTP only.
var SetAutoFix = &Definition[api.SetAutoFixInput, api.Written[api.ProjectView]]{
	Name: "set_auto_fix", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/set_auto_fix",
	Doc:  "Set whether the Project opens fix Tickets for CI failures itself, with its Fix instructions and limit.",
}
