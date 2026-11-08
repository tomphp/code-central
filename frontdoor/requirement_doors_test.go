package frontdoor_test

import (
	"testing"

	"github.com/tomoram/code-central/core"
)

// Submitting a Requirement (code-central-67).
func init() {
	cases["submit_requirement"] = opCase{
		ok: call{
			mcp:  map[string]any{"text": "Dark mode\n\nA toggle in settings."},
			http: httpCall{"POST", "/api/p/acme/submit_requirement", `{"text":"Dark mode\n\nA toggle in settings."}`},
			cli:  []string{"--text", "Dark mode\n\nA toggle in settings."},
		},
		check: func(t *testing.T, out map[string]any) {
			sp, _ := out["spec"].(map[string]any)
			field("id", "acme-4")(t, sp)
			field("title", "Dark mode")(t, sp)
			stages, _ := out["stages"].([]any)
			if len(stages) != 3 {
				t.Fatalf("stages %v", out["stages"])
			}
			for i, want := range []string{"stage:grilling", "stage:spec", "stage:tickets"} {
				st := stages[i].(map[string]any)
				field("state", "Ready")(t, st)
				labels(want)(t, st)
			}
		},
		bad: call{
			mcp:  map[string]any{"text": "  "},
			http: httpCall{"POST", "/api/p/acme/submit_requirement", `{"text":"  "}`},
			cli:  []string{"--text", "  "},
		},
		code: core.Invalid,
	}
}

func TestTheSubmitRequirementCommandReadsTheTextFromAFile(t *testing.T) {
	srv := newServer(t)
	out := cliOK(t, srv, []string{"submit-requirement", "--file", "testdata/requirement.md"})
	sp, _ := out["spec"].(map[string]any)
	field("title", "Export to CSV")(t, sp)
}
