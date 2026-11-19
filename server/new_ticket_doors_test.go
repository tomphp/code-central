package server_test

import (
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/codecentraltest"
)

// New Ticket (Spec code-central-1067): the human's, so HTTP only. The
// suite's Spec acme-1 is Long-lived, so the case opens the Ticket on plain
// Spec acme-4, as acme-5.
func init() {
	cases["new_ticket"] = opCase{
		setup: plainSpec,
		ok:    call{http: newTicket(`{"spec":"acme-4","text":"Rename the toggle\n\nAs review asked."}`)},
		check: ticketAcknowledgement("acme-5", "Ready", "title", "state", "spec", "description", "spec_landed", "moves", "claimable"),
		full: func(t *testing.T, out map[string]any) {
			field("id", "acme-5")(t, out)
			field("title", "Rename the toggle")(t, out)
			field("state", "Ready")(t, out)
			sp, _ := out["spec"].(map[string]any)
			field("id", "acme-4")(t, sp)
			field("description", "Rename the toggle\n\nAs review asked.")(t, out)
			labels()(t, out)
		},
		bad:  call{http: newTicket(`{"spec":"acme-1","text":"More"}`)},
		code: api.ActionNotAllowed,
	}
}

func newTicket(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/new_ticket", body}
}

// plainSpec is plain Spec acme-4, neither Long-lived, a Map nor a
// Requirement.
func plainSpec(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/create_spec", `{"title":"Dark mode"}`})
}

// A Spec read says whether it takes a New Ticket, and new_ticket refuses
// where it says not.
func TestASpecReadSaysWhetherNewTicketIsAllowed(t *testing.T) {
	srv := newServer(t, plainSpec)
	boolField(t, httpOK(t, srv, httpCall{"GET", "/api/p/acme/specs/4", ""}), "ticketable", true)
	boolField(t, httpOK(t, srv, httpCall{"GET", "/api/p/acme/specs/1", ""}), "ticketable", false)
	e := httpRejected(t, srv, newTicket(`{"spec":"acme-1","text":"More"}`), api.ActionNotAllowed)
	if !strings.Contains(e.Message, "Long-lived") {
		t.Fatalf("new_ticket on a Long-lived Spec refused with %q, want it to say why", e.Message)
	}
	httpRejected(t, srv, newTicket(`{"spec":"acme-4","text":"  "}`), api.Invalid)
}
