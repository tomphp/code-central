package frontdoor_test

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
)

// Options (Spec code-central-160): the human's, so HTTP only.
func init() {
	cases["get_options"] = opCase{
		ok:    call{http: httpCall{"GET", "/api/options", ""}},
		check: field("focus_clear", "activity_dashboard"),
		bad:   call{http: httpCall{"GET", "/api/options?colour=red", ""}},
		code:  core.Invalid,
	}
	cases["set_options"] = opCase{
		ok:    call{http: httpCall{"POST", "/api/set_options", `{"focus_clear":"all_clear"}`}},
		check: field("focus_clear", "all_clear"),
		bad:   call{http: httpCall{"POST", "/api/set_options", `{"focus_clear":"fireworks"}`}},
		code:  core.Invalid,
	}
}

func TestTheGHPathOptionIsAnAbsolutePathOrEmpty(t *testing.T) {
	srv := newServer(t)
	field("gh_path", "")(t, httpOK(t, srv, httpCall{"GET", "/api/options", ""}))
	gh := filepath.Join(t.TempDir(), "gh")
	field("gh_path", gh)(t, httpOK(t, srv, httpCall{"POST", "/api/set_options", `{"gh_path":` + strconv.Quote(" "+gh+" ") + `}`}))
	field("gh_path", gh)(t, httpOK(t, srv, httpCall{"GET", "/api/options", ""}))

	e := httpRejected(t, srv, httpCall{"POST", "/api/set_options", `{"gh_path":"bin/gh"}`}, core.Invalid)
	if e.Field != "gh_path" || !strings.Contains(e.Message, "bin/gh") {
		t.Fatalf("refusal = %+v, want gh_path naming the path", e)
	}
	field("gh_path", "")(t, httpOK(t, srv, httpCall{"POST", "/api/set_options", `{"gh_path":""}`}))
}
