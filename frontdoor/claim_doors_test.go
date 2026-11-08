package frontdoor_test

import (
	"testing"

	"github.com/tomoram/code-central/core"
)

// A session claiming the Ticket it already holds gets the same Claim back
// on every door; another session is told who holds it.
func TestTheHoldingSessionClaimsAgainOnEveryDoor(t *testing.T) {
	const held = "acme-3 is held by Claim 1 of another session; claim another Ticket, or ask the human to release it"
	for door, claim := range map[string]func(t *testing.T) (first, again map[string]any, other core.Error){
		"mcp": func(t *testing.T) (map[string]any, map[string]any, core.Error) {
			s := connect(t, newServer(t), "acme")
			args := map[string]any{"ticket": "acme-3", "session_id": "sess-1"}
			return mcpOK(t, s, "claim", args), mcpOK(t, s, "claim", args),
				mcpRejected(t, s, "claim", map[string]any{"ticket": "acme-3", "session_id": "sess-2"}, core.ActionNotAllowed)
		},
		"http": func(t *testing.T) (map[string]any, map[string]any, core.Error) {
			srv := newServer(t)
			c := httpCall{"POST", "/api/p/acme/claim", `{"ticket":"acme-3","session_id":"sess-1"}`}
			return httpOK(t, srv, c), httpOK(t, srv, c),
				httpRejected(t, srv, httpCall{"POST", "/api/p/acme/claim", `{"ticket":"acme-3","session_id":"sess-2"}`}, core.ActionNotAllowed)
		},
		"cli": func(t *testing.T) (map[string]any, map[string]any, core.Error) {
			srv := newServer(t)
			args := []string{"claim", "--ticket", "acme-3", "--session", "sess-1"}
			return cliOK(t, srv, args), cliOK(t, srv, args),
				cliRejected(t, srv, []string{"claim", "--ticket", "acme-3", "--session", "sess-2"}, core.ActionNotAllowed)
		},
	} {
		t.Run(door, func(t *testing.T) {
			first, again, other := claim(t)
			if id, _ := first["claim_id"].(string); id == "" || again["claim_id"] != id {
				t.Fatalf("claim_id %v, then %v; want the same Claim ID back", first["claim_id"], again["claim_id"])
			}
			if tk, _ := again["ticket"].(map[string]any); tk["id"] != "acme-3" || tk["state"] != "In Progress" || again["stage_guide"] == nil {
				t.Fatalf("re-claim %v, want acme-3 In Progress with its Stage Guide", again)
			}
			if other.Message != held {
				t.Fatalf("message %q, want %q", other.Message, held)
			}
		})
	}
}
