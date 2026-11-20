package server_test

import "github.com/tomoram/code-central/api"

// Project modes (ticket 41).
func init() {
	cases["set_project_mode"] = opCase{
		ok:    call{http: httpCall{"POST", "/api/projects/acme/set_project_mode", `{"mode":"local"}`}},
		check: field("mode", "local"),
		bad:   call{http: httpCall{"POST", "/api/projects/acme/set_project_mode", `{"mode":"shared"}`}},
		code:  api.Invalid,
	}
}
