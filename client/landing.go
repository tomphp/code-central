package client

import (
	"context"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ops"
)

// Landing and Repositories (Spec code-central-848). The Repository writes
// are the human's: set Actor to human.

// Land is land.
func (c *Client) Land(ctx context.Context, key string, in core.LandInput) (core.Written[core.TicketView], error) {
	return written(ctx, c, ops.Land, key, in, in.Full)
}

// AddRepository is the human's add_repository.
func (c *Client) AddRepository(ctx context.Context, key string, in core.AddRepositoryInput) (core.Written[core.ProjectView], error) {
	return written(ctx, c, ops.AddRepository, key, in, in.Full)
}

// EditRepository is the human's edit_repository.
func (c *Client) EditRepository(ctx context.Context, key string, in core.EditRepositoryInput) (core.Written[core.ProjectView], error) {
	return written(ctx, c, ops.EditRepository, key, in, in.Full)
}

// RemoveRepository is the human's remove_repository.
func (c *Client) RemoveRepository(ctx context.Context, key string, in core.RemoveRepositoryInput) (core.Written[core.ProjectView], error) {
	return written(ctx, c, ops.RemoveRepository, key, in, in.Full)
}

// SetLandingOrder is the human's set_landing_order.
func (c *Client) SetLandingOrder(ctx context.Context, key string, in core.SetLandingOrderInput) (core.Written[core.ProjectView], error) {
	return written(ctx, c, ops.SetLandingOrder, key, in, in.Full)
}

// ReportRepositories is the Factory's report_repositories: each
// Repository's clone, ready or missing.
func (c *Client) ReportRepositories(ctx context.Context, key string, in core.ReportRepositoriesInput) (core.Written[core.ProjectView], error) {
	return written(ctx, c, ops.ReportRepositories, key, in, in.Full)
}

// ReportChangedRepositories is the Factory's report_changed_repositories.
func (c *Client) ReportChangedRepositories(ctx context.Context, key string, in core.ReportChangedRepositoriesInput) (core.Written[core.TicketView], error) {
	return written(ctx, c, ops.ReportChangedRepositories, key, in, in.Full)
}

// FailFinish is the Factory's fail_finish.
func (c *Client) FailFinish(ctx context.Context, key string, in core.FailFinishInput) (core.Written[core.TicketView], error) {
	return written(ctx, c, ops.FailFinish, key, in, in.Full)
}

// SetLandingMethod is the human's set_landing_method.
func (c *Client) SetLandingMethod(ctx context.Context, key string, in core.SetLandingMethodInput) (core.ProjectView, error) {
	return call(ctx, c, ops.SetLandingMethod, key, in)
}
