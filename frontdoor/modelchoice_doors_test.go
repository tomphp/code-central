package frontdoor_test

import (
	"context"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
)

// The Model Choice (Spec code-central-301): the Factory's
// get_model_choice and report_model_choice, both over HTTP only.
func init() {
	cases["get_model_choice"] = opCase{
		setup: taggedTicket,
		ok:    call{http: httpCall{"GET", "/api/p/acme/tickets/3/model-choice", ""}},
		check: func(t *testing.T, out map[string]any) {
			t.Helper()
			model, _ := out["model"].(map[string]any)
			field("value", "opus")(t, model)
			field("source", "ticket_tag")(t, model)
			effort, _ := out["effort"].(map[string]any)
			field("source", "none")(t, effort)
		},
		bad:  call{http: httpCall{"GET", "/api/p/acme/tickets/99/model-choice", ""}},
		code: core.NotFound,
	}
	cases["report_model_choice"] = opCase{
		setup: claimedTicket,
		ok:    call{http: reportModelChoice(`{"claim_id":"claim-1","model":"opus","effort":"high"}`)},
		check: ticketAcknowledgement("acme-3", "In Progress", "claims"),
		full: func(t *testing.T, out map[string]any) {
			claims, _ := out["claims"].([]any)
			if len(claims) != 1 {
				t.Fatalf("claims %v", out["claims"])
			}
			claim := claims[0].(map[string]any)
			field("model", "opus")(t, claim)
			field("effort", "high")(t, claim)
		},
		bad:  call{http: reportModelChoice(`{"claim_id":"claim-9","model":"opus"}`)},
		code: core.StaleClaim,
	}
}

func reportModelChoice(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/tickets/3/report_model_choice", body}
}

// taggedTicket gives acme-3 the model Tag opus.
func taggedTicket(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	must(t, func() error {
		_, err := srv.Core.SetTags(context.Background(), "acme", core.SetTagsInput{Target: "3", Tags: map[string]string{"model": "opus"}})
		return err
	})
}

// The UI reads a Ticket over HTTP with its Model Choice; agents' MCP
// get_ticket leaves it out, to keep their context small.
func TestOnlyTheUIsTicketReadCarriesTheModelChoice(t *testing.T) {
	srv := newServer(t, taggedTicket)
	s := connect(t, srv, "acme")
	out := httpOK(t, srv, httpCall{"GET", "/api/p/acme/tickets/3", ""})
	mc, _ := out["model_choice"].(map[string]any)
	model, _ := mc["model"].(map[string]any)
	field("value", "opus")(t, model)
	field("source", "ticket_tag")(t, model)
	effort, _ := mc["effort"].(map[string]any)
	field("source", "none")(t, effort)

	out = mcpOK(t, s, "get_ticket", map[string]any{"ticket": "acme-3"})
	if _, ok := out["model_choice"]; ok {
		t.Errorf("MCP get_ticket carries model_choice: %v", out["model_choice"])
	}
}
