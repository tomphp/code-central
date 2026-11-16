package client

import (
	"context"
	"net/url"

	"github.com/tomoram/code-central/core"
)

// Repositories (Spec code-central-848).

// AddRepository is the human's add_repository; set Actor to human.
func (c *Client) AddRepository(ctx context.Context, key string, in core.AddRepositoryInput) (core.Written[core.ProjectView], error) {
	return write[core.ProjectView](ctx, c, "/api/p/"+url.PathEscape(key)+"/add_repository", in, in.Full)
}

// EditRepository is the human's edit_repository; set Actor to human.
func (c *Client) EditRepository(ctx context.Context, key string, in core.EditRepositoryInput) (core.Written[core.ProjectView], error) {
	return write[core.ProjectView](ctx, c, "/api/p/"+url.PathEscape(key)+"/edit_repository", in, in.Full)
}

// RemoveRepository is the human's remove_repository; set Actor to human.
func (c *Client) RemoveRepository(ctx context.Context, key string, in core.RemoveRepositoryInput) (core.Written[core.ProjectView], error) {
	return write[core.ProjectView](ctx, c, "/api/p/"+url.PathEscape(key)+"/remove_repository", in, in.Full)
}

// SetLandingOrder is the human's set_landing_order; set Actor to human.
func (c *Client) SetLandingOrder(ctx context.Context, key string, in core.SetLandingOrderInput) (core.Written[core.ProjectView], error) {
	return write[core.ProjectView](ctx, c, "/api/p/"+url.PathEscape(key)+"/set_landing_order", in, in.Full)
}
