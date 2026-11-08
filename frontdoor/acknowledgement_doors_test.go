package frontdoor_test

import (
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
)

// Acknowledgements (ADR 0009): a write that used to echo the whole record
// returns an Acknowledgement on every door, and the record with full.

// acknowledgement checks a result is the Acknowledgement of id, changing
// exactly changed, and holds none of the record's values.
func acknowledgement(id string, changed ...string) func(*testing.T, map[string]any) {
	return func(t *testing.T, out map[string]any) {
		t.Helper()
		field("id", id)(t, out)
		got := []string{}
		for _, c := range out["changed"].([]any) {
			got = append(got, c.(string))
		}
		if !slices.Equal(got, changed) || out["updated"] == nil {
			t.Fatalf("acknowledgement %v, want changed %v and updated", out, changed)
		}
		for _, k := range []string{"body", "description", "edits", "comments", "moves"} {
			if _, ok := out[k]; ok {
				t.Fatalf("acknowledgement %v holds the record's %s", out, k)
			}
		}
	}
}

// specWrites are the Spec writes that acknowledge, with the check each
// one's call with full makes on the record.
var specWrites = map[string]func(*testing.T, map[string]any){
	"create_spec": func(t *testing.T, out map[string]any) {
		field("id", "acme-4")(t, out)
		field("body", "Find things.")(t, out)
	},
	"edit_spec": func(t *testing.T, out map[string]any) {
		field("title", "Sign in")(t, out)
		field("body", "New.")(t, out)
		if edits, _ := out["edits"].([]any); len(edits) != 1 {
			t.Fatalf("edits %v", out["edits"])
		}
	},
	"create_linked_spec": func(t *testing.T, out map[string]any) {
		field("id", "acme-4")(t, out)
		field("external_state", "open")(t, out["link"].(map[string]any))
	},
	"update_linked_spec": field("title", "Epic 2"),
}

// withFull is c asking for the record: full on MCP, "full": true in the
// HTTP body and --full on the CLI.
func withFull(t *testing.T, c call) call {
	t.Helper()
	body := map[string]any{}
	if err := json.Unmarshal([]byte(c.http.body), &body); err != nil {
		t.Fatal(err)
	}
	body["full"] = true
	b, _ := json.Marshal(body)
	out := call{http: httpCall{c.http.method, c.http.path, string(b)}}
	if c.mcp != nil {
		out.mcp = maps.Clone(c.mcp)
		out.mcp["full"] = true
		out.cli = append(slices.Clone(c.cli), "--full")
	}
	return out
}

// onEveryDoor runs c on each door that exposes op, each against a fresh
// fixture, returning each door's result.
func onEveryDoor(t *testing.T, op string, setup func(*testing.T, *codecentraltest.Server), c call) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{"http": httpOK(t, newServer(t, setup), c.http)}
	if c.mcp != nil {
		out["mcp"] = mcpOK(t, connect(t, newServer(t, setup), "acme"), op, c.mcp)
		out["cli"] = cliOK(t, newServer(t, setup), append([]string{strings.ReplaceAll(op, "_", "-")}, c.cli...))
	}
	return out
}

func TestSpecWritesAcknowledgeTheSameOnEveryDoor(t *testing.T) {
	for op := range specWrites {
		t.Run(op, func(t *testing.T) {
			c := cases[op]
			results := onEveryDoor(t, op, c.setup, c.ok)
			for door, out := range results {
				c.check(t, out)
				if !reflect.DeepEqual(out, results["http"]) {
					t.Fatalf("%s acknowledged %v, HTTP %v", door, out, results["http"])
				}
			}
		})
	}
}

func TestSpecWritesWithFullReturnTheSpecOnEveryDoor(t *testing.T) {
	for op, check := range specWrites {
		t.Run(op, func(t *testing.T) {
			c := cases[op]
			for door, out := range onEveryDoor(t, op, c.setup, withFull(t, c.ok)) {
				t.Run(door, func(t *testing.T) { check(t, out) })
			}
		})
	}
}

func TestTheSpecWriteToolsOfferFull(t *testing.T) {
	s := connect(t, newServer(t), "acme")
	for tool, err := range s.Tools(t.Context(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := specWrites[tool.Name]; !ok {
			continue
		}
		schema, _ := json.Marshal(tool.InputSchema)
		if !strings.Contains(string(schema), `"full"`) || !strings.Contains(tool.Description, "full") {
			t.Errorf("tool %s doesn't offer full: %s; %s", tool.Name, tool.Description, schema)
		}
	}
}
