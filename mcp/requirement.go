package mcp

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// Submitting a Requirement.
func init() {
	tools["submit_requirement"] = tool{
		description: "Submit a Requirement: its text becomes a Spec (titled with the first line) with three Ready Stage Tickets, grilling, then spec, then tickets, for a Factory to work; with map, a finished Map's Decisions replace the grilling Stage. Returns their acknowledgements, or the Spec and Tickets with full.",
		schema: object(map[string]any{
			"text":          text("The Requirement's text, in Markdown; its first line becomes the Spec's title."),
			"map":           ref("A Map with no unfinished Tickets to write the Spec from: the Requirement gets only the spec and tickets Stages, and a map Tag naming the Map."),
			"skip_grilling": flag("Straight to Spec: create no grilling Stage, so the spec Stage is written from the text alone. Can't be given with map."),
			"full":          flag("Return the whole Spec and Stage Tickets instead of their acknowledgements."),
		}, "text"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.SubmitRequirementInput) (any, error) {
			return c.SubmitRequirement(ctx, key, in)
		}),
	}
}
