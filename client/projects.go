package client

import (
	"context"
	"net/url"

	"github.com/tomoram/code-central/core"
)

// SetProjectMode is set_project_mode.
func (c *Client) SetProjectMode(ctx context.Context, key string, in core.SetProjectModeInput) (core.ProjectView, error) {
	var out core.ProjectView
	return out, c.do(ctx, "POST", "/api/projects/"+url.PathEscape(key)+"/set_project_mode", in, &out)
}
