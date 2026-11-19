package server_test

import (
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/httpapi"
)

// Auto-accept (Spec code-central-536): set_auto_accept is the human's, so
// HTTP only, and a Project reads every kind off until it is set.
func init() {
	cases["set_auto_accept"] = opCase{
		ok: call{http: setAutoAccept(`{"kinds":{"implementation":true}}`)},
		check: func(t *testing.T, out map[string]any) {
			acknowledgement("acme", "auto_accept")(t, out)
		},
		full: func(t *testing.T, out map[string]any) {
			t.Helper()
			aa, _ := out["auto_accept"].(map[string]any)
			if aa["implementation"] != true || aa["grilling"] != false || aa["merge"] != false || len(aa) != 5 {
				t.Fatalf("auto_accept %v, want implementation on and the other four kinds off", out["auto_accept"])
			}
		},
		bad:  call{http: setAutoAccept(`{"kinds":{"prototype":true}}`)},
		code: api.Invalid,
	}
}

func setAutoAccept(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/set_auto_accept", body}
}

func TestSetAutoAcceptIsRejectedForAnAgentAndAnIntegrationAndProjectsReadItOff(t *testing.T) {
	srv := newServer(t)
	for _, actor := range []api.Actor{api.Agent, api.Integration} {
		status, b := httpDoAs(t, srv, actor, setAutoAccept(`{"kinds":{"implementation":true}}`))
		if e := errorJSON(t, b, api.ActionNotAllowed); status != httpapi.Status(api.ActionNotAllowed) || !strings.Contains(e.Message, "human") {
			t.Fatalf("as %s: status %d, want %d; message %q", actor, status, httpapi.Status(api.ActionNotAllowed), e.Message)
		}
	}
	out := httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""})
	aa, _ := out["auto_accept"].(map[string]any)
	if len(aa) != 5 {
		t.Fatalf("auto_accept %v, want five kinds", out["auto_accept"])
	}
	for kind, on := range aa {
		if on != false {
			t.Errorf("auto_accept %s = %v, want off", kind, on)
		}
	}
}
