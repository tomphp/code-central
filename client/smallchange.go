package client

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// SubmitSmallChange is submit_small_change.
func (c *Client) SubmitSmallChange(ctx context.Context, key string, in core.SubmitSmallChangeInput) (core.Written[core.TicketView], error) {
	return write[core.TicketView](ctx, c, scoped(key, "submit_small_change"), in, in.Full)
}
