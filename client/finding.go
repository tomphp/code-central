package client

import (
	"context"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/ops"
)

// Blockers, Replacements and finding work (ticket 28).

// AddBlocker is add_blocker.
func (c *Client) AddBlocker(ctx context.Context, key string, in api.BlockerInput) (api.Written[api.TicketView], error) {
	return written(ctx, c, ops.AddBlocker, key, in, in.Full)
}

// RemoveBlocker is remove_blocker.
func (c *Client) RemoveBlocker(ctx context.Context, key string, in api.BlockerInput) (api.Written[api.TicketView], error) {
	return written(ctx, c, ops.RemoveBlocker, key, in, in.Full)
}

// Cancel is cancel.
func (c *Client) Cancel(ctx context.Context, key string, in api.CancelInput) (api.Written[api.TicketView], error) {
	return written(ctx, c, ops.Cancel, key, in, in.Full)
}

// GetSpec is get_spec.
func (c *Client) GetSpec(ctx context.Context, key string, in api.GetSpecInput) (api.SpecDetail, error) {
	return call(ctx, c, ops.GetSpec, key, in)
}

// FindSpecs is find_specs.
func (c *Client) FindSpecs(ctx context.Context, key string, in api.FindSpecsInput) ([]api.SpecRow, error) {
	return call(ctx, c, ops.FindSpecs, key, in)
}
