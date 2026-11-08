package frontdoor_test

import (
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
)

// Stage Guides (code-central-73).
func init() {
	cases["get_stage_guide"] = opCase{
		ok: call{
			mcp:  map[string]any{"stage": "tickets"},
			http: httpCall{"GET", "/api/stage-guides/tickets", ""},
			cli:  []string{"tickets"},
		},
		check: func(t *testing.T, out map[string]any) {
			field("stage", "tickets")(t, out)
			if g, _ := out["guide"].(string); !strings.Contains(g, "# Stage Guide: tickets") {
				t.Fatalf("guide %v", out)
			}
		},
		bad: call{
			mcp:  map[string]any{"stage": "build"},
			http: httpCall{"GET", "/api/stage-guides/build", ""},
			cli:  []string{"build"},
		},
		code: core.Invalid,
	}
}

// claim's response carries the guide on every door: the implementation
// guide for acme-3, which has no Stage Label, and a Stage's own guide for
// a Stage Ticket.
func TestClaimCarriesTheStageGuideOnEveryDoor(t *testing.T) {
	for door, claim := range map[string]func(t *testing.T) (implementation, grilling map[string]any){
		"mcp": func(t *testing.T) (map[string]any, map[string]any) {
			srv := newServer(t)
			s := connect(t, srv, "acme")
			a := mcpOK(t, s, "claim", map[string]any{"ticket": "acme-3"})
			submittedRequirement(t, srv)
			return a, mcpOK(t, s, "claim", map[string]any{"ticket": "acme-5"})
		},
		"http": func(t *testing.T) (map[string]any, map[string]any) {
			srv := newServer(t)
			a := httpOK(t, srv, httpCall{"POST", "/api/p/acme/claim", `{"ticket":"acme-3"}`})
			submittedRequirement(t, srv)
			return a, httpOK(t, srv, httpCall{"POST", "/api/p/acme/claim", `{"ticket":"acme-5"}`})
		},
		"cli": func(t *testing.T) (map[string]any, map[string]any) {
			srv := newServer(t)
			a := cliOK(t, srv, []string{"claim", "--ticket", "acme-3"})
			submittedRequirement(t, srv)
			return a, cliOK(t, srv, []string{"claim", "--ticket", "acme-5"})
		},
	} {
		t.Run(door, func(t *testing.T) {
			impl, grill := claim(t)
			for want, out := range map[string]map[string]any{"implementation": impl, "grilling": grill} {
				g, _ := out["stage_guide"].(map[string]any)
				if g == nil || g["stage"] != want || !strings.Contains(g["guide"].(string), "# Stage Guide: "+want) {
					t.Errorf("claim carried stage_guide %v, want the %s guide", out["stage_guide"], want)
				}
			}
		})
	}
}
