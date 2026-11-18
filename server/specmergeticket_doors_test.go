package server_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
)

// Spec merge Tickets (code-central-542): the Factory's open_spec_merge,
// over HTTP only.
func init() {
	cases["open_spec_merge"] = opCase{
		setup: notLongLived,
		ok:    call{http: openSpecMerge(`{"conflicts":["README"],"base_tip":"aaa111","spec_branch_tip":"bbb222"}`)},
		check: func(t *testing.T, out map[string]any) {
			t.Helper()
			field("ticket", "acme-4")(t, out)
			field("outcome", "opened")(t, out)
		},
		bad:  call{http: openSpecMerge(`{"conflicts":[],"base_tip":"aaa111","spec_branch_tip":"bbb222"}`)},
		code: core.Invalid,
	}
}

func openSpecMerge(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/specs/1/open_spec_merge", body}
}

// notLongLived takes the long-lived Label off acme-1, so it has a Spec
// Branch.
func notLongLived(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	must(t, func() error {
		_, err := srv.Core.RemoveLabels(context.Background(), "acme", core.LabelsInput{Target: "acme-1", Labels: []string{core.LongLivedLabel}})
		return err
	})
}

// open_spec_merge names each conflicting Repository's files and tips
// (Spec code-central-848), and refuses a Repository the Project hasn't.
func TestOpenSpecMergeTakesEachRepositorysConflictOverHTTP(t *testing.T) {
	srv := newServer(t, withAPI(notLongLived))
	out := httpOK(t, srv, openSpecMerge(`{"repositories":[{"name":"api","conflicts":["endpoint.go"],"base_tip":"ccc333","spec_branch_tip":"ddd444"}]}`))
	field("outcome", "opened")(t, out)
	tk := httpOK(t, srv, httpCall{"GET", "/api/p/acme/tickets/4", ""})
	if d, _ := tk["description"].(string); !strings.Contains(d, "- api: merging spec/acme-1 at ddd444 into main at ccc333 conflicts in these files:\n  - endpoint.go\n") {
		t.Fatalf("description %q, want it to name api's conflict", d)
	}
	_, b := httpDo(t, srv, openSpecMerge(`{"repositories":[{"name":"web","conflicts":["a"],"base_tip":"a","spec_branch_tip":"b"}]}`))
	if e := errorJSON(t, b, core.Invalid); e.Field != "repositories[0].name" || !strings.Contains(e.Message, "acme, api") {
		t.Fatalf("refusal %+v, want it to name the Repositories", e)
	}
}
