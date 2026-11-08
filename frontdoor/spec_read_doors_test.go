package frontdoor_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/httpapi"
)

// The lighter get_spec (ticket 134): parts and sections, the same through
// every door.
func TestTheLighterGetSpecThroughEveryDoor(t *testing.T) {
	srv := newServer(t, func(t *testing.T, srv *codecentraltest.Server) {
		body := "## Solution\n\nDo it.\n\n## Testing Decisions\n\nSeams.\n"
		must(t, func() error {
			_, err := srv.Core.EditSpec(context.Background(), "acme", core.EditSpecInput{Spec: "1", Body: &body})
			return err
		})
	})
	s := connect(t, srv, "acme")
	outline := []any{
		map[string]any{"text": "Solution", "level": float64(2)},
		map[string]any{"text": "Testing Decisions", "level": float64(2)},
	}
	reads := map[string]struct {
		check func(*testing.T, map[string]any)
		calls []map[string]any
	}{
		"parts": {
			check: func(t *testing.T, out map[string]any) {
				if _, ok := out["body"]; ok {
					t.Fatalf("body returned though not asked for: %v", out)
				}
				if tickets, _ := out["tickets"].([]any); len(tickets) != 2 || out["edits"] == nil || out["title"] != "Login" {
					t.Fatalf("tickets and edits %v", out)
				}
				if !reflect.DeepEqual(out["outline"], outline) {
					t.Fatalf("outline %v", out["outline"])
				}
			},
			calls: []map[string]any{
				httpOK(t, srv, httpCall{"GET", "/api/p/acme/specs/1?part=tickets&part=edits", ""}),
				mcpOK(t, s, "get_spec", map[string]any{"spec": "acme-1", "parts": []string{"tickets", "edits"}}),
				cliOK(t, srv, []string{"get-spec", "acme-1", "--part", "tickets", "--part", "edits"}),
			},
		},
		"sections": {
			check: func(t *testing.T, out map[string]any) {
				if out["body"] != "## Testing Decisions\n\nSeams." {
					t.Fatalf("body %q", out["body"])
				}
				for _, k := range []string{"tickets", "edits"} {
					if _, ok := out[k]; ok {
						t.Fatalf("%s returned though not asked for: %v", k, out)
					}
				}
				if !reflect.DeepEqual(out["outline"], outline) {
					t.Fatalf("outline %v", out["outline"])
				}
			},
			calls: []map[string]any{
				httpOK(t, srv, httpCall{"GET", "/api/p/acme/specs/1?part=body&section=testing%20decisions", ""}),
				mcpOK(t, s, "get_spec", map[string]any{"spec": "acme-1", "parts": []string{"body"}, "sections": []string{"testing decisions"}}),
				cliOK(t, srv, []string{"get-spec", "acme-1", "--part", "body", "--section", "testing decisions"}),
			},
		},
	}
	doors := []string{"http", "mcp", "cli"}
	for name, r := range reads {
		for i, out := range r.calls {
			t.Run(name+"/"+doors[i], func(t *testing.T) { r.check(t, out) })
		}
	}

	namesTheHeadings := func(t *testing.T, e core.Error) {
		t.Helper()
		if e.Field != "sections" || !strings.Contains(e.Message, `"Missing"`) || !strings.Contains(e.Message, "Testing Decisions") {
			t.Fatalf("error %+v", e)
		}
	}
	t.Run("unknown section/http", func(t *testing.T) {
		status, b := httpDo(t, srv, httpCall{"GET", "/api/p/acme/specs/1?section=Missing", ""})
		if status != httpapi.Status(core.Invalid) {
			t.Fatalf("status %d: %s", status, b)
		}
		namesTheHeadings(t, errorJSON(t, b, core.Invalid))
	})
	t.Run("unknown section/mcp", func(t *testing.T) {
		namesTheHeadings(t, mcpRejected(t, s, "get_spec", map[string]any{"spec": "acme-1", "sections": []string{"Missing"}}, core.Invalid))
	})
	t.Run("unknown section/cli", func(t *testing.T) {
		namesTheHeadings(t, cliRejected(t, srv, []string{"get-spec", "acme-1", "--section", "Missing"}, core.Invalid))
	})
	// Other query parameters are still rejected.
	t.Run("unknown parameter/http", func(t *testing.T) {
		httpRejected(t, srv, httpCall{"GET", "/api/p/acme/specs/1?parts=body", ""}, core.Invalid)
	})
}
