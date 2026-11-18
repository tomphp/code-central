package server_test

import (
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/httpapi"
)

// Auto-fix (Spec code-central-1022): set_auto_fix is the human's, so HTTP
// only.
func init() {
	cases["set_auto_fix"] = opCase{
		ok: call{http: setAutoFix(`{"on":false,"fix_instructions":"Fix it.","limit":5}`)},
		check: func(t *testing.T, out map[string]any) {
			acknowledgement("acme", "ci")(t, out)
		},
		full: func(t *testing.T, out map[string]any) {
			t.Helper()
			c, _ := out["ci"].(map[string]any)
			af, _ := c["auto_fix"].(map[string]any)
			if af["on"] != false || af["fix_instructions"] != "Fix it." || af["limit"] != float64(5) {
				t.Fatalf("ci.auto_fix %v, want off, Fix it. and 5", c["auto_fix"])
			}
		},
		bad:  call{http: setAutoFix(`{"on":false,"fix_instructions":"Fix it.","limit":0}`)},
		code: core.Invalid,
	}
}

func setAutoFix(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/set_auto_fix", body}
}

func TestSetAutoFixIsRejectedForAnAgentAndAnIntegration(t *testing.T) {
	srv := newServer(t)
	for _, actor := range []core.Actor{core.Agent, core.Integration} {
		status, b := httpDoAs(t, srv, actor, setAutoFix(`{"on":false,"fix_instructions":"Fix it.","limit":5}`))
		if e := errorJSON(t, b, core.ActionNotAllowed); status != httpapi.Status(core.ActionNotAllowed) || !strings.Contains(e.Message, "human") {
			t.Fatalf("as %s: status %d, want %d; message %q", actor, status, httpapi.Status(core.ActionNotAllowed), e.Message)
		}
	}
}
