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
	// report_models is the Factory's, over HTTP only (code-central-311).
	cases["report_models"] = opCase{
		ok: call{http: reportModels(`{"effort_levels":["low","high"],"aliases":["opus"],"model_flag":"opus","effort_flag":""}`)},
		check: func(t *testing.T, out map[string]any) {
			acknowledgement("acme", "reported_models")(t, out)
		},
		full: func(t *testing.T, out map[string]any) {
			t.Helper()
			r, _ := out["reported_models"].(map[string]any)
			field("model_flag", "opus")(t, r)
			field("effort_flag", "")(t, r)
			if levels, _ := r["effort_levels"].([]any); len(levels) != 2 || levels[0] != "low" || levels[1] != "high" {
				t.Fatalf("effort_levels = %v, want [low high]", r["effort_levels"])
			}
			if _, ok := r["reported"].(string); !ok {
				t.Fatalf("reported = %v, want when it arrived", r["reported"])
			}
		},
		bad:  call{http: reportModels(`{"aliases":["opus\nsonnet"]}`)},
		code: core.Invalid,
	}
}

func reportModels(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/report_models", body}
}

func setModels(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/set_models", body}
}
