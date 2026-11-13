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
	"fmt"
	"io"
	"net/http"
	"regexp"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/usage"
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
		description: "Create a Spec, the description of a piece of work delivered through one or more Tickets; returns an acknowledgement with its ID, or the Spec with full.",
		schema: object(map[string]any{
			"title": text("The Spec's title."),
			"body":  text("The Spec in Markdown."),
			"full":  flag("Return the whole Spec instead of the acknowledgement."),
		}, "title"),
		call: func(ctx context.Context, c *core.Core, key string, args []byte) (any, error) {
			var in core.CreateSpecInput
			if err := core.Decode(ctx, args, &in); err != nil {
				return nil, err
			}
			return c.CreateSpec(ctx, key, in)
		},
	},
	"create_ticket": {
		description: "Create a Ticket in a Spec, as Draft (the default) or Ready. Returns an acknowledgement with the Ticket's State, or the Ticket with full.",
		schema: object(map[string]any{
			"spec":        ref("The Spec it belongs to."),
			"title":       text("The Ticket's title."),
			"description": text("What to build, in Markdown."),
			"state":       map[string]any{"type": "string", "enum": []string{"Draft", "Ready"}, "description": "Draft (the default) or Ready."},
			"labels":      list("Labels to add."),
			"blocked_by":  map[string]any{"type": "array", "items": map[string]any{"type": []string{"string", "integer"}}, "description": "Tickets it waits on until they are Done. IDs: <key>-<n> or <n>."},
			"tags":        map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Tags to set on it, as {key: value}; checked as set_tags checks them, so effort must be a valid level."},
			"full":        flag("Return the whole Ticket instead of the acknowledgement."),
		}, "spec", "title"),
		call: func(ctx context.Context, c *core.Core, key string, args []byte) (any, error) {
			var in core.CreateTicketInput
			if err := core.Decode(ctx, args, &in); err != nil {
				return nil, err
			}
			return c.CreateTicket(ctx, key, in)
		},
	},
	"get_ticket": {
		description: "Get one Ticket: its description, State, Blockers, past Claims and how each ended, every Failure Report and every comment; left_out counts what it leaves out, and parts (claims, failure_reports, comments, rounds, moves, edits) asks for exactly those.",
		schema: object(map[string]any{
			"ticket": ref("The Ticket."),
			"parts":  list("Only these parts, any of claims, failure_reports, comments, rounds, moves and edits; claims, failure_reports and comments when left out."),
		}, "ticket"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.GetTicketInput) (any, error) {
			return c.GetTicket(ctx, key, in)
		}),
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
			if err := core.Decode(ctx, args, &in); err != nil {
				return nil, err
			}
			return c.FindTickets(ctx, key, in)
		},
	},
	"claim": {
		description: "Claim the next Ready Ticket (or the one named, or the next in a Spec); returns a Claim ID and the Ticket, or ticket null when none. Claiming a Ticket this session already holds, with its session_id, returns the same Claim.",
		schema: object(map[string]any{
			"spec":           ref("Claim the next Ticket in this Spec."),
			"ticket":         ref("Claim this Ticket."),
			"session_id":     text("Your session ID, shown to the human for resuming you."),
			"working_dir":    text("The directory you work in, where the human runs the resume command. A hook usually fills it in for you."),
			"resume_command": text("The command that resumes your session from working_dir, shown to the human as is. Without one the human is shown claude --resume <session_id>."),
		}),
		call: func(ctx context.Context, c *core.Core, key string, args []byte) (any, error) {
			var in core.ClaimInput
			if err := core.Decode(ctx, args, &in); err != nil {
				return nil, err
			}
			return c.Claim(ctx, key, in)
		},
	},
	"finish": {
		description: "Finish a claimed Ticket into In Review with a summary: what was done, where (branch or commit), and what to check. Returns an acknowledgement with the next step, or the Ticket with full.",
		schema: object(map[string]any{
			"ticket":         ref("The Ticket."),
			"claim_id":       text("The Claim ID claim returned."),
			"summary":        text("What was done, where, and what the reviewer should check, in Markdown."),
			"branch":         text("The branch the work is on, if any. Accepting work on a branch moves it to Accepted, to land once merged."),
			"commit":         text("The branch's head commit, if any."),
			"recommendation": map[string]any{"type": "string", "enum": []string{"implement", "split"}, "description": "Required on a spec Stage only: implement or split."},
			"full":           flag("Return the whole Ticket instead of the acknowledgement."),
		}, "ticket", "claim_id", "summary"),
		call: func(ctx context.Context, c *core.Core, key string, args []byte) (any, error) {
			var in core.FinishInput
			if err := core.Decode(ctx, args, &in); err != nil {
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
	for name, t := range sessionTools {
		tools[name] = t
	}
}

// sessionTools keep a Claim's session ID current (ticket 31).
var sessionTools = map[string]tool{
	"set_session": {
		description: "Record the session now working your Claim, and how to get back into it, so the human can resume it. A hook usually fills session_id in for you. Returns an acknowledgement with the Ticket's State, or the Ticket with full.",
		schema: object(map[string]any{
			"ticket":         ref("The Ticket."),
			"claim_id":       text("The Claim ID claim returned."),
			"session_id":     text("Your session ID."),
			"working_dir":    text("The directory you work in now. Left out, the recorded one stays."),
			"resume_command": text("The command that resumes your session from working_dir. Left out, the recorded one stays."),
			"full":           flag("Return the whole Ticket instead of the acknowledgement."),
		}, "ticket", "claim_id", "session_id"),
		call: func(ctx context.Context, c *core.Core, key string, args []byte) (any, error) {
			var in core.SetSessionInput
			if err := core.Decode(ctx, args, &in); err != nil {
				return nil, err
			}
			return c.SetSession(ctx, key, in)
		},
	},
}

// definingTools shape work after it is created (ticket 27).
var definingTools = map[string]tool{
	"edit_spec": {
		description: "Edit a Spec's title or body; a body given replaces the old one whole, and the old text is kept in its edit history. Returns an acknowledgement naming the changed fields, or the Spec with full.",
		schema: object(map[string]any{
			"spec":  ref("The Spec."),
			"title": text("The new title."),
			"body":  text("The new body in Markdown, replacing the old one whole."),
			"full":  flag("Return the whole Spec instead of the acknowledgement."),
		}, "spec"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.EditSpecInput) (any, error) {
			return c.EditSpec(ctx, key, in)
		}),
	},
	"edit_ticket": {
		description: "Edit a Ticket's title or description; the old text is kept in its edit history. Returns an acknowledgement with the Ticket's State, or the Ticket with full.",
		schema: object(map[string]any{
			"ticket":      ref("The Ticket."),
			"title":       text("The new title."),
			"description": text("The new description in Markdown, replacing the old one whole."),
			"full":        flag("Return the whole Ticket instead of the acknowledgement."),
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
		description: "Comment on any Ticket, even one another agent holds; no Claim ID is needed. Returns an acknowledgement with the Ticket's State, or the Ticket with full.",
		schema:      object(map[string]any{"ticket": ref("The Ticket."), "body": text("The comment in Markdown."), "full": flag("Return the whole Ticket instead of the acknowledgement.")}, "ticket", "body"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.CommentInput) (any, error) {
			return c.Comment(ctx, key, in)
		}),
	},
	"approve": {
		description: "Approve a Draft Ticket you have finished defining, moving it to Ready so it can be claimed. Returns an acknowledgement with the Ticket's State, or the Ticket with full.",
		schema:      object(map[string]any{"ticket": ref("The Ticket."), "full": flag("Return the whole Ticket instead of the acknowledgement.")}, "ticket"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.TicketInput) (any, error) {
			return c.Approve(ctx, key, in)
		}),
	},
	"defer": {
		description: "Defer a Ready Ticket that isn't ready back to Draft, with the reason stored as a comment. Returns an acknowledgement with the Ticket's State, or the Ticket with full.",
		schema:      object(map[string]any{"ticket": ref("The Ticket."), "reason": text("Why it isn't ready."), "full": flag("Return the whole Ticket instead of the acknowledgement.")}, "ticket", "reason"),
		call: decoded(func(ctx context.Context, c *core.Core, key string, in core.DeferInput) (any, error) {
			return c.Defer(ctx, key, in)
		}),
	},
}

// decoded makes a tool call that decodes its arguments strictly into In.
func decoded[In any](f func(ctx context.Context, c *core.Core, key string, in In) (any, error)) func(context.Context, *core.Core, string, []byte) (any, error) {
	return func(ctx context.Context, c *core.Core, key string, args []byte) (any, error) {
		var in In
		if err := core.Decode(ctx, args, &in); err != nil {
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
//
// log, when not nil, gets one line per tool call with the size of the
// text the agent is given (see logCall). Each such call is also recorded
// as Usage, under the URL's Project when it exists.
func Register(mux *http.ServeMux, c *core.Core, log io.Writer) {
	if log == nil {
		log = io.Discard
	}
	h := sdk.NewStreamableHTTPHandler(func(r *http.Request) *sdk.Server {
		return newServer(c, r.PathValue("key"), log)
	}, &sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true, DisableLocalhostProtection: true})
	mux.Handle("/p/{key}/mcp", h)
}

func newServer(c *core.Core, key string, log io.Writer) *sdk.Server {
	s := sdk.NewServer(&sdk.Implementation{Name: "code-central", Version: "v1"}, &sdk.ServerOptions{Instructions: core.Instructions})
	for _, op := range core.Operations {
		t, ok := tools[op.Name]
		if !ok || op.Audience != core.AgentAndHuman {
			continue
		}
		s.AddTool(&sdk.Tool{Name: op.Name, Description: t.description, InputSchema: t.schema},
			func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
				v, err := t.call(core.WithActor(ctx, core.Agent), c, key, req.Params.Arguments)
				res, text, code, err := result(v, err)
				if err == nil {
					logCall(log, op.Name, key, req.Params.Arguments, text, code)
					c.RecordCall(key, usage.Call{
						Op: op.Name, ID: callID(key, req.Params.Arguments, text),
						RequestBytes: len(req.Params.Arguments), ResponseBytes: len(text), Error: string(code),
					})
				}
				return res, err
			})
	}
	return s
}

// result is a successful value as JSON text, or a core error as JSON text
// with isError set and its code returned. Any other error is a bug inside
// the service.
func result(v any, err error) (*sdk.CallToolResult, []byte, core.Code, error) {
	var code core.Code
	if err != nil {
		var e *core.Error
		if !errors.As(err, &e) {
			e = &core.Error{Code: "internal", Message: err.Error()}
		}
		v, code = e, e.Code
	}
	b, merr := json.Marshal(v)
	if merr != nil {
		return nil, nil, "", merr
	}
	res := &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(b)}}, IsError: code != ""}
	return res, b, code, nil
}

// logCall writes a tool call's line to the service log:
//
//	code-central serve: mcp get_ticket project=acme id=acme-2 bytes=1234
//
// bytes is the length of the text the agent is given, the context the
// call loads. id is the Ticket or Spec the call names, or else the one it
// returns, and is left out when there is neither; a rejected call ends
// with error=<code>.
func logCall(log io.Writer, op, key string, args, text []byte, code core.Code) {
	line := fmt.Sprintf("code-central serve: mcp %s project=%s", op, key)
	if id := callID(key, args, text); id != "" {
		line += " id=" + id
	}
	line += fmt.Sprintf(" bytes=%d", len(text))
	if code != "" {
		line += " error=" + string(code)
	}
	fmt.Fprintln(log, line)
}

// callID is the ID a call's arguments name (ticket, spec or target), as
// <key>-<n>, or else the ID its result holds, or "".
func callID(key string, args, text []byte) string {
	var in struct{ Ticket, Spec, Target core.Ref }
	_ = json.Unmarshal(args, &in)
	for _, r := range []core.Ref{in.Ticket, in.Spec, in.Target} {
		if r == "" {
			continue
		}
		if bareNumber.MatchString(string(r)) {
			return key + "-" + string(r)
		}
		return string(r)
	}
	var out struct {
		ID     string `json:"id"`
		Ticket *struct {
			ID string `json:"id"`
		} `json:"ticket"`
	}
	_ = json.Unmarshal(text, &out)
	if out.ID == "" && out.Ticket != nil {
		return out.Ticket.ID
	}
	return out.ID
}

var bareNumber = regexp.MustCompile(`^[0-9]+$`)
