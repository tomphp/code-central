package ops

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// CI Tracking (Spec code-central-247), the CI Watcher's and the Pull
// Request Watcher's own, and CI failed's failure options.

// WatchRead is the input of a Watcher's read: it takes only the Project.
type WatchRead struct{}

// SetCITracking is set_ci_tracking.
var SetCITracking = &Definition[core.SetCITrackingInput, core.Written[core.ProjectView]]{
	Name: "set_ci_tracking", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/set_ci_tracking",
	Doc:  "Set whether the Project tracks CI, and the Home Repository's CI entry.",
	Call: (*core.Core).SetCITracking,
}

// SetRepositoryCI is set_repository_ci.
var SetRepositoryCI = &Definition[core.SetRepositoryCIInput, core.Written[core.ProjectView]]{
	Name: "set_repository_ci", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/set_repository_ci",
	Doc:  "Set one Repository's CI entry.",
	Call: (*core.Core).SetRepositoryCI,
}

// ReportPushRemote is report_push_remote.
var ReportPushRemote = &Definition[core.ReportPushRemoteInput, core.Written[core.ProjectView]]{
	Name: "report_push_remote", Audience: FactoryOnly, Write: true,
	HTTP: "POST /api/p/{key}/report_push_remote",
	Doc:  "The Factory's report of its Push remote.",
	Call: (*core.Core).ReportPushRemote,
}

// AddCIRule is add_ci_rule.
var AddCIRule = &Definition[core.CIRuleInput, core.Written[core.ProjectView]]{
	Name: "add_ci_rule", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/add_ci_rule",
	Doc:  "Add a CI Rule: when the workflow fails on the branch, open a Ticket to fix it.",
	Call: (*core.Core).AddCIRule,
}

// EditCIRule is edit_ci_rule.
var EditCIRule = &Definition[core.CIRuleInput, core.Written[core.ProjectView]]{
	Name: "edit_ci_rule", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/edit_ci_rule",
	Doc:  "Edit a CI Rule.",
	Call: (*core.Core).EditCIRule,
}

// RemoveCIRule is remove_ci_rule.
var RemoveCIRule = &Definition[core.RemoveCIRuleInput, core.Written[core.ProjectView]]{
	Name: "remove_ci_rule", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/remove_ci_rule",
	Doc:  "Remove a CI Rule.",
	Call: (*core.Core).RemoveCIRule,
}

// GetCIWatch is get_ci_watch.
var GetCIWatch = &Definition[WatchRead, core.CIWatch]{
	Name: "get_ci_watch", Audience: FactoryOnly,
	HTTP: "GET /api/p/{key}/ci-watch",
	Doc:  "What the CI Watcher polls: the CI settings, Rules and the pushed commits still waiting.",
	Call: func(c *core.Core, ctx context.Context, key string, _ WatchRead) (core.CIWatch, error) {
		return c.GetCIWatch(ctx, key)
	},
}

// ReportCIWatcherStatus is report_ci_watcher_status.
var ReportCIWatcherStatus = &Definition[core.ReportWatcherStatusInput, core.Written[core.ProjectView]]{
	Name: "report_ci_watcher_status", Audience: FactoryOnly, Write: true,
	HTTP: "POST /api/p/{key}/report_ci_watcher_status",
	Doc:  "The CI Watcher's report of its status.",
	Call: (*core.Core).ReportCIWatcherStatus,
}

// ReportCIRuns is report_ci_runs.
var ReportCIRuns = &Definition[core.ReportCIRunsInput, core.Written[core.CIRunsView]]{
	Name: "report_ci_runs", Audience: FactoryOnly, Write: true,
	HTTP: "POST /api/p/{key}/report_ci_runs",
	Doc:  "The CI Watcher's report of CI Runs.",
	Call: (*core.Core).ReportCIRuns,
}

// ReportPushedCommit is report_pushed_commit.
var ReportPushedCommit = &Definition[core.ReportPushedCommitInput, core.Written[core.TicketView]]{
	Name: "report_pushed_commit", Audience: FactoryOnly, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/report_pushed_commit",
	Doc:  "The Factory's report of the commit it pushed for a Ticket.",
	Call: (*core.Core).ReportPushedCommit,
}

// ReportSpecMerge is report_spec_merge.
var ReportSpecMerge = &Definition[core.ReportSpecMergeInput, core.Written[core.TicketView]]{
	Name: "report_spec_merge", Audience: FactoryOnly, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/report_spec_merge",
	Doc:  "The Factory's report of the Spec Branch commit it merged a Ticket's work into.",
	Call: (*core.Core).ReportSpecMerge,
}

// LandAnyway is land_anyway.
var LandAnyway = &Definition[core.CIResponseInput, core.Written[core.TicketView]]{
	Name: "land_anyway", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/land_anyway",
	Doc:  "Land a Ticket whose CI failed anyway.",
	Call: (*core.Core).LandAnyway,
}

// RerunCI is rerun_ci.
var RerunCI = &Definition[core.CIResponseInput, core.Written[core.TicketView]]{
	Name: "rerun_ci", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/rerun_ci",
	Doc:  "Rerun a Ticket's failed CI.",
	Call: (*core.Core).RerunCI,
}

// NewFixTicket is new_fix_ticket.
var NewFixTicket = &Definition[core.NewFixTicketInput, core.Written[core.TicketView]]{
	Name: "new_fix_ticket", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/new_fix_ticket",
	Doc:  "Open a Ticket to fix a Ticket's failed CI.",
	Call: (*core.Core).NewFixTicket,
}

// The Pull Request Watcher's own (code-central-388).

// GetPullRequestWatch is get_pull_request_watch.
var GetPullRequestWatch = &Definition[WatchRead, core.PullRequestWatch]{
	Name: "get_pull_request_watch", Audience: FactoryOnly,
	HTTP: "GET /api/p/{key}/pull-request-watch",
	Doc:  "What the Pull Request Watcher polls.",
	Call: func(c *core.Core, ctx context.Context, key string, _ WatchRead) (core.PullRequestWatch, error) {
		return c.GetPullRequestWatch(ctx, key)
	},
}

// ReportPullRequest is report_pull_request.
var ReportPullRequest = &Definition[core.ReportPullRequestInput, core.Written[core.PullRequestReport]]{
	Name: "report_pull_request", Audience: FactoryOnly, Write: true,
	HTTP: "POST /api/p/{key}/report_pull_request",
	Doc:  "The Pull Request Watcher's report of a pull request.",
	Call: (*core.Core).ReportPullRequest,
}

// ReportPullRequestWatcherStatus is report_pull_request_watcher_status.
var ReportPullRequestWatcherStatus = &Definition[core.ReportWatcherStatusInput, core.Written[core.ProjectView]]{
	Name: "report_pull_request_watcher_status", Audience: FactoryOnly, Write: true,
	HTTP: "POST /api/p/{key}/report_pull_request_watcher_status",
	Doc:  "The Pull Request Watcher's report of its status.",
	Call: (*core.Core).ReportPullRequestWatcherStatus,
}
