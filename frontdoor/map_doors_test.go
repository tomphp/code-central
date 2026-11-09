package frontdoor_test

import (
	"context"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
)

// Submitting a Map (code-central-218).
func init() {
	cases["submit_map"] = opCase{
		ok: call{
			mcp:  map[string]any{"text": "Offline mode\n\nWorks on a plane."},
			http: httpCall{"POST", "/api/p/acme/submit_map", `{"text":"Offline mode\n\nWorks on a plane."}`},
			cli:  []string{"--text", "Offline mode\n\nWorks on a plane."},
		},
		check: func(t *testing.T, out map[string]any) {
			sp, _ := out["spec"].(map[string]any)
			acknowledgement("acme-4", "title", "labels", "body")(t, sp)
			ch, _ := out["charting"].(map[string]any)
			ticketAcknowledgement("acme-5", "Ready", "title", "state", "spec", "labels", "description", "moves", "claimable")(t, ch)
		},
		full: func(t *testing.T, out map[string]any) {
			sp, _ := out["spec"].(map[string]any)
			field("id", "acme-4")(t, sp)
			field("title", "Offline mode")(t, sp)
			labels("wayfinder:map")(t, sp)
			ch, _ := out["charting"].(map[string]any)
			field("title", "Charting: Offline mode")(t, ch)
			field("state", "Ready")(t, ch)
			labels("wayfinder:chart")(t, ch)
		},
		bad: call{
			mcp:  map[string]any{"text": "  "},
			http: httpCall{"POST", "/api/p/acme/submit_map", `{"text":"  "}`},
			cli:  []string{"--text", "  "},
		},
		code: core.Invalid,
	}
}

// submittedMap submits Map acme-4, whose Charting Ticket is acme-5.
func submittedMap(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	must(t, func() error {
		_, err := srv.Core.SubmitMap(context.Background(), "acme", core.SubmitMapInput{Text: "Offline mode"})
		return err
	})
}

// The Map block and its Ticket rows' node types are the same on every
// door.
func TestEveryDoorReadsAMap(t *testing.T) {
	srv := newServer(t, submittedMap)
	s := connect(t, srv, "acme")
	gets := map[string]map[string]any{
		"http": httpOK(t, srv, httpCall{"GET", "/api/p/acme/specs/4", ""}),
		"mcp":  mcpOK(t, s, "get_spec", map[string]any{"spec": "acme-4"}),
		"cli":  cliOK(t, srv, []string{"get-spec", "acme-4"}),
	}
	for door, out := range gets {
		t.Run(door, func(t *testing.T) {
			m, _ := out["map"].(map[string]any)
			if m == nil || m["decided"] != float64(0) || m["total"] != float64(1) || m["needs_you"] != float64(0) || m["can_write_spec"] != false {
				t.Fatalf("map %v", out["map"])
			}
			tickets, _ := out["tickets"].([]any)
			if len(tickets) != 1 {
				t.Fatalf("tickets %v", out["tickets"])
			}
			field("node_type", "chart")(t, tickets[0].(map[string]any))
		})
	}
	plain := httpOK(t, srv, httpCall{"GET", "/api/p/acme/specs/1", ""})
	if _, ok := plain["map"]; ok {
		t.Fatalf("a plain Spec has no map block: %v", plain)
	}
}
