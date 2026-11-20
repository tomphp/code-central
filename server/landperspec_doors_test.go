package server_test

import (
	"context"
	"github.com/tomoram/code-central/domain"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/codecentraltest"
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
		code: api.Invalid,
	}
}

// specLandedAccepted makes acme-1 a Requirement and accepts acme-3, so
// it is a Spec-landed Ticket under the default Landing Method.
func specLandedAccepted(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	must(t, func() error {
		_, err := srv.Core.RemoveLabels(context.Background(), "acme", api.LabelsInput{Target: "acme-1", Labels: []string{domain.LongLivedLabel}})
		return err
	})
	must(t, func() error {
		_, err := srv.Core.CreateTicket(context.Background(), "acme", api.CreateTicketInput{Spec: "1", Title: "Grilling", Labels: []string{domain.StageLabel("grilling")}})
		return err
	})
	acceptedTicket(t, srv)
}

// report_spec_merge names the Repository of the Spec Branch it reports
// (Spec code-central-848), and refuses one the Project hasn't.
func TestReportSpecMergeTakesARepositoryOverHTTP(t *testing.T) {
	srv := newServer(t, withAPI(specLandedAccepted))
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/3/report_spec_merge", `{"commit":"def456","repository":"api"}`})
	tk := httpOK(t, srv, httpCall{"GET", "/api/p/acme/tickets/3", ""})
	if sm, _ := tk["spec_merge"].(map[string]any); sm["repository"] != "api" || sm["commit"] != "def456" {
		t.Fatalf("spec_merge %v, want def456 in api", tk["spec_merge"])
	}
	_, b := httpDo(t, srv, httpCall{"POST", "/api/p/acme/tickets/3/report_spec_merge", `{"commit":"def456","repository":"web"}`})
	if e := errorJSON(t, b, api.Invalid); !strings.Contains(e.Message, "acme, api") {
		t.Fatalf("refusal %+v, want it to name the Repositories", e)
	}
}
