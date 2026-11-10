package client

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// The Model Choice (Spec code-central-301).

// GetModelChoice is the Factory's get_model_choice.
func (c *Client) GetModelChoice(ctx context.Context, key string, in core.GetModelChoiceInput) (core.ModelChoice, error) {
	var out core.ModelChoice
	return out, c.do(ctx, "GET", item(key, "tickets", in.Ticket, "model-choice"), nil, &out)
}

// ReportModelChoice is the Factory's report_model_choice.
func (c *Client) ReportModelChoice(ctx context.Context, key string, in core.ReportModelChoiceInput) (core.Written[core.TicketView], error) {
	return write[core.TicketView](ctx, c, item(key, "tickets", in.Ticket, "report_model_choice"), in, in.Full)
}
