package frontdoor_test

import (
	"testing"

	"github.com/tomoram/code-central/core"
)

// A Repository's CI entry (Spec code-central-848): set_repository_ci is
// the human's, so HTTP only. Its refusal names the field to put right.
func init() {
	cases["set_repository_ci"] = opCase{
		setup: withRepository(`{"name":"web","clone_url":"git@github.com:acme/web.git"}`),
		ok:    call{http: setRepositoryCI(`{"name":"web","not_watched":true}`)},
		check: func(t *testing.T, out map[string]any) {
			acknowledgement("acme", "ci")(t, out)
		},
		full: func(t *testing.T, out map[string]any) {
			entries, _ := out["ci"].(map[string]any)["repositories"].([]any)
			if len(entries) != 1 || entries[0].(map[string]any)["name"] != "web" || entries[0].(map[string]any)["not_watched"] != true {
				t.Fatalf("CI entries = %v, want web Not watched", entries)
			}
		},
		bad:  call{http: setRepositoryCI(`{"name":"web","repository":"not a repository"}`)},
		code: core.Invalid,
	}
}

func setRepositoryCI(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/set_repository_ci", body}
}
