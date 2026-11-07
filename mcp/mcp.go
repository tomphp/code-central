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
			"state":  list("Only Tickets in one of these States."),
			"title":  text("Only Tickets whose title contains this text."),
			"labels": list("Only Tickets with all of these Labels."),
			"spec":   ref("Only Tickets in this Spec."),
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
