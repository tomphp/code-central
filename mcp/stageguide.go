package mcp

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// Stage Guides (ticket 73).
func init() {
	tools["get_stage_guide"] = tool{
		description: "Get a Stage Guide again (claim returns it): what a Ticket of that Stage produces and how to finish it.",
		schema: object(map[string]any{
			"stage": map[string]any{"type": "string", "enum": core.GuideNames, "description": "The guide's name: the Ticket's Stage Label without stage:, map-<type> for a Wayfinder Label wayfinder:<type>, prototype, or implementation when it has none of these."},
		}, "stage"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.StageGuideInput) (any, error) {
			// The guides are the same for every Project, but every tool on
			// an unknown key says so.
			if _, err := c.GetProject(ctx, key); err != nil {
				return nil, err
			}
			return c.GetStageGuide(ctx, in)
		}),
	}
}
