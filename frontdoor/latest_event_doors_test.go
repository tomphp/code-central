package frontdoor_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
)

// The latest Session Event on reads (ticket 107): computed on read, the
// same through every door.
func TestLatestEventThroughEveryDoor(t *testing.T) {
	srv := newServer(t, func(t *testing.T, srv *codecentraltest.Server) {
		ctx := context.Background()
		must(t, func() error {
			_, err := srv.Core.Claim(ctx, "acme", core.ClaimInput{SessionID: "s1"})
			return err
		})
		// acme-2 is claimed too, but posts nothing.
		must(t, func() error { _, err := srv.Core.Approve(ctx, "acme", core.TicketInput{Ticket: "2"}); return err })
		must(t, func() error {
			_, err := srv.Core.Claim(ctx, "acme", core.ClaimInput{SessionID: "s2"})
			return err
		})
		srv.Core.PostSessionEvents(ctx, "s1", []core.SessionEvent{
			{Time: time.Date(2026, 11, 7, 9, 0, 4, 0, time.UTC), Kind: "text", Text: "Planning."},
			{Time: time.Date(2026, 11, 7, 9, 0, 5, 0, time.UTC), Kind: "tool_call", Text: "Bash: go test ./...", Tool: "Bash", ToolUseID: "tu1"},
		})
	})
	want := map[string]any{"kind": "tool_call", "text": "Bash: go test ./...", "time": "2026-11-07T09:00:05Z"}
	s := connect(t, srv, "acme")

	reads := map[string]struct {
		check func(*testing.T, map[string]any)
		calls []map[string]any
		// doors names the calls' doors, when not http, mcp and cli.
		doors []string
	}{
		"get_ticket": {
			check: func(t *testing.T, out map[string]any) {
				claims := out["claims"].([]any)
				if got := claims[0].(map[string]any)["latest_event"]; !reflect.DeepEqual(got, want) {
					t.Fatalf("Claim's latest_event %v, want %v", got, want)
				}
			},
			calls: []map[string]any{
				httpOK(t, srv, httpCall{"GET", "/api/p/acme/tickets/3", ""}),
				mcpOK(t, s, "get_ticket", map[string]any{"ticket": "acme-3", "parts": []string{"claims"}}),
				cliOK(t, srv, []string{"get-ticket", "acme-3", "--part", "claims"}),
			},
		},
		// An agent's default read leaves it out and counts it
		// (code-central-320).
		"get_ticket by default": {
			check: func(t *testing.T, out map[string]any) {
				if got, ok := out["claims"].([]any)[0].(map[string]any)["latest_event"]; ok {
					t.Fatalf("the default read has latest_event %v", got)
				}
				if lo := out["left_out"].(map[string]any); lo["claims"] != float64(1) {
					t.Fatalf("left_out %v, want claims 1", lo)
				}
			},
			calls: []map[string]any{
				mcpOK(t, s, "get_ticket", map[string]any{"ticket": "acme-3"}),
				cliOK(t, srv, []string{"get-ticket", "acme-3"}),
			},
			doors: []string{"mcp", "cli"},
		},
		"get_ticket with none": {
			check: func(t *testing.T, out map[string]any) {
				claims := out["claims"].([]any)
				if got, ok := claims[0].(map[string]any)["latest_event"]; ok {
					t.Fatalf("a Claim with no Session Events has latest_event %v", got)
				}
			},
			calls: []map[string]any{
				httpOK(t, srv, httpCall{"GET", "/api/p/acme/tickets/2", ""}),
				mcpOK(t, s, "get_ticket", map[string]any{"ticket": "acme-2"}),
				cliOK(t, srv, []string{"get-ticket", "acme-2"}),
			},
		},
		"find_tickets": {
			check: func(t *testing.T, out map[string]any) { rowsHaveLatestEvent(t, out["rows"], want) },
			calls: []map[string]any{
				httpOK(t, srv, httpCall{"GET", "/api/p/acme/tickets", ""}),
				mcpOK(t, s, "find_tickets", map[string]any{}),
				cliOK(t, srv, []string{"find-tickets"}),
			},
		},
		"get_spec": {
			check: func(t *testing.T, out map[string]any) { rowsHaveLatestEvent(t, out["tickets"], want) },
			calls: []map[string]any{
				httpOK(t, srv, httpCall{"GET", "/api/p/acme/specs/1", ""}),
				mcpOK(t, s, "get_spec", map[string]any{"spec": "acme-1"}),
				cliOK(t, srv, []string{"get-spec", "acme-1"}),
			},
		},
	}
	for name, r := range reads {
		doors := r.doors
		if doors == nil {
			doors = []string{"http", "mcp", "cli"}
		}
		for i, out := range r.calls {
			t.Run(name+"/"+doors[i], func(t *testing.T) { r.check(t, out) })
		}
	}
}

// rowsHaveLatestEvent checks Ticket rows: acme-3, claimed and posting,
// carries want; acme-2, claimed but silent, reads as it did before, with
// none.
func rowsHaveLatestEvent(t *testing.T, rows any, want map[string]any) {
	t.Helper()
	seen := 0
	for _, r := range rows.([]any) {
		row := r.(map[string]any)
		got, ok := row["latest_event"]
		switch row["id"] {
		case "acme-3":
			seen++
			if !reflect.DeepEqual(got, want) {
				t.Errorf("acme-3 row's latest_event %v, want %v", got, want)
			}
		case "acme-2":
			seen++
			if ok {
				t.Errorf("acme-2 has no Session Events but its row has latest_event %v", got)
			}
		}
	}
	if seen != 2 {
		t.Fatalf("rows %v, want acme-2 and acme-3", rows)
	}
}
