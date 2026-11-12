package frontdoor_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/httpapi"
)

// Stage Guides (ticket 73).
func init() {
	cases["get_stage_guide"] = opCase{
		ok: call{
			mcp:  map[string]any{"stage": "tickets"},
			http: httpCall{"GET", "/api/p/acme/stage-guides/tickets", ""},
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
			http: httpCall{"GET", "/api/p/acme/stage-guides/build", ""},
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

// Subagent Exploration (code-central-441): set_subagent_exploration is the
// human's, so HTTP only.
func init() {
	cases["set_subagent_exploration"] = opCase{
		ok: call{http: setSubagentExploration(`{"on":false}`)},
		check: func(t *testing.T, out map[string]any) {
			acknowledgement("acme", "stage_guides")(t, out)
		},
		full: func(t *testing.T, out map[string]any) {
			t.Helper()
			sg, _ := out["stage_guides"].(map[string]any)
			if sg["subagent_exploration"] != false {
				t.Fatalf("stage_guides = %v, want Subagent Exploration off", out["stage_guides"])
			}
		},
		bad:  call{http: setSubagentExploration(`{"on":"no"}`)},
		code: core.Invalid,
	}
}

func setSubagentExploration(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/set_subagent_exploration", body}
}

func TestSetSubagentExplorationIsRejectedForAnAgent(t *testing.T) {
	srv := newServer(t)
	status, b := httpDoAs(t, srv, core.Agent, setSubagentExploration(`{"on":false}`))
	if e := errorJSON(t, b, core.ActionNotAllowed); status != httpapi.Status(core.ActionNotAllowed) || !strings.Contains(e.Message, "human") {
		t.Fatalf("status %d, want %d; message %q", status, httpapi.Status(core.ActionNotAllowed), e.Message)
	}
	sg, _ := httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""})["stage_guides"].(map[string]any)
	if sg["subagent_exploration"] != true {
		t.Fatalf("stage_guides = %v, want Subagent Exploration still on", sg)
	}
}

func TestEveryProjectReadCarriesSubagentExplorationOn(t *testing.T) {
	srv := newServer(t)
	for _, p := range []map[string]any{httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""})} {
		if sg, _ := p["stage_guides"].(map[string]any); sg["subagent_exploration"] != true {
			t.Errorf("stage_guides = %v, want Subagent Exploration on", p["stage_guides"])
		}
	}
}

func TestClaimWithSubagentExplorationOffCarriesAGuideWithoutTheLineOnEveryDoor(t *testing.T) {
	const line = "ask an Explore subagent"
	for door, claim := range map[string]func(srv *codecentraltest.Server) map[string]any{
		"mcp": func(srv *codecentraltest.Server) map[string]any {
			return mcpOK(t, connect(t, srv, "acme"), "claim", map[string]any{"ticket": "acme-3"})
		},
		"http": func(srv *codecentraltest.Server) map[string]any {
			return httpOK(t, srv, httpCall{"POST", "/api/p/acme/claim", `{"ticket":"acme-3"}`})
		},
		"cli": func(srv *codecentraltest.Server) map[string]any {
			return cliOK(t, srv, []string{"claim", "--ticket", "acme-3"})
		},
	} {
		t.Run(door, func(t *testing.T) {
			for _, on := range []bool{true, false} {
				srv := newServer(t)
				httpOK(t, srv, setSubagentExploration(fmt.Sprintf(`{"on":%v}`, on)))
				g, _ := claim(srv)["stage_guide"].(map[string]any)
				if g == nil || g["stage"] != "implementation" || strings.Contains(g["guide"].(string), line) != on {
					t.Errorf("with Subagent Exploration %v, claim carried %v", on, g)
				}
			}
		})
	}
}
