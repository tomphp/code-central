package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// GitHub Issues (Spec code-central-279): the human's GitHub Issues
// settings and triage actions.
func init() {
	Routes["set_github_issues"] = "POST /api/p/{key}/set_github_issues"
	Routes["keep_as_draft"] = "POST /api/p/{key}/tickets/{n}/keep_as_draft"
	Routes["to_requirement"] = "POST /api/p/{key}/tickets/{n}/to_requirement"
	Routes["wont_fix"] = "POST /api/p/{key}/tickets/{n}/wont_fix"

	handlers["set_github_issues"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.SetGitHubIssuesInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.SetGitHubIssues(r.Context(), r.PathValue("key"), in)
	}
	handlers["keep_as_draft"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.TicketInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		in.Ticket = core.Ref(r.PathValue("n"))
		return c.KeepAsDraft(r.Context(), r.PathValue("key"), in)
	}
	handlers["to_requirement"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.ToRequirementInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		in.Ticket = core.Ref(r.PathValue("n"))
		return c.ToRequirement(r.Context(), r.PathValue("key"), in)
	}
	handlers["wont_fix"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.WontFixInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		in.Ticket = core.Ref(r.PathValue("n"))
		return c.WontFix(r.Context(), r.PathValue("key"), in)
	}
}

// The Issue Watcher's own (code-central-285): the read it polls, its
// status, and each comment it writes to an issue.
func init() {
	Routes["get_issue_watch"] = "GET /api/p/{key}/integration/issue-watch"
	Routes["report_issue_watcher_status"] = "POST /api/p/{key}/integration/report_issue_watcher_status"
	Routes["report_issue_written"] = "POST /api/p/{key}/integration/report_issue_written"

	handlers["get_issue_watch"] = func(c *core.Core, r *http.Request) (any, error) {
		return c.GetIssueWatch(r.Context(), r.PathValue("key"))
	}
	handlers["report_issue_watcher_status"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.ReportWatcherStatusInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.ReportIssueWatcherStatus(r.Context(), r.PathValue("key"), in)
	}
	handlers["report_issue_written"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.ReportIssueWrittenInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.ReportIssueWritten(r.Context(), r.PathValue("key"), in)
	}
}
