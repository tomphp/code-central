package client

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// SubmitRetro is submit_retro.
func (c *Client) SubmitRetro(ctx context.Context, key string, in core.SubmitRetroInput) (core.Written[core.TicketView], error) {
	return write[core.TicketView](ctx, c, scoped(key, "submit_retro"), in, in.Full)
}
