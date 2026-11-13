package frontdoor_test

import (
	"testing"

	"github.com/tomoram/code-central/core"
)

// Submitting a Small Change (code-central-479). The Project's Small Changes
// Spec is acme-4, so the Ticket is acme-5.
func init() {
	cases["submit_small_change"] = opCase{
		ok: call{
			mcp:  map[string]any{"text": "Fix the typo\n\nIn the footer."},
			http: httpCall{"POST", "/api/p/acme/submit_small_change", `{"text":"Fix the typo\n\nIn the footer."}`},
			cli:  []string{"--text", "Fix the typo\n\nIn the footer."},
		},
		check: ticketAcknowledgement("acme-5", "Ready", "title", "state", "spec", "description", "moves", "claimable"),
		full: func(t *testing.T, out map[string]any) {
			field("id", "acme-5")(t, out)
			field("title", "Fix the typo")(t, out)
			field("state", "Ready")(t, out)
			sp, _ := out["spec"].(map[string]any)
			field("id", "acme-4")(t, sp)
			field("title", "Small Changes")(t, sp)
			field("description", "Fix the typo\n\nIn the footer.")(t, out)
			labels()(t, out)
		},
		bad: call{
			mcp:  map[string]any{"text": "  "},
			http: httpCall{"POST", "/api/p/acme/submit_small_change", `{"text":"  "}`},
			cli:  []string{"--text", "  "},
		},
		code: core.Invalid,
	}
}
