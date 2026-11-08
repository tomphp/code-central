package mcp

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// Submitting a Requirement.
func init() {
	tools["submit_requirement"] = tool{
		description: "Submit a Requirement: its text becomes a Spec (titled with the first line) with three Ready Stage Tickets, grilling, then spec, then tickets, for a Factory to work.",
		schema: object(map[string]any{
			"text": text("The Requirement's text, in Markdown; its first line becomes the Spec's title."),
		}, "text"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.SubmitRequirementInput) (any, error) {
			return c.SubmitRequirement(ctx, key, in)
		}),
	}
}
