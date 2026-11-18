package mcp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/mcp"
	"github.com/tomoram/code-central/ops"
)

// recordingDispatcher records each call it dispatches and answers with
// answer and err.
type recordingDispatcher struct {
	calls  []ops.Call
	answer any
	err    error
}

func (d *recordingDispatcher) Dispatch(_ context.Context, call ops.Call) (any, error) {
	d.calls = append(d.calls, call)
	return d.answer, d.err
}

// The MCP door depends on ops.Dispatcher alone: a tool call becomes one
// Call naming the agent Actor, with the URL's key and the arguments as
// sent, and the dispatch's result or error is the tool's text.
func TestTheMCPDoorDispatchesEachToolCallThroughItsDispatcher(t *testing.T) {
	d := &recordingDispatcher{answer: map[string]string{"id": "acme-2"}}
	mux := http.NewServeMux()
	mcp.Register(mux, d)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	s, err := sdk.NewClient(&sdk.Implementation{Name: "door-test", Version: "v0"}, nil).Connect(context.Background(),
		&sdk.StreamableClientTransport{Endpoint: srv.URL + "/p/acme/mcp", DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	if text, isError := callTool(t, s, "get_ticket", map[string]any{"ticket": 2}); text != `{"id":"acme-2"}` || isError {
		t.Fatalf("text %s, isError %v", text, isError)
	}
	if len(d.calls) != 1 {
		t.Fatalf("%d calls, want 1", len(d.calls))
	}
	got := d.calls[0]
	want := ops.Call{Op: ops.GetTicket, Key: "acme", Args: []byte(`{"ticket":2}`), Actor: core.Agent}
	if got.Op != want.Op || got.Key != want.Key || got.In != nil || string(got.Args) != string(want.Args) || got.Actor != want.Actor {
		t.Fatalf("call %+v, want %+v", got, want)
	}

	d.err = &core.Error{Code: core.NotFound, Message: "No Ticket acme-2."}
	if text, isError := callTool(t, s, "get_ticket", map[string]any{"ticket": 2}); text != `{"code":"not_found","message":"No Ticket acme-2."}` || !isError {
		t.Fatalf("error: text %s, isError %v", text, isError)
	}
}

func callTool(t *testing.T, s *sdk.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := s.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: protocol error %v", name, err)
	}
	return res.Content[0].(*sdk.TextContent).Text, res.IsError
}
