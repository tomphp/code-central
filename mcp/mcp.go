// Package mcp is the MCP front door: a thin translator from tool calls
// onto core's agent operations. It is served statelessly over Streamable
// HTTP at /p/<key>/mcp; the URL names the Project, so no tool takes a
// Project argument. Every call names the agent Actor.
//
// Errors are tool results with isError set and the one error JSON shape
// as text, never protocol errors, so an agent can read and explain them.
package mcp

import (
	"context"
	"encoding/json"
	"net/http"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ops"
)

// A tool is one agent operation as an MCP tool: a one-line description,
// an input schema and the operation it calls.
type tool struct {
	description string
	schema      map[string]any
	op          ops.Operation
}

func object(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func text(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

// ref is an ID within this Project: "<key>-<n>" or a bare <n>.
func ref(desc string) map[string]any {
	return map[string]any{"type": []string{"string", "integer"}, "description": desc + " An ID: <key>-<n> or <n>."}
}

// Register serves MCP at /p/{key}/mcp on the service's one mux, over d.
// An unknown key is still served: every tool call then returns
// unknown_project, which tells the agent to ask for `code-central init`.
//
// The library's own Host check is off: the shared middleware guards this
// route along with the HTTP API.
func Register(mux *http.ServeMux, d ops.Dispatcher) {
	h := sdk.NewStreamableHTTPHandler(func(r *http.Request) *sdk.Server {
		return newServer(d, r.PathValue("key"))
	}, &sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true, DisableLocalhostProtection: true})
	mux.Handle("/p/{key}/mcp", h)
}

func newServer(d ops.Dispatcher, key string) *sdk.Server {
	s := sdk.NewServer(&sdk.Implementation{Name: "code-central", Version: "v1"}, &sdk.ServerOptions{Instructions: core.Instructions})
	for _, op := range ops.All() {
		t, ok := derived[op.Name]
		if !ok || op.Audience != ops.AgentAndHuman {
			continue
		}
		s.AddTool(&sdk.Tool{Name: op.Name, Description: t.description, InputSchema: t.schema},
			func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
				// The dispatch decodes the arguments strictly.
				return result(d.Dispatch(ctx, ops.Call{Op: t.op, Key: key, Args: req.Params.Arguments, Actor: core.Agent}))
			})
	}
	return s
}

// result is a successful value as JSON text, or a core error as JSON text
// with isError set. Any other error is a bug inside the service.
func result(v any, err error) (*sdk.CallToolResult, error) {
	if err != nil {
		v = core.ErrorOf(err)
	}
	b, merr := json.Marshal(v)
	if merr != nil {
		return nil, merr
	}
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(b)}}, IsError: err != nil}, nil
}
