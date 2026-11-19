package client

import (
	"context"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/ops"
)

// SubmitSmallChange is submit_small_change.
func (c *Client) SubmitSmallChange(ctx context.Context, key string, in api.SubmitSmallChangeInput) (api.Written[api.TicketView], error) {
	return written(ctx, c, ops.SubmitSmallChange, key, in, in.Full)
}
