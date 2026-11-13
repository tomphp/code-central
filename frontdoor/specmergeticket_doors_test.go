package frontdoor_test

import (
	"context"
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
