package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// GitHub Issues (Spec code-central-279): the human's GitHub Issues
// settings.
func init() {
	Routes["set_github_issues"] = "POST /api/p/{key}/set_github_issues"

	handlers["set_github_issues"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.SetGitHubIssuesInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.SetGitHubIssues(r.Context(), r.PathValue("key"), in)
	}
}
