package client

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// Land is land.
func (c *Client) Land(ctx context.Context, key string, in core.LandInput) (core.Written[core.TicketView], error) {
	return write[core.TicketView](ctx, c, item(key, "tickets", in.Ticket, "land"), in, in.Full)
}
