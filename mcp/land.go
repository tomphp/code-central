package mcp

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// Landing Accepted work (ADR 0006).
func init() {
	tools["land"] = tool{
		description: "Land a Ticket: once its work is merged into the base of one of its Changed Repositories, record the commit that put it there; once it has landed in every one, it moves from Accepted (or In Review) to Done. Returns an acknowledgement with the Ticket's State, or the Ticket with full.",
		schema: object(map[string]any{
			"ticket":     ref("The Ticket."),
			"commit":     text("The commit that put the work into the base, such as the merge commit."),
			"repository": text("The Repository Name whose base it landed in; the Home Repository when left out."),
			"full":       flag("Return the whole Ticket instead of the acknowledgement."),
		}, "ticket", "commit"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.LandInput) (any, error) {
			return c.Land(ctx, key, in)
		}),
	}
}
