package mcp

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// Submitting a Map.
func init() {
	tools["submit_map"] = tool{
		description: "Submit a Map: its text becomes a Spec labelled wayfinder:map (titled with the first line, with the Map's empty sections) and one Ready Charting Ticket for a Factory to work; returns their acknowledgements, or the Spec and Ticket with full.",
		schema: object(map[string]any{
			"text": text("The Map's text, in Markdown; its first line becomes the Spec's title."),
			"full": flag("Return the whole Spec and Charting Ticket instead of their acknowledgements."),
		}, "text"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.SubmitMapInput) (any, error) {
			return c.SubmitMap(ctx, key, in)
		}),
	}
}
