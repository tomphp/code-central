package client

import (
	"context"
	"net/url"

	"github.com/tomoram/code-central/core"
)

// The Integration contract (ticket 42). A Go Integration sets Actor to
// integration; Unlink is the human's.

// CreateLinkedSpec is the integration's create_linked_spec.
func (c *Client) CreateLinkedSpec(ctx context.Context, key string, in core.CreateLinkedSpecInput) (core.Written[core.SpecView], error) {
	return write[core.SpecView](ctx, c, scoped(key, "integration/create_linked_spec"), in, in.Full)
}

// CreateLinkedTicket is the integration's create_linked_ticket.
func (c *Client) CreateLinkedTicket(ctx context.Context, key string, in core.CreateLinkedTicketInput) (core.Written[core.TicketView], error) {
	return write[core.TicketView](ctx, c, scoped(key, "integration/create_linked_ticket"), in, in.Full)
}

// UpdateLinkedSpec is the integration's update_linked_spec.
func (c *Client) UpdateLinkedSpec(ctx context.Context, key string, in core.UpdateLinkedSpecInput) (core.Written[core.SpecView], error) {
	return write[core.SpecView](ctx, c, item(key, "integration/specs", in.Spec, "update_linked_spec"), in, in.Full)
}

// UpdateLinkedTicket is the integration's update_linked_ticket.
func (c *Client) UpdateLinkedTicket(ctx context.Context, key string, in core.UpdateLinkedTicketInput) (core.Written[core.TicketView], error) {
	return write[core.TicketView](ctx, c, item(key, "integration/tickets", in.Ticket, "update_linked_ticket"), in, in.Full)
}

// SetExternalState is the integration's set_external_state.
func (c *Client) SetExternalState(ctx context.Context, key string, in core.SetExternalStateInput) (core.LinkedRow, error) {
	var out core.LinkedRow
	return out, c.do(ctx, "POST", scoped(key, "integration/set_external_state"), in, &out)
}

// FindLinked is find_linked(integration, id?), detached links included.
func (c *Client) FindLinked(ctx context.Context, key string, in core.FindLinkedInput) ([]core.LinkedRow, error) {
	q := url.Values{"integration": {in.Integration}}
	if in.ID != "" {
		q.Set("id", in.ID)
	}
	var out []core.LinkedRow
	return out, c.do(ctx, "GET", scoped(key, "integration/linked?"+q.Encode()), nil, &out)
}

// Unlink is the human's unlink; set Actor to human.
func (c *Client) Unlink(ctx context.Context, key string, in core.UnlinkInput) (core.LinkedRow, error) {
	var out core.LinkedRow
	return out, c.do(ctx, "POST", scoped(key, "unlink"), in, &out)
}
