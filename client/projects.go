package client

import (
	"context"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/ops"
)

// Options (Spec code-central-160).

// GetOptions is get_options.
func (c *Client) GetOptions(ctx context.Context) (api.OptionsView, error) {
	return call(ctx, c, ops.GetOptions, "", struct{}{})
}

// SetOptions is the human's set_options; set Actor to human.
func (c *Client) SetOptions(ctx context.Context, in api.SetOptionsInput) (api.OptionsView, error) {
	return call(ctx, c, ops.SetOptions, "", in)
}

// SetProjectMode is set_project_mode.
func (c *Client) SetProjectMode(ctx context.Context, key string, in api.SetProjectModeInput) (api.ProjectView, error) {
	return call(ctx, c, ops.SetProjectMode, key, in)
}

// Needs Me, moving by hand and Projects (ticket 32). Each is the
// human's: set Actor to human for the writes.

// NeedsMe is needs_me: what waits on the human across every Project.
func (c *Client) NeedsMe(ctx context.Context) (api.NeedsMeView, error) {
	return call(ctx, c, ops.NeedsMe, "", struct{}{})
}

// ListProjects is list_projects.
func (c *Client) ListProjects(ctx context.Context) ([]api.ProjectRow, error) {
	return call(ctx, c, ops.ListProjects, "", struct{}{})
}

// Move is the human's move(to_state).
func (c *Client) Move(ctx context.Context, key string, in api.MoveInput) (api.Written[api.TicketView], error) {
	return written(ctx, c, ops.Move, key, in, in.Full)
}

// RenameProject is the human's rename_project.
func (c *Client) RenameProject(ctx context.Context, key string, in api.RenameProjectInput) (api.ProjectView, error) {
	return call(ctx, c, ops.RenameProject, key, in)
}
