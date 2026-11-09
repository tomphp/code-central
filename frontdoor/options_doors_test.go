package frontdoor_test

import "github.com/tomoram/code-central/core"

// Options (Spec code-central-160): the human's, so HTTP only.
func init() {
	cases["get_options"] = opCase{
		ok:    call{http: httpCall{"GET", "/api/options", ""}},
		check: field("focus_clear", "all_clear"),
		bad:   call{http: httpCall{"GET", "/api/options?colour=red", ""}},
		code:  core.Invalid,
	}
	cases["set_options"] = opCase{
		ok:    call{http: httpCall{"POST", "/api/set_options", `{"focus_clear":"activity_dashboard"}`}},
		check: field("focus_clear", "activity_dashboard"),
		bad:   call{http: httpCall{"POST", "/api/set_options", `{"focus_clear":"fireworks"}`}},
		code:  core.Invalid,
	}
}
