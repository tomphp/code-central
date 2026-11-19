package server_test

import (
	"testing"

	"github.com/tomoram/code-central/core"
)

// The Storage tab (Spec code-central-1032): the human's, so HTTP only.
func init() {
	cases["get_storage"] = opCase{
		ok: call{http: httpCall{"GET", "/api/storage?project=acme", ""}},
		check: func(t *testing.T, out map[string]any) {
			t.Helper()
			field("kind", "memory")(t, out)
			projects, _ := out["projects"].([]any)
			if len(projects) != 1 {
				t.Fatalf("projects %v, want acme's alone", out["projects"])
			}
			acme := projects[0].(map[string]any)
			field("key", "acme")(t, acme)
			if acme["specs"] != float64(1) || acme["tickets"] != float64(2) {
				t.Fatalf("acme counts %v, want its Spec and its two Tickets", acme)
			}
		},
		bad:  call{http: httpCall{"GET", "/api/storage?project=nope", ""}},
		code: core.NotFound,
	}
}
