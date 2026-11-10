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
}
