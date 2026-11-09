package frontdoor_test

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
)

// The service logs each MCP tool call's response size, so the context
// Code Central's own operations load can be measured from the log.

// syncBuffer is a log the server's goroutines write while the test reads.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func loggedServer(t *testing.T) (*codecentraltest.Server, *syncBuffer) {
	t.Helper()
	log := &syncBuffer{}
	srv := codecentraltest.NewServer(t, codecentraltest.WithLog(log))
	ctx := context.Background()
	must(t, func() error { _, err := srv.Core.CreateProject(ctx, core.CreateProjectInput{Key: "acme"}); return err })
	must(t, func() error {
		_, err := srv.Core.CreateSpec(ctx, "acme", core.CreateSpecInput{Title: "Login"})
		return err
	})
	must(t, func() error {
		_, err := srv.Core.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "Login form", Description: "A form."})
		return err
	})
	must(t, func() error {
		_, err := srv.Core.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "Login API", State: "Ready"})
		return err
	})
	return srv, log
}

func TestMCPLogsEachToolCallsResponseSize(t *testing.T) {
	srv, log := loggedServer(t)
	_, b := mcpCall(t, connect(t, srv, "acme"), "get_ticket", map[string]any{"ticket": 2})

	want := fmt.Sprintf("code-central serve: mcp get_ticket project=acme id=acme-2 bytes=%d\n", len(b))
	if got := log.String(); got != want {
		t.Fatalf("log %q, want %q", got, want)
	}
}

func TestMCPLogsTheIDACallReturns(t *testing.T) {
	srv, log := loggedServer(t)
	s := connect(t, srv, "acme")
	_, created := mcpCall(t, s, "create_spec", map[string]any{"title": "Search"})
	_, claimed := mcpCall(t, s, "claim", map[string]any{})

	want := fmt.Sprintf("code-central serve: mcp create_spec project=acme id=acme-4 bytes=%d\n", len(created)) +
		fmt.Sprintf("code-central serve: mcp claim project=acme id=acme-3 bytes=%d\n", len(claimed))
	if got := log.String(); got != want {
		t.Fatalf("log %q, want %q", got, want)
	}
}

func TestMCPLogsACallWithNoIDAndARejectedCall(t *testing.T) {
	srv, log := loggedServer(t)
	s := connect(t, srv, "acme")
	_, found := mcpCall(t, s, "find_tickets", map[string]any{})
	_, rejected := mcpCall(t, s, "get_ticket", map[string]any{"ticket": "acme-99"})

	want := fmt.Sprintf("code-central serve: mcp find_tickets project=acme bytes=%d\n", len(found)) +
		fmt.Sprintf("code-central serve: mcp get_ticket project=acme id=acme-99 bytes=%d error=not_found\n", len(rejected))
	if got := log.String(); got != want {
		t.Fatalf("log %q, want %q", got, want)
	}
}
