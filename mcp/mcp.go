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
	"github.com/tomoram/code-central/ops"
	"github.com/tomoram/code-central/usage"
)

// A tool is one agent operation as an MCP tool: a one-line description,
// an input schema and a call that decodes the arguments strictly.
type tool struct {
	description string
	schema      map[string]any
	call        func(ctx context.Context, c *core.Core, key string, args []byte) (any, error)
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
	for _, op := range ops.All() {
		t, ok := derived[op.Name]
		if !ok || op.Audience != ops.AgentAndHuman {
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
