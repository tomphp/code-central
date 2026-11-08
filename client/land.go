package client

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// Land is land.
func (c *Client) Land(ctx context.Context, key string, in core.LandInput) (core.TicketView, error) {
	var out core.TicketView
	return out, c.do(ctx, "POST", item(key, "tickets", in.Ticket, "land"), in, &out)
}
