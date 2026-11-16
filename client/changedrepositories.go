package client

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// A Ticket's Changed Repositories (Spec code-central-848).

// ReportChangedRepositories is the Factory's report_changed_repositories.
func (c *Client) ReportChangedRepositories(ctx context.Context, key string, in core.ReportChangedRepositoriesInput) (core.Written[core.TicketView], error) {
	return write[core.TicketView](ctx, c, item(key, "tickets", in.Ticket, "report_changed_repositories"), in, in.Full)
}

// FailFinish is the Factory's fail_finish.
func (c *Client) FailFinish(ctx context.Context, key string, in core.FailFinishInput) (core.Written[core.TicketView], error) {
	return write[core.TicketView](ctx, c, item(key, "tickets", in.Ticket, "fail_finish"), in, in.Full)
}
