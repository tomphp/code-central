package mcp

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// The published workflow (ticket 26).
func init() {
	tools["get_workflow"] = tool{
		description: "Get the whole workflow (every State, action, who may take it, what it requires) and how to work a Ticket; call it before working one.",
		schema:      object(map[string]any{}),
		call: decoded(func(ctx context.Context, c *core.Core, key string, _ struct{}) (any, error) {
			// The workflow is the same for every Project, but every tool on
			// an unknown key says so, so the agent can explain it.
			if _, err := c.GetProject(ctx, key); err != nil {
				return nil, err
			}
			return c.GetWorkflow(ctx)
		}),
	}
}

// Descriptions maps each tool to its description, for core's context
// budget test.
func Descriptions() map[string]string {
	out := map[string]string{}
	for name, t := range tools {
		out[name] = t.description
	}
	return out
}
