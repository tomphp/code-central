package frontdoor_test

import (
	"testing"

	"github.com/tomoram/code-central/core"
)

// Silent Claims, sessions and release (ticket 31).
func init() {
	for name, c := range map[string]opCase{
		"set_session": {
			setup: claimedTicket,
			ok: call{
				mcp:  map[string]any{"ticket": "acme-3", "claim_id": "claim-1", "session_id": "sess-2", "working_dir": "/w", "resume_command": "agent resume sess-2"},
				http: httpCall{"POST", "/api/p/acme/tickets/3/set_session", `{"claim_id":"claim-1","session_id":"sess-2","working_dir":"/w","resume_command":"agent resume sess-2"}`},
				cli:  []string{"acme-3", "--claim", "claim-1", "--session", "sess-2", "--working-dir", "/w", "--resume-command", "agent resume sess-2"},
			},
			check: func(t *testing.T, out map[string]any) {
				claims, _ := out["claims"].([]any)
				if len(claims) != 1 {
					t.Fatalf("claims %v", out["claims"])
				}
				claim := claims[0].(map[string]any)
				field("session_id", "sess-2")(t, claim)
				field("working_dir", "/w")(t, claim)
				field("resume_command", "agent resume sess-2")(t, claim)
			},
			bad: call{
				mcp:  map[string]any{"ticket": "acme-3", "claim_id": "claim-9", "session_id": "sess-2"},
				http: httpCall{"POST", "/api/p/acme/tickets/3/set_session", `{"claim_id":"claim-9","session_id":"sess-2"}`},
				cli:  []string{"acme-3", "--claim", "claim-9", "--session", "sess-2"},
			},
			code: core.StaleClaim,
		},
		"release": {
			setup: claimedTicket,
			ok:    call{http: httpCall{"POST", "/api/p/acme/tickets/3/release", `{"note":"Session died."}`}},
			check: field("state", "Ready"),
			bad:   call{http: httpCall{"POST", "/api/p/acme/tickets/2/release", `{}`}},
			code:  core.ActionNotAllowed,
		},
	} {
		cases[name] = c
	}
}
