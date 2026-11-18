package client

import (
	"context"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ops"
)

// Blockers, Replacements and finding work (ticket 28).

// AddBlocker is add_blocker.
func (c *Client) AddBlocker(ctx context.Context, key string, in core.BlockerInput) (core.Written[core.TicketView], error) {
	return write[core.TicketView](ctx, c, item(key, "tickets", in.Ticket, "add_blocker"), in, in.Full)
}

// RemoveBlocker is remove_blocker.
func (c *Client) RemoveBlocker(ctx context.Context, key string, in core.BlockerInput) (core.Written[core.TicketView], error) {
	return write[core.TicketView](ctx, c, item(key, "tickets", in.Ticket, "remove_blocker"), in, in.Full)
}

// Cancel is cancel.
func (c *Client) Cancel(ctx context.Context, key string, in core.CancelInput) (core.Written[core.TicketView], error) {
	return write[core.TicketView](ctx, c, item(key, "tickets", in.Ticket, "cancel"), in, in.Full)
}

// GetSpec is get_spec.
func (c *Client) GetSpec(ctx context.Context, key string, in core.GetSpecInput) (core.SpecDetail, error) {
	return call(ctx, c, ops.GetSpec, key, in)
}

// FindSpecs is find_specs.
func (c *Client) FindSpecs(ctx context.Context, key string, in core.FindSpecsInput) ([]core.SpecRow, error) {
	return call(ctx, c, ops.FindSpecs, key, in)
}
