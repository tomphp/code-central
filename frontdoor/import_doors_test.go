package frontdoor_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
)

// payload reads a testdata payload, for the MCP and HTTP calls; the CLI
// reads the same file with --file.
func payload(name string) map[string]any {
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		panic(err)
	}
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		panic(err)
	}
	return v
}

func body(v map[string]any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// Scratch Import (ticket 40).
func init() {
	good, bad := payload("import.json"), payload("import-bad.json")
	cases["import"] = opCase{
		ok: call{
			mcp:  map[string]any{"payload": good},
			http: httpCall{"POST", "/api/p/acme/import", body(map[string]any{"payload": good})},
			cli:  []string{"--file", "testdata/import.json"},
		},
		check: func(t *testing.T, out map[string]any) {
			specs, _ := out["specs"].([]any)
			if len(specs) != 1 || out["dry_run"] != false || !strings.Contains(out["next_step"].(string), "Delete `.scratch/`") {
				t.Fatalf("import result %v", out)
			}
			sp := specs[0].(map[string]any)
			field("id", "acme-4")(t, sp)
			tickets, _ := sp["tickets"].([]any)
			if len(tickets) != 1 || tickets[0].(map[string]any)["id"] != "acme-5" || tickets[0].(map[string]any)["state"] != "Done" {
				t.Fatalf("tickets %v", sp["tickets"])
			}
		},
		bad: call{
			mcp:  map[string]any{"payload": bad, "dry_run": true},
			http: httpCall{"POST", "/api/p/acme/import", body(map[string]any{"payload": bad, "dry_run": true})},
			cli:  []string{"--dry-run", "--file", "testdata/import-bad.json"},
		},
		code: core.Invalid,
	}
	cases["get_import_guide"] = opCase{
		ok: call{
			mcp:  map[string]any{},
			http: httpCall{"GET", "/api/p/acme/import-guide", ""},
			cli:  []string{},
		},
		check: func(t *testing.T, out map[string]any) {
			if g, _ := out["guide"].(string); !strings.Contains(g, "scratch.status") {
				t.Fatalf("guide %v", out)
			}
		},
		bad: call{
			mcp:  map[string]any{"ticket": 2},
			http: httpCall{"GET", "/api/p/acme/import-guide?ticket=2", ""},
			cli:  []string{"2"},
		},
		code:     core.Invalid,
		cliUsage: true,
	}
}

func TestTheImportGuideCommandIsGetImportGuide(t *testing.T) {
	srv := newServer(t)
	a, b := cliOK(t, srv, []string{"import-guide"}), cliOK(t, srv, []string{"get-import-guide"})
	if a["guide"] == nil || a["guide"] != b["guide"] {
		t.Fatal("code-central import-guide should print get_import_guide")
	}
}

func TestAnImportOverHTTPWritesAsTheAgent(t *testing.T) {
	srv := newServer(t)
	httpOK(t, srv, cases["import"].ok.http) // the suite's HTTP door sends the human Actor
	tk := httpOK(t, srv, httpCall{"GET", "/api/p/acme/tickets/5", ""})
	moves, _ := tk["moves"].([]any)
	if len(moves) != 1 {
		t.Fatalf("moves %v", tk["moves"])
	}
	m := moves[0].(map[string]any)
	if m["actor"] != "agent" || m["via"] != "import" || m["from"] != nil {
		t.Fatalf("move %v", m)
	}
}
