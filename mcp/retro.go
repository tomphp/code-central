package mcp

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// Starting a Retro.
func init() {
	tools["submit_retro"] = tool{
		description: "Start a Retro on finished work, a Done or Cancelled Ticket or a Finished Spec: a Ready Ticket labelled retro on the Project's Retros Spec, titled \"Retro: <target title>\", whose description lists every Claim on the work to read; returns its acknowledgement, or the Ticket with full.",
		schema: object(map[string]any{
			"target": ref("The finished Ticket or Spec the Retro looks back on."),
			"note":   text("What the human wants the Retro to look at, in Markdown; optional."),
			"full":   flag("Return the whole Ticket instead of the acknowledgement."),
		}, "target"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.SubmitRetroInput) (any, error) {
			return c.SubmitRetro(ctx, key, in)
		}),
	}
}
