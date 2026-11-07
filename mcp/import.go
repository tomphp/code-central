package mcp

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// Scratch Import (ticket 40). The payload's full schema is in the guide;
// core validates it strictly, naming the field of any fault.
func init() {
	tags := tagFilter("Tags as {key: value}; source is set from source.")
	tools["get_import_guide"] = tool{
		description: "Get how to map a repository's .scratch/ folder into one import payload, and how to present the plan to the human.",
		schema:      object(map[string]any{}),
		call: decoded(func(ctx context.Context, c *core.Core, key string, _ struct{}) (any, error) {
			return c.GetImportGuide(ctx, key)
		}),
	}
	tools["import"] = tool{
		description: "Import a whole .scratch/ folder mapped as get_import_guide says; run with dry_run first and wait for the human's go-ahead.",
		schema: object(map[string]any{
			"payload": object(map[string]any{
				"specs": map[string]any{"type": "array", "description": "One Spec per feature folder.", "items": object(map[string]any{
					"source": text("The spec.md, map.md or folder path from the repository root."),
					"title":  text("The Spec's title."),
					"body":   text("The Spec's body, in Markdown."),
					"labels": list("Labels."),
					"tags":   tags,
				}, "source", "title")},
				"tickets": map[string]any{"type": "array", "description": "Every Ticket, in folder then file-number order.", "items": object(map[string]any{
					"source":       text("The file's path from the repository root."),
					"spec":         text("Its Spec's source."),
					"title":        text("The Ticket's title."),
					"state":        map[string]any{"type": "string", "enum": []string{"Draft", "Ready", "Done", "Cancelled"}, "description": "Defaults to Draft."},
					"description":  text("The description, in Markdown, without the header lines that became State, Blockers, Labels or Tags."),
					"labels":       list("Labels."),
					"tags":         tags,
					"blocked_by":   list("Sources of the Tickets it waits on, in this payload."),
					"replacements": list("For a Cancelled Ticket, sources of the Tickets that take over its work."),
					"comment":      text("A reason to keep as a comment (wontfix, Deferred)."),
				}, "source", "spec", "title")},
			}, "specs"),
			"dry_run": flag("Check everything and return the plan, writing nothing."),
		}, "payload"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.ImportInput) (any, error) {
			return c.Import(ctx, key, in)
		}),
	}
}
