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
		for _, k := range []string{"body", "description", "edits", "comments", "moves", "claims", "rounds"} {
			if _, ok := out[k]; ok {
				t.Fatalf("acknowledgement %v holds the record's %s", out, k)
			}
		}
	}
}

// ticketAcknowledgement is acknowledgement for a Ticket write, which
// also gives the Ticket's State after it.
func ticketAcknowledgement(id, state string, changed ...string) func(*testing.T, map[string]any) {
	return func(t *testing.T, out map[string]any) {
		t.Helper()
		acknowledgement(id, changed...)(t, out)
		field("state", state)(t, out)
	}
}

// stopAcknowledgement is ticketAcknowledgement for acme-3 after an action
// that ends the agent's work, which also gives its next step.
func stopAcknowledgement(state string, changed ...string) func(*testing.T, map[string]any) {
	return func(t *testing.T, out map[string]any) {
		t.Helper()
		ticketAcknowledgement("acme-3", state, changed...)(t, out)
		field("next_step", "stop working this Ticket")(t, out)
		if _, ok := out["ticket"]; ok {
			t.Fatalf("acknowledgement %v holds the Ticket", out)
		}
	}
}

// writes are the operations that acknowledge: the cases with a check on
// their call with full.
func writes() []string {
	var out []string
	for op, c := range cases {
		if c.full != nil {
			out = append(out, op)
		}
	}
	slices.Sort(out)
	return out
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

func TestWritesAcknowledgeTheSameOnEveryDoor(t *testing.T) {
	for _, op := range writes() {
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

func TestWritesWithFullReturnTheRecordOnEveryDoor(t *testing.T) {
	for _, op := range writes() {
		t.Run(op, func(t *testing.T) {
			c := cases[op]
			for door, out := range onEveryDoor(t, op, c.setup, withFull(t, c.ok)) {
				t.Run(door, func(t *testing.T) { c.full(t, out) })
			}
		})
	}
}

// TestEveryToolWithFullIsAWrite keeps a tool that offers full from
// missing the two tests above: its case must check its call with full.
func TestEveryToolWithFullIsAWrite(t *testing.T) {
	s := connect(t, newServer(t), "acme")
	for tool, err := range s.Tools(t.Context(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		schema, _ := json.Marshal(tool.InputSchema)
		if strings.Contains(string(schema), `"full"`) && !slices.Contains(writes(), tool.Name) {
			t.Errorf("tool %s offers full but its case has no full check", tool.Name)
		}
	}
}

func TestTheWriteToolsOfferFull(t *testing.T) {
	s := connect(t, newServer(t), "acme")
	for tool, err := range s.Tools(t.Context(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(writes(), tool.Name) {
			continue
		}
		schema, _ := json.Marshal(tool.InputSchema)
		if !strings.Contains(string(schema), `"full"`) || !strings.Contains(tool.Description, "full") {
			t.Errorf("tool %s doesn't offer full: %s; %s", tool.Name, tool.Description, schema)
		}
	}
}
