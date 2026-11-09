package mcp

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// Submitting a Prototype.
func init() {
	tools["submit_prototype"] = tool{
		description: "Submit a Prototype on any Spec: a Ready Ticket labelled prototype, titled \"Prototype: <first line>\" with the text as its description, for a Factory to build on a local branch that never lands; returns its acknowledgement, or the Ticket with full.",
		schema: object(map[string]any{
			"spec": ref("The Spec the Prototype is on."),
			"text": text("What to prototype and the question it answers, in Markdown; its first line becomes the title."),
			"full": flag("Return the whole Ticket instead of the acknowledgement."),
		}, "spec", "text"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.SubmitPrototypeInput) (any, error) {
			return c.SubmitPrototype(ctx, key, in)
		}),
	}
}
