package frontdoor_test

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
)

// get_ticket's smaller default and its parts (ticket 192, ADR 0015): the
// same through MCP and the CLI, while the HTTP API, which the UI and the
// Factory read, returns the whole Ticket unless parts are named.
func TestTheSmallerGetTicketThroughEveryDoor(t *testing.T) {
	srv := newServer(t, func(t *testing.T, srv *codecentraltest.Server) {
		ctx := context.Background()
		must(t, func() error {
			_, err := srv.Core.Comment(ctx, "acme", core.CommentInput{Ticket: "2", Body: "Use the old form"})
			return err
		})
		must(t, func() error {
			_, err := srv.Core.Approve(ctx, "acme", core.TicketInput{Ticket: "2"})
			return err
		})
	})
	s := connect(t, srv, "acme")
	has := func(t *testing.T, out map[string]any, parts ...string) {
		t.Helper()
		for _, p := range core.TicketParts {
			_, ok := out[p]
			if want := slices.Contains(parts, p); ok != want {
				t.Fatalf("%s there: %v, want %v: %v", p, ok, want, out)
			}
		}
		if out["title"] != "Login form" {
			t.Fatalf("title %v", out["title"])
		}
	}
	reads := map[string]struct {
		check func(*testing.T, map[string]any)
		calls []map[string]any
	}{
		"default": {
			check: func(t *testing.T, out map[string]any) {
				has(t, out, core.DefaultTicketParts...)
				if c := out["comments"].([]any); len(c) != 1 {
					t.Fatalf("comments %v", c)
				}
				if !reflect.DeepEqual(out["left_out"], map[string]any{"moves": float64(1)}) {
					t.Fatalf("left_out %v", out["left_out"])
				}
			},
			calls: []map[string]any{
				mcpOK(t, s, "get_ticket", map[string]any{"ticket": "acme-2"}),
				cliOK(t, srv, []string{"get-ticket", "acme-2"}),
			},
		},
		"parts": {
			check: func(t *testing.T, out map[string]any) {
				has(t, out, "moves", "rounds")
				if !reflect.DeepEqual(out["left_out"], map[string]any{"comments": float64(1)}) {
					t.Fatalf("left_out %v", out["left_out"])
				}
			},
			calls: []map[string]any{
				httpOK(t, srv, httpCall{"GET", "/api/p/acme/tickets/2?part=moves&part=rounds", ""}),
				mcpOK(t, s, "get_ticket", map[string]any{"ticket": "acme-2", "parts": []string{"moves", "rounds"}}),
				cliOK(t, srv, []string{"get-ticket", "acme-2", "--part", "moves", "--part", "rounds"}),
			},
		},
		"whole": {
			check: func(t *testing.T, out map[string]any) {
				has(t, out, core.TicketParts...)
				if _, ok := out["left_out"]; ok {
					t.Fatalf("left_out on the whole Ticket: %v", out)
				}
			},
			calls: []map[string]any{
				httpOK(t, srv, httpCall{"GET", "/api/p/acme/tickets/2", ""}),
				mcpOK(t, s, "get_ticket", map[string]any{"ticket": "acme-2", "parts": core.TicketParts}),
			},
		},
	}
	doors := map[string][]string{"default": {"mcp", "cli"}, "parts": {"http", "mcp", "cli"}, "whole": {"http", "mcp"}}
	for name, r := range reads {
		for i, out := range r.calls {
			t.Run(name+"/"+doors[name][i], func(t *testing.T) { r.check(t, out) })
		}
	}

	t.Run("unknown part/http", func(t *testing.T) {
		httpRejected(t, srv, httpCall{"GET", "/api/p/acme/tickets/2?part=history", ""}, core.Invalid)
	})
	t.Run("unknown part/mcp", func(t *testing.T) {
		mcpRejected(t, s, "get_ticket", map[string]any{"ticket": "acme-2", "parts": []string{"history"}}, core.Invalid)
	})
	t.Run("unknown part/cli", func(t *testing.T) {
		cliRejected(t, srv, []string{"get-ticket", "acme-2", "--part", "history"}, core.Invalid)
	})
	t.Run("unknown parameter/http", func(t *testing.T) {
		httpRejected(t, srv, httpCall{"GET", "/api/p/acme/tickets/2?parts=moves", ""}, core.Invalid)
	})
}
