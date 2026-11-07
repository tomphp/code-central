package mcp

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// Rounds of Questions (ticket 29).
func init() {
	tools["post_round"] = tool{
		description: "Ask the human a Round of Questions on a Ticket you hold, in one write; it moves to Needs Answer and you keep the Claim.",
		schema: object(map[string]any{
			"ticket":     ref("The Ticket."),
			"claim_id":   text("The Claim ID claim returned."),
			"session_id": text("Your session ID, shown to the human for resuming you."),
			"questions": map[string]any{
				"type":     "array",
				"minItems": 1,
				"items": object(map[string]any{
					"title":       text("A short title."),
					"body":        text("The Question in Markdown, with any choices as prose."),
					"recommended": text("Your recommended answer, if you have one."),
				}, "title", "body"),
				"description": "The Questions, each with a title, a body and an optional recommended answer.",
			},
		}, "ticket", "claim_id", "questions"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.PostRoundInput) (any, error) {
			return c.PostRound(ctx, key, in)
		}),
	}
	tools["pick_up_answers"] = tool{
		description: "Pick up the answers to your latest Round on an Answered Ticket you hold, moving it back to In Progress; returns the Round and comments since.",
		schema: object(map[string]any{
			"ticket":   ref("The Ticket."),
			"claim_id": text("The Claim ID claim returned."),
		}, "ticket", "claim_id"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.PickUpAnswersInput) (any, error) {
			return c.PickUpAnswers(ctx, key, in)
		}),
	}
}
