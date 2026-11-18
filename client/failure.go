package client

import (
	"context"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ops"
)

// Failure Reports and giving up (ticket 30).

// ReportFailure is report_failure.
func (c *Client) ReportFailure(ctx context.Context, key string, in core.ReportFailureInput) (core.StopResult, error) {
	return call(ctx, c, ops.ReportFailure, key, in)
}

// GiveUp is give_up.
func (c *Client) GiveUp(ctx context.Context, key string, in core.GiveUpInput) (core.StopResult, error) {
	return call(ctx, c, ops.GiveUp, key, in)
}

// SendBack is send_back: the agent's from Accepted, or, with Actor set to
// human, the human's from Failed.
func (c *Client) SendBack(ctx context.Context, key string, in core.SendBackInput) (core.Written[core.TicketView], error) {
	return written(ctx, c, ops.SendBack, key, in, in.Full)
}

// ToDraft is the human's to_draft; set Actor to human.
func (c *Client) ToDraft(ctx context.Context, key string, in core.TicketInput) (core.Written[core.TicketView], error) {
	return written(ctx, c, ops.ToDraft, key, in, in.Full)
}
