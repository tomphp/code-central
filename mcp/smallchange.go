package mcp

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// Submitting a Small Change.
func init() {
	tools["submit_small_change"] = tool{
		description: "Submit a Small Change: a Ready implementation Ticket on the Project's Small Changes Spec, titled with the text's first line and carrying the text as its description, which a Factory builds and lands on its own; returns its acknowledgement, or the Ticket with full.",
		schema: object(map[string]any{
			"text":  text("The change to make, in Markdown; its first line becomes the Ticket's title."),
			"draft": flag("Save the Ticket as Draft instead of Ready, so nothing is claimed until the human moves it to Ready."),
			"full":  flag("Return the whole Ticket instead of the acknowledgement."),
		}, "text"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.SubmitSmallChangeInput) (any, error) {
			return c.SubmitSmallChange(ctx, key, in)
		}),
	}
}
