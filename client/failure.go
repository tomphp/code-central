package client

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// Failure Reports and giving up (ticket 30).

// ReportFailure is report_failure.
func (c *Client) ReportFailure(ctx context.Context, key string, in core.ReportFailureInput) (core.StopResult, error) {
	var out core.StopResult
	return out, c.do(ctx, "POST", item(key, "tickets", in.Ticket, "report_failure"), in, &out)
}

// GiveUp is give_up.
func (c *Client) GiveUp(ctx context.Context, key string, in core.GiveUpInput) (core.StopResult, error) {
	var out core.StopResult
	return out, c.do(ctx, "POST", item(key, "tickets", in.Ticket, "give_up"), in, &out)
}

// SendBack is send_back: the agent's from Accepted, or, with Actor set to
// human, the human's from Failed.
func (c *Client) SendBack(ctx context.Context, key string, in core.SendBackInput) (core.TicketView, error) {
	var out core.TicketView
	return out, c.do(ctx, "POST", item(key, "tickets", in.Ticket, "send_back"), in, &out)
}

// ToDraft is the human's to_draft; set Actor to human.
func (c *Client) ToDraft(ctx context.Context, key string, in core.TicketInput) (core.TicketView, error) {
	var out core.TicketView
	return out, c.do(ctx, "POST", item(key, "tickets", in.Ticket, "to_draft"), in, &out)
}
