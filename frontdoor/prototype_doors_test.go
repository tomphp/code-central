package frontdoor_test

import (
	"testing"

	"github.com/tomoram/code-central/core"
)

// Submitting a Prototype (code-central-221).
func init() {
	cases["submit_prototype"] = opCase{
		ok: call{
			mcp:  map[string]any{"spec": "acme-1", "text": "Theme picker\n\nPicker or toggle?"},
			http: httpCall{"POST", "/api/p/acme/submit_prototype", `{"spec":"acme-1","text":"Theme picker\n\nPicker or toggle?"}`},
			cli:  []string{"acme-1", "--text", "Theme picker\n\nPicker or toggle?"},
		},
		check: ticketAcknowledgement("acme-4", "Ready", "title", "state", "spec", "labels", "description", "moves", "claimable"),
		full: func(t *testing.T, out map[string]any) {
			field("id", "acme-4")(t, out)
			field("title", "Prototype: Theme picker")(t, out)
			field("state", "Ready")(t, out)
			field("description", "Theme picker\n\nPicker or toggle?")(t, out)
			labels("prototype")(t, out)
		},
		bad: call{
			mcp:  map[string]any{"spec": "acme-1", "text": "  "},
			http: httpCall{"POST", "/api/p/acme/submit_prototype", `{"spec":"acme-1","text":"  "}`},
			cli:  []string{"acme-1", "--text", "  "},
		},
		code: core.Invalid,
	}
}
