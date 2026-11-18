package client

import (
	"context"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ops"
)

// SubmitRetro is submit_retro.
func (c *Client) SubmitRetro(ctx context.Context, key string, in core.SubmitRetroInput) (core.Written[core.TicketView], error) {
	return written(ctx, c, ops.SubmitRetro, key, in, in.Full)
}
