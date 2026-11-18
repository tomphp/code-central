package client

import (
	"context"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ops"
)

// The Integration contract (ticket 42). A Go Integration sets Actor to
// integration; Unlink is the human's.

// CreateLinkedSpec is the integration's create_linked_spec.
func (c *Client) CreateLinkedSpec(ctx context.Context, key string, in core.CreateLinkedSpecInput) (core.Written[core.SpecView], error) {
	return written(ctx, c, ops.CreateLinkedSpec, key, in, in.Full)
}

// CreateLinkedTicket is the integration's create_linked_ticket.
func (c *Client) CreateLinkedTicket(ctx context.Context, key string, in core.CreateLinkedTicketInput) (core.Written[core.TicketView], error) {
	return written(ctx, c, ops.CreateLinkedTicket, key, in, in.Full)
}

// UpdateLinkedSpec is the integration's update_linked_spec.
func (c *Client) UpdateLinkedSpec(ctx context.Context, key string, in core.UpdateLinkedSpecInput) (core.Written[core.SpecView], error) {
	return written(ctx, c, ops.UpdateLinkedSpec, key, in, in.Full)
}

// UpdateLinkedTicket is the integration's update_linked_ticket.
func (c *Client) UpdateLinkedTicket(ctx context.Context, key string, in core.UpdateLinkedTicketInput) (core.Written[core.TicketView], error) {
	return written(ctx, c, ops.UpdateLinkedTicket, key, in, in.Full)
}

// SetExternalState is the integration's set_external_state.
func (c *Client) SetExternalState(ctx context.Context, key string, in core.SetExternalStateInput) (core.LinkedRow, error) {
	return call(ctx, c, ops.SetExternalState, key, in)
}

// FindLinked is find_linked(integration, id?), detached links included.
func (c *Client) FindLinked(ctx context.Context, key string, in core.FindLinkedInput) ([]core.LinkedRow, error) {
	return call(ctx, c, ops.FindLinked, key, in)
}

// Unlink is the human's unlink; set Actor to human.
func (c *Client) Unlink(ctx context.Context, key string, in core.UnlinkInput) (core.LinkedRow, error) {
	return call(ctx, c, ops.Unlink, key, in)
}
