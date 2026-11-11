package frontdoor_test

import (
	"context"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
)

// Land per Spec (code-central-371): the Factory's report_spec_merge, over
// HTTP only.
func init() {
	cases["report_spec_merge"] = opCase{
		setup: specLandedAccepted,
		ok:    call{http: httpCall{"POST", "/api/p/acme/tickets/3/report_spec_merge", `{"commit":"def456"}`}},
		check: ticketAcknowledgement("acme-3", "Accepted", "spec_merge"),
		full: func(t *testing.T, out map[string]any) {
			if out["spec_landed"] != true {
				t.Fatalf("spec_landed = %v, want true in %v", out["spec_landed"], out)
			}
		},
		bad:  call{http: httpCall{"POST", "/api/p/acme/tickets/3/report_spec_merge", `{"commit":" "}`}},
		code: core.Invalid,
	}
}

// specLandedAccepted makes acme-1 a Requirement and accepts acme-3, so
// it is a Spec-landed Ticket under the default Landing Method.
func specLandedAccepted(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	must(t, func() error {
		_, err := srv.Core.RemoveLabels(context.Background(), "acme", core.LabelsInput{Target: "acme-1", Labels: []string{core.LongLivedLabel}})
		return err
	})
	must(t, func() error {
		_, err := srv.Core.CreateTicket(context.Background(), "acme", core.CreateTicketInput{Spec: "1", Title: "Grilling", Labels: []string{core.StageLabel("grilling")}})
		return err
	})
	acceptedTicket(t, srv)
}
