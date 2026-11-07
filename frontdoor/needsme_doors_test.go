package frontdoor_test

import (
	"testing"

	"github.com/tomoram/code-central/core"
)

// Needs Me, moving by hand and Projects (ticket 32). All are the human's,
// so HTTP only.
func init() {
	for name, c := range map[string]opCase{
		"needs_me": {
			setup: ticketInReview,
			ok:    call{http: httpCall{"GET", "/api/needs-me", ""}},
			check: func(t *testing.T, out map[string]any) {
				groups, _ := out["groups"].([]any)
				if out["count"] != float64(1) || len(groups) != 1 {
					t.Fatalf("needs-me %v", out)
				}
				g := groups[0].(map[string]any)
				field("reason", "in_review")(t, g)
				row := g["tickets"].([]any)[0].(map[string]any)
				field("id", "acme-3")(t, row)
				field("summary", "Done.")(t, row)
			},
			bad:  call{http: httpCall{"GET", "/api/needs-me?project=acme", ""}},
			code: core.Invalid,
		},
		"list_projects": {
			ok: call{http: httpCall{"GET", "/api/projects", ""}},
			check: func(t *testing.T, out map[string]any) {
				rows, _ := out["rows"].([]any)
				if len(rows) != 2 {
					t.Fatalf("projects %v", out)
				}
				field("key", "acme")(t, rows[0].(map[string]any))
				if rows[0].(map[string]any)["needs_me"] != float64(0) {
					t.Fatalf("acme %v", rows[0])
				}
			},
			bad:  call{http: httpCall{"GET", "/api/projects?archived=true", ""}},
			code: core.Invalid,
		},
		"move": {
			ok:    call{http: httpCall{"POST", "/api/p/acme/tickets/2/move", `{"to_state":"Done"}`}},
			check: field("state", "Done"),
			bad:   call{http: httpCall{"POST", "/api/p/acme/tickets/2/move", `{"to_state":"Failed"}`}},
			code:  core.Invalid,
		},
		"rename_project": {
			ok: call{http: httpCall{"POST", "/api/p/acme/rename_project", `{"display_name":"Acme Web"}`}},
			check: func(t *testing.T, out map[string]any) {
				field("key", "acme")(t, out)
				field("display_name", "Acme Web")(t, out)
			},
			bad:  call{http: httpCall{"POST", "/api/p/nope/rename_project", `{"display_name":"X"}`}},
			code: core.UnknownProject,
		},
	} {
		cases[name] = c
	}
}
