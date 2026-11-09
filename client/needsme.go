package client

import (
	"context"
	"net/url"

	"github.com/tomoram/code-central/core"
)

// Needs Me, moving by hand and Projects (ticket 32). Each is the
// human's: set Actor to human for the writes.

// NeedsMe is needs_me: what waits on the human across every Project.
func (c *Client) NeedsMe(ctx context.Context) (core.NeedsMeView, error) {
	var out core.NeedsMeView
	return out, c.do(ctx, "GET", "/api/needs-me", nil, &out)
}

// ListProjects is list_projects.
func (c *Client) ListProjects(ctx context.Context) ([]core.ProjectRow, error) {
	var out []core.ProjectRow
	return out, c.do(ctx, "GET", "/api/projects", nil, &out)
}

// Move is the human's move(to_state).
func (c *Client) Move(ctx context.Context, key string, in core.MoveInput) (core.Written[core.TicketView], error) {
	return write[core.TicketView](ctx, c, item(key, "tickets", in.Ticket, "move"), in, in.Full)
}

// RenameProject is the human's rename_project.
func (c *Client) RenameProject(ctx context.Context, key string, in core.RenameProjectInput) (core.ProjectView, error) {
	var out core.ProjectView
	return out, c.do(ctx, "POST", "/api/p/"+url.PathEscape(key)+"/rename_project", in, &out)
}

// SetLandingMethod is the human's set_landing_method.
func (c *Client) SetLandingMethod(ctx context.Context, key string, in core.SetLandingMethodInput) (core.ProjectView, error) {
	var out core.ProjectView
	return out, c.do(ctx, "POST", "/api/p/"+url.PathEscape(key)+"/set_landing_method", in, &out)
}
