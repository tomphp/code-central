package mcp

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// Failure Reports and giving up (ticket 30).
func init() {
	tools["report_failure"] = tool{
		description: "Report a failure when nothing the human can answer will unblock you: what broke, why, what to fix and where the work stands. Ends your Claim; then stop working this Ticket.",
		schema: object(map[string]any{
			"ticket":            ref("The Ticket."),
			"claim_id":          text("The Claim ID claim returned."),
			"what_broke":        text("A headline plus the symptom, in Markdown."),
			"why":               text("Why it broke, in Markdown."),
			"what_to_fix":       text("What has to change before this Ticket can go on, in Markdown."),
			"where_work_stands": text("Where the work stands: branch, commits, what is done, in Markdown."),
		}, "ticket", "claim_id", "what_broke", "why", "what_to_fix", "where_work_stands"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.ReportFailureInput) (any, error) {
			return c.ReportFailure(ctx, key, in)
		}),
	}
	tools["give_up"] = tool{
		description: "Give a claimed Ticket back to Ready when nothing is wrong but you can't finish it, with a note on how far you got. Ends your Claim; then stop working this Ticket.",
		schema: object(map[string]any{
			"ticket":   ref("The Ticket."),
			"claim_id": text("The Claim ID claim returned."),
			"note":     text("How far you got and why you are stopping, in Markdown."),
		}, "ticket", "claim_id", "note"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.GiveUpInput) (any, error) {
			return c.GiveUp(ctx, key, in)
		}),
	}
	// Sending back Accepted work that can't land (ticket 72).
	tools["send_back"] = tool{
		description: "Send an Accepted Ticket back to Ready when its branch can't land, such as on a merge conflict, with a note naming the base commit and the conflicting files. Returns an acknowledgement with the Ticket's State, or the Ticket with full.",
		schema: object(map[string]any{
			"ticket": ref("The Ticket."),
			"note":   text("Why it can't land: the base commit and the conflicting files, in Markdown."),
			"full":   flag("Return the whole Ticket instead of the acknowledgement."),
		}, "ticket", "note"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.SendBackInput) (any, error) {
			return c.SendBack(ctx, key, in)
		}),
	}
}
