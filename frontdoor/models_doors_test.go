package frontdoor_test

import (
	"testing"

	"github.com/tomoram/code-central/core"
)

// The Models section of Project Options (Spec code-central-301): set_models is
// the human's, so HTTP only.
func init() {
	cases["set_models"] = opCase{
		ok: call{http: setModels(`{"default":{"model":"opus","effort":""},"kinds":[{"kind":"implementation","model":"","effort":"xhigh"}]}`)},
		check: func(t *testing.T, out map[string]any) {
			acknowledgement("acme", "models")(t, out)
		},
		full: func(t *testing.T, out map[string]any) {
			t.Helper()
			m, _ := out["models"].(map[string]any)
			def, _ := m["default"].(map[string]any)
			field("model", "opus")(t, def)
			kinds, _ := m["kinds"].([]any)
			if len(kinds) != len(core.GuideNames) {
				t.Fatalf("kinds = %v, want a row per kind of work", m["kinds"])
			}
			impl := kinds[len(kinds)-1].(map[string]any)
			field("kind", "implementation")(t, impl)
			field("effort", "xhigh")(t, impl)
		},
		bad:  call{http: setModels(`{"kinds":[{"kind":"implementation","effort":"extreme"}]}`)},
		code: core.Invalid,
	}
}

func setModels(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/set_models", body}
}
