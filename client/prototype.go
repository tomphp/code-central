package client

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// SubmitPrototype is submit_prototype.
func (c *Client) SubmitPrototype(ctx context.Context, key string, in core.SubmitPrototypeInput) (core.Written[core.TicketView], error) {
	return write[core.TicketView](ctx, c, scoped(key, "submit_prototype"), in, in.Full)
}
