package frontdoor_test

import (
	"context"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
)

// Blockers, Replacements and finding work (ticket 28).
func init() {
	for name, c := range map[string]opCase{
		"add_blocker": {
			ok: call{
				mcp:  map[string]any{"ticket": "acme-2", "blocker": 3},
				http: httpCall{"POST", "/api/p/acme/tickets/2/add_blocker", `{"blocker":"acme-3"}`},
				cli:  []string{"acme-2", "acme-3"},
			},
			check: ticketAcknowledgement("acme-2", "Draft", "blocked_by", "blockers", "blocked"),
			full:  ids("blocked_by", "acme-3"),
			bad: call{
				mcp:  map[string]any{"ticket": 3, "blocker": 3},
				http: httpCall{"POST", "/api/p/acme/tickets/3/add_blocker", `{"blocker":3}`},
				cli:  []string{"3", "3"},
			},
			code: core.Cycle,
		},
		"remove_blocker": {
			setup: blockedDraft,
			ok: call{
				mcp:  map[string]any{"ticket": "acme-2", "blocker": "acme-3"},
				http: httpCall{"POST", "/api/p/acme/tickets/2/remove_blocker", `{"blocker":"acme-3"}`},
				cli:  []string{"acme-2", "acme-3"},
			},
			check: ticketAcknowledgement("acme-2", "Draft", "blocked_by", "blockers", "blocked"),
			full:  ids("blocked_by"),
			bad: call{
				mcp:  map[string]any{"ticket": 2, "blocker": 9},
				http: httpCall{"POST", "/api/p/acme/tickets/2/remove_blocker", `{"blocker":9}`},
				cli:  []string{"2", "9"},
			},
			code: core.Invalid,
		},
		"cancel": {
			setup: readyTicket,
			ok: call{
				mcp:  map[string]any{"ticket": "acme-3", "replacements": []any{"acme-4"}, "reason": "Superseded."},
				http: httpCall{"POST", "/api/p/acme/tickets/3/cancel", `{"replacements":[4],"reason":"Superseded."}`},
				cli:  []string{"acme-3", "acme-4", "--reason", "Superseded."},
			},
			check: ticketAcknowledgement("acme-3", "Cancelled", "state", "replacements", "comments", "moves", "claimable"),
			full: func(t *testing.T, out map[string]any) {
				field("state", "Cancelled")(t, out)
				ids("replacements", "acme-4")(t, out)
			},
			bad: call{
				mcp:  map[string]any{"ticket": 2, "replacements": []any{9}},
				http: httpCall{"POST", "/api/p/acme/tickets/2/cancel", `{"replacements":[9]}`},
				cli:  []string{"2", "9"},
			},
			code: core.Invalid,
		},
		"get_spec": {
			ok: call{
				mcp:  map[string]any{"spec": "acme-1"},
				http: httpCall{"GET", "/api/p/acme/specs/1", ""},
				cli:  []string{"acme-1"},
			},
			check: func(t *testing.T, out map[string]any) {
				field("title", "Login")(t, out)
				summary, _ := out["state_summary"].(map[string]any)
				if summary["Draft"] != float64(1) || summary["Ready"] != float64(1) || len(summary) != 2 {
					t.Fatalf("state_summary %v", out["state_summary"])
				}
				if tickets, _ := out["tickets"].([]any); len(tickets) != 2 {
					t.Fatalf("tickets %v", out["tickets"])
				}
			},
			bad: call{
				mcp:  map[string]any{"spec": 9},
				http: httpCall{"GET", "/api/p/acme/specs/9", ""},
				cli:  []string{"9"},
			},
			code: core.NotFound,
		},
		"find_specs": {
			ok: call{
				mcp:  map[string]any{"title": "log", "finished": true},
				http: httpCall{"GET", "/api/p/acme/specs?title=log&finished=true", ""},
				cli:  []string{"--title", "log", "--finished"},
			},
			check: func(t *testing.T, out map[string]any) {
				rows, _ := out["rows"].([]any)
				if len(rows) != 1 || rows[0].(map[string]any)["id"] != "acme-1" {
					t.Fatalf("rows %v", out["rows"])
				}
			},
			bad: call{
				mcp:  map[string]any{"tags": map[string]any{"Bad Key": "x"}},
				http: httpCall{"GET", "/api/p/acme/specs?tag=Bad+Key%3Dx", ""},
				cli:  []string{"--tag", "Bad Key=x"},
			},
			code: core.Invalid,
		},
	} {
		cases[name] = c
	}
}

// blockedDraft makes Draft acme-2 wait on acme-3.
func blockedDraft(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	must(t, func() error {
		_, err := srv.Core.AddBlocker(context.Background(), "acme", core.BlockerInput{Ticket: "2", Blocker: "3"})
		return err
	})
}

// ids checks a result's list of IDs is exactly want.
func ids(name string, want ...string) func(*testing.T, map[string]any) {
	return func(t *testing.T, out map[string]any) {
		t.Helper()
		got, ok := out[name].([]any)
		if !ok || len(got) != len(want) {
			t.Fatalf("%s = %v, want %v", name, out[name], want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s = %v, want %v", name, out[name], want)
			}
		}
	}
}

func TestBlockedByTagAndClaimableReachEveryDoor(t *testing.T) {
	tagged := func(t *testing.T, srv *codecentraltest.Server) {
		t.Helper()
		must(t, func() error {
			_, err := srv.Core.SetTags(context.Background(), "acme", core.SetTagsInput{Target: "3", Tags: map[string]string{"area": "web"}})
			return err
		})
	}
	oneRow := func(t *testing.T, out map[string]any) {
		t.Helper()
		rows, _ := out["rows"].([]any)
		if len(rows) != 1 || rows[0].(map[string]any)["id"] != "acme-3" {
			t.Fatalf("rows %v, want only acme-3", out["rows"])
		}
	}
	created := func(t *testing.T, out map[string]any) {
		t.Helper()
		ids("blocked_by", "acme-3")(t, out)
		if out["blocked"] != true {
			t.Fatalf("blocked = %v", out["blocked"])
		}
	}
	t.Run("http", func(t *testing.T) {
		srv := newServer(t, tagged)
		oneRow(t, httpOK(t, srv, httpCall{"GET", "/api/p/acme/tickets?tag=area%3Dweb&claimable=true", ""}))
		created(t, httpOK(t, srv, httpCall{"POST", "/api/p/acme/create_ticket", `{"spec":1,"title":"Next","blocked_by":["acme-3"],"full":true}`}))
	})
	t.Run("mcp", func(t *testing.T) {
		srv := newServer(t, tagged)
		s := connect(t, srv, "acme")
		oneRow(t, mcpOK(t, s, "find_tickets", map[string]any{"tags": map[string]any{"area": "web"}, "claimable": true}))
		created(t, mcpOK(t, s, "create_ticket", map[string]any{"spec": 1, "title": "Next", "blocked_by": []any{3}, "full": true}))
	})
	t.Run("cli", func(t *testing.T) {
		srv := newServer(t, tagged)
		oneRow(t, cliOK(t, srv, []string{"find-tickets", "--tag", "area=web", "--claimable"}))
		created(t, cliOK(t, srv, []string{"create-ticket", "--spec", "1", "--title", "Next", "--blocked-by", "acme-3", "--full"}))
	})
}
