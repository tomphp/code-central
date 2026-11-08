package mcp

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// Submitting a Requirement.
func init() {
	tools["submit_requirement"] = tool{
		description: "Submit a Requirement: its text becomes a Spec (titled with the first line) with three Ready Stage Tickets, grilling, then spec, then tickets, for a Factory to work; returns their acknowledgements, or the Spec and Tickets with full.",
		schema: object(map[string]any{
			"text": text("The Requirement's text, in Markdown; its first line becomes the Spec's title."),
			"full": flag("Return the whole Spec and Stage Tickets instead of their acknowledgements."),
		}, "text"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.SubmitRequirementInput) (any, error) {
			return c.SubmitRequirement(ctx, key, in)
		}),
	}
}
