package client

import (
	"context"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/ops"
)

// SubmitPrototype is submit_prototype.
func (c *Client) SubmitPrototype(ctx context.Context, key string, in api.SubmitPrototypeInput) (api.Written[api.TicketView], error) {
	return written(ctx, c, ops.SubmitPrototype, key, in, in.Full)
}
