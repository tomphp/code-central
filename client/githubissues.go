package client

import (
	"context"
	"net/url"

	"github.com/tomoram/code-central/core"
)

// GitHub Issues (Spec code-central-279).

// SetGitHubIssues is the human's set_github_issues; set Actor to human.
func (c *Client) SetGitHubIssues(ctx context.Context, key string, in core.SetGitHubIssuesInput) (core.Written[core.ProjectView], error) {
	return write[core.ProjectView](ctx, c, "/api/p/"+url.PathEscape(key)+"/set_github_issues", in, in.Full)
}
