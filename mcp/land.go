package mcp

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// Landing Accepted work (ADR 0006).
func init() {
	tools["land"] = tool{
		description: "Land a Ticket: once its work is merged into the base, move it from Accepted (or In Review) to Done, recording the commit that put it there.",
		schema: object(map[string]any{
			"ticket": ref("The Ticket."),
			"commit": text("The commit that put the work into the base, such as the merge commit."),
		}, "ticket", "commit"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.LandInput) (any, error) {
			return c.Land(ctx, key, in)
		}),
	}
}
