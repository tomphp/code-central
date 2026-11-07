// Package mcp is the MCP front door: a thin translator from tool calls
// onto core's agent operations. It is served statelessly over Streamable
// HTTP at /p/<key>/mcp; the URL names the Project, so no tool takes a
// Project argument. Every write is made as the agent Actor.
//
// Errors are tool results with isError set and the one error JSON shape
// as text, never protocol errors, so an agent can read and explain them.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tomoram/code-central/core"
)

// A tool is one agent operation as an MCP tool: a one-line description,
// an input schema and a call that decodes the arguments strictly.
type tool struct {
	description string
	schema      map[string]any
	call        func(ctx context.Context, c *core.Core, key string, args []byte) (any, error)
}

// tools maps each agent operation in core.Operations to its tool. The
// cross-door suite checks the served list against core.Operations.
var tools = map[string]tool{
	"create_spec": {
		description: "Create a Spec, the description of a piece of work delivered through one or more Tickets.",
		schema: object(map[string]any{
			"title": text("The Spec's title."),
			"body":  text("The Spec in Markdown."),
		}, "title"),
		call: func(ctx context.Context, c *core.Core, key string, args []byte) (any, error) {
			var in core.CreateSpecInput
			if err := core.Decode(args, &in); err != nil {
				return nil, err
			}
			return c.CreateSpec(ctx, key, in)
		},
	},
	"create_ticket": {
		description: "Create a Ticket in a Spec, as Draft (the default) or Ready.",
		schema: object(map[string]any{
			"spec":        ref("The Spec it belongs to."),
			"title":       text("The Ticket's title."),
			"description": text("What to build, in Markdown."),
			"state":       map[string]any{"type": "string", "enum": []string{"Draft", "Ready"}, "description": "Draft (the default) or Ready."},
			"labels":      list("Labels to add."),
			"blocked_by":  map[string]any{"type": "array", "items": map[string]any{"type": []string{"string", "integer"}}, "description": "Tickets it waits on until they are Done. IDs: <key>-<n> or <n>."},
		}, "spec", "title"),
		call: func(ctx context.Context, c *core.Core, key string, args []byte) (any, error) {
			var in core.CreateTicketInput
			if err := core.Decode(args, &in); err != nil {
				return nil, err
			}
			return c.CreateTicket(ctx, key, in)
		},
	},
	"get_ticket": {
		description: "Get one Ticket's full view, including its description and moves.",
		schema:      object(map[string]any{"ticket": ref("The Ticket.")}, "ticket"),
		call: func(ctx context.Context, c *core.Core, key string, args []byte) (any, error) {
			var in struct {
				Ticket core.Ref `json:"ticket"`
			}
			if err := core.Decode(args, &in); err != nil {
				return nil, err
			}
			return c.GetTicket(ctx, key, in.Ticket)
		},
	},
	"find_tickets": {
		description: "Find this Project's Tickets in claim order; every filter given must match.",
		schema: object(map[string]any{
			"state":     list("Only Tickets in one of these States."),
			"title":     text("Only Tickets whose title contains this text."),
			"labels":    list("Only Tickets with all of these Labels."),
			"tags":      tagFilter("Only Tickets with all of these Tags, as {key: value}."),
			"spec":      ref("Only Tickets in this Spec."),
			"claimable": flag("Only Tickets that are (true) or aren't (false) claimable."),
		}),
		call: func(ctx context.Context, c *core.Core, key string, args []byte) (any, error) {
			var in core.FindTicketsInput
			if err := core.Decode(args, &in); err != nil {
				return nil, err
			}
			return c.FindTickets(ctx, key, in)
		},
	},
	"claim": {
		description: "Claim the next Ready Ticket (or the one named, or the next in a Spec); returns a Claim ID and the Ticket, or ticket null when none.",
		schema: object(map[string]any{
			"spec":       ref("Claim the next Ticket in this Spec."),
			"ticket":     ref("Claim this Ticket."),
			"session_id": text("Your session ID, shown to the human for resuming you."),
		}),
		call: func(ctx context.Context, c *core.Core, key string, args []byte) (any, error) {
			var in core.ClaimInput
			if err := core.Decode(args, &in); err != nil {
				return nil, err
			}
			return c.Claim(ctx, key, in)
		},
	},
	"finish": {
		description: "Finish a claimed Ticket into In Review with a summary: what was done, where (branch or commit), and what to check.",
		schema: object(map[string]any{
			"ticket":   ref("The Ticket."),
			"claim_id": text("The Claim ID claim returned."),
			"summary":  text("What was done, where, and what the reviewer should check, in Markdown."),
		}, "ticket", "claim_id", "summary"),
		call: func(ctx context.Context, c *core.Core, key string, args []byte) (any, error) {
			var in core.FinishInput
			if err := core.Decode(args, &in); err != nil {
				return nil, err
			}
			return c.Finish(ctx, key, in)
		},
	},
}

func init() {
	for name, t := range definingTools {
		tools[name] = t
	}
}

// definingTools shape work after it is created (ticket 27).
var definingTools = map[string]tool{
	"edit_spec": {
		description: "Edit a Spec's title or body; a body given replaces the old one whole, and the old text is kept in its edit history.",
		schema: object(map[string]any{
			"spec":  ref("The Spec."),
			"title": text("The new title."),
			"body":  text("The new body in Markdown, replacing the old one whole."),
		}, "spec"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.EditSpecInput) (any, error) {
			return c.EditSpec(ctx, key, in)
		}),
	},
	"edit_ticket": {
		description: "Edit a Ticket's title or description; the old text is kept in its edit history.",
		schema: object(map[string]any{
			"ticket":      ref("The Ticket."),
			"title":       text("The new title."),
			"description": text("The new description in Markdown, replacing the old one whole."),
		}, "ticket"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.EditTicketInput) (any, error) {
			return c.EditTicket(ctx, key, in)
		}),
	},
	"add_labels": {
		description: "Add Labels to a Spec or Ticket, leaving its other Labels alone.",
		schema:      object(map[string]any{"target": ref("The Spec or Ticket."), "labels": list("Labels to add.")}, "target", "labels"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.LabelsInput) (any, error) {
			return c.AddLabels(ctx, key, in)
		}),
	},
	"remove_labels": {
		description: "Remove Labels from a Spec or Ticket, leaving its other Labels alone.",
		schema:      object(map[string]any{"target": ref("The Spec or Ticket."), "labels": list("Labels to remove.")}, "target", "labels"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.LabelsInput) (any, error) {
			return c.RemoveLabels(ctx, key, in)
		}),
	},
	"set_tags": {
		description: "Set Tags on a Spec or Ticket, overwriting only the keys named; keys are lowercase [a-z0-9._-], values one non-empty line.",
		schema: object(map[string]any{
			"target": ref("The Spec or Ticket."),
			"tags":   map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Tags to set, as {key: value}."},
		}, "target", "tags"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.SetTagsInput) (any, error) {
			return c.SetTags(ctx, key, in)
		}),
	},
	"remove_tags": {
		description: "Remove Tags from a Spec or Ticket by key.",
		schema:      object(map[string]any{"target": ref("The Spec or Ticket."), "keys": list("Tag keys to remove.")}, "target", "keys"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.RemoveTagsInput) (any, error) {
			return c.RemoveTags(ctx, key, in)
		}),
	},
	"comment": {
		description: "Comment on any Ticket, even one another agent holds; no Claim ID is needed.",
		schema:      object(map[string]any{"ticket": ref("The Ticket."), "body": text("The comment in Markdown.")}, "ticket", "body"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.CommentInput) (any, error) {
			return c.Comment(ctx, key, in)
		}),
	},
	"approve": {
		description: "Approve a Draft Ticket you have finished defining, moving it to Ready so it can be claimed.",
		schema:      object(map[string]any{"ticket": ref("The Ticket.")}, "ticket"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.TicketInput) (any, error) {
			return c.Approve(ctx, key, in)
		}),
	},
	"defer": {
		description: "Defer a Ready Ticket that isn't ready back to Draft, with the reason stored as a comment.",
		schema:      object(map[string]any{"ticket": ref("The Ticket."), "reason": text("Why it isn't ready.")}, "ticket", "reason"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.DeferInput) (any, error) {
			return c.Defer(ctx, key, in)
		}),
	},
}

// decoded makes a tool call that decodes its arguments strictly into In.
func decoded[In any](f func(ctx context.Context, c *core.Core, key string, in In) (any, error)) func(context.Context, *core.Core, string, []byte) (any, error) {
	return func(ctx context.Context, c *core.Core, key string, args []byte) (any, error) {
		var in In
		if err := core.Decode(args, &in); err != nil {
			return nil, err
		}
		return f(ctx, c, key, in)
	}
}

func object(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func text(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

func list(desc string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": desc}
}

// ref is an ID within this Project: "<key>-<n>" or a bare <n>.
func ref(desc string) map[string]any {
	return map[string]any{"type": []string{"string", "integer"}, "description": desc + " An ID: <key>-<n> or <n>."}
}

// Register serves MCP at /p/{key}/mcp on the service's one mux. An
// unknown key is still served: every tool call then returns
// unknown_project, which tells the agent to ask for `code-central init`.
//
// The library's own Host check is off: the shared middleware guards this
// route along with the HTTP API.
func Register(mux *http.ServeMux, c *core.Core) {
	h := sdk.NewStreamableHTTPHandler(func(r *http.Request) *sdk.Server {
		return newServer(c, r.PathValue("key"))
	}, &sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true, DisableLocalhostProtection: true})
	mux.Handle("/p/{key}/mcp", h)
}

func newServer(c *core.Core, key string) *sdk.Server {
	s := sdk.NewServer(&sdk.Implementation{Name: "code-central", Version: "v1"}, nil)
	for _, op := range core.Operations {
		t, ok := tools[op.Name]
		if !ok || op.Audience != core.AgentAndHuman {
			continue
		}
		s.AddTool(&sdk.Tool{Name: op.Name, Description: t.description, InputSchema: t.schema},
			func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
				v, err := t.call(core.WithActor(ctx, core.Agent), c, key, req.Params.Arguments)
				return result(v, err)
			})
	}
	return s
}

// result is a successful value as JSON text, or a core error as JSON text
// with isError set. Any other error is a bug inside the service.
func result(v any, err error) (*sdk.CallToolResult, error) {
	isError := false
	if err != nil {
		var e *core.Error
		if !errors.As(err, &e) {
			e = &core.Error{Code: "internal", Message: err.Error()}
		}
		v, isError = e, true
	}
	b, merr := json.Marshal(v)
	if merr != nil {
		return nil, merr
	}
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(b)}}, IsError: isError}, nil
}
