package mcp

import (
	"context"

	"github.com/tomoram/code-central/core"
)

func init() {
	for name, t := range findingTools {
		tools[name] = t
	}
}

// tagFilter is a {key: value} Tag filter: every pair must match exactly.
func tagFilter(desc string) map[string]any {
	return map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": desc}
}

func flag(desc string) map[string]any { return map[string]any{"type": "boolean", "description": desc} }

// findingTools order and cancel work and find it (ticket 28).
var findingTools = map[string]tool{
	"add_blocker": {
		description: "Make a Ticket wait on another Ticket in this Project (any Spec) until that one is Done; a link that would form a cycle is rejected. Returns an acknowledgement with the Ticket's State, or the Ticket with full.",
		schema:      object(map[string]any{"ticket": ref("The Ticket that waits."), "blocker": ref("The Ticket it waits on."), "full": flag("Return the whole Ticket instead of the acknowledgement.")}, "ticket", "blocker"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.BlockerInput) (any, error) {
			return c.AddBlocker(ctx, key, in)
		}),
	},
	"remove_blocker": {
		description: "Remove a Blocker from a Ticket. Returns an acknowledgement with the Ticket's State, or the Ticket with full.",
		schema:      object(map[string]any{"ticket": ref("The Ticket that waits."), "blocker": ref("The Blocker to remove."), "full": flag("Return the whole Ticket instead of the acknowledgement.")}, "ticket", "blocker"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.BlockerInput) (any, error) {
			return c.RemoveBlocker(ctx, key, in)
		}),
	},
	"cancel": {
		description: "Cancel a Draft or Ready Ticket by naming the Replacements that take over its work; anything it blocked then waits on them. Returns an acknowledgement with the Ticket's State, or the Ticket with full.",
		schema: object(map[string]any{
			"ticket":       ref("The Ticket to cancel."),
			"replacements": map[string]any{"type": "array", "items": map[string]any{"type": []string{"string", "integer"}}, "description": "The Tickets taking over its work (at least one). IDs: <key>-<n> or <n>."},
			"reason":       text("Why, stored as a comment."),
			"full":         flag("Return the whole Ticket instead of the acknowledgement."),
		}, "ticket", "replacements"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.CancelInput) (any, error) {
			return c.Cancel(ctx, key, in)
		}),
	},
}
