package client

import (
	"context"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/ops"
)

// SubmitRetro is submit_retro.
func (c *Client) SubmitRetro(ctx context.Context, key string, in api.SubmitRetroInput) (api.Written[api.TicketView], error) {
	return written(ctx, c, ops.SubmitRetro, key, in, in.Full)
}
