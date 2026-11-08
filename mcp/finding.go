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
		description: "Make a Ticket wait on another Ticket in this Project (any Spec) until that one is Done; a link that would form a cycle is rejected.",
		schema:      object(map[string]any{"ticket": ref("The Ticket that waits."), "blocker": ref("The Ticket it waits on.")}, "ticket", "blocker"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.BlockerInput) (any, error) {
			return c.AddBlocker(ctx, key, in)
		}),
	},
	"remove_blocker": {
		description: "Remove a Blocker from a Ticket.",
		schema:      object(map[string]any{"ticket": ref("The Ticket that waits."), "blocker": ref("The Blocker to remove.")}, "ticket", "blocker"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.BlockerInput) (any, error) {
			return c.RemoveBlocker(ctx, key, in)
		}),
	},
	"cancel": {
		description: "Cancel a Draft or Ready Ticket by naming the Replacements that take over its work; anything it blocked then waits on them.",
		schema: object(map[string]any{
			"ticket":       ref("The Ticket to cancel."),
			"replacements": map[string]any{"type": "array", "items": map[string]any{"type": []string{"string", "integer"}}, "description": "The Tickets taking over its work (at least one). IDs: <key>-<n> or <n>."},
			"reason":       text("Why, stored as a comment."),
		}, "ticket", "replacements"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.CancelInput) (any, error) {
			return c.Cancel(ctx, key, in)
		}),
	},
	"get_spec": {
		description: "Get a Spec with its body, a count of its Tickets per State, and its Tickets as find_tickets rows; parts (body, tickets, edits) and sections (heading texts) read less and add the body's outline.",
		schema: object(map[string]any{
			"spec":     ref("The Spec."),
			"parts":    list("Only these parts, any of body, tickets and edits; all three when left out."),
			"sections": list("Only the body's Sections under these headings, matched case-insensitively at any level."),
		}, "spec"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.GetSpecInput) (any, error) {
			return c.GetSpec(ctx, key, in)
		}),
	},
	"find_specs": {
		description: "Find this Project's Specs with their State summaries; finished Specs are left out unless finished is true.",
		schema: object(map[string]any{
			"title":        text("Only Specs whose title contains this text."),
			"labels":       list("Only Specs with all of these Labels."),
			"tags":         tagFilter("Only Specs with all of these Tags, as {key: value}."),
			"finished":     flag("Include finished Specs."),
			"requirements": flag("Only Requirements: Specs with Stage Tickets."),
		}),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.FindSpecsInput) (any, error) {
			return c.FindSpecs(ctx, key, in)
		}),
	},
}
