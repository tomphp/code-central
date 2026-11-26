package server_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/httpapi"
)

// Stopping a Factory Claim (Spec code-central-1233): the human's alone,
// over HTTP.
func init() {
	for name, c := range map[string]opCase{
		"stop": {
			setup: factoryClaimedTicket,
			ok:    call{http: httpCall{"POST", "/api/p/acme/tickets/3/stop", `{"note":"Wrong approach."}`}},
			check: ticketAcknowledgement("acme-3", "In Progress", "claims", "comments", "stoppable", "resumable", "needs_me_reason"),
			full:  trueField("resumable"),
			bad:   call{http: httpCall{"POST", "/api/p/acme/tickets/2/stop", `{}`}},
			code:  api.ActionNotAllowed,
		},
		"resume": {
			setup: stoppedTicket,
			ok:    call{http: httpCall{"POST", "/api/p/acme/tickets/3/resume", `{}`}},
			check: ticketAcknowledgement("acme-3", "In Progress", "claims", "stoppable", "resumable", "needs_me_reason"),
			full:  trueField("stoppable"),
			bad:   call{http: httpCall{"POST", "/api/p/acme/tickets/2/resume", `{}`}},
			code:  api.ActionNotAllowed,
		},
	} {
		cases[name] = c
	}
}

// factoryClaimedTicket has a Factory claim acme-3.
func factoryClaimedTicket(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	must(t, func() error {
		_, err := srv.Core.Claim(context.Background(), "acme", api.ClaimInput{SessionID: "s1", Factory: "cc", Host: "box"})
		return err
	})
}

// stoppedTicket has a Factory claim acme-3 and the human stop it.
func stoppedTicket(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	factoryClaimedTicket(t, srv)
	must(t, func() error {
		_, err := srv.Core.Stop(api.WithActor(context.Background(), api.Human), "acme", api.StopInput{Ticket: "3"})
		return err
	})
}

func TestStopAndResumeAreRefusedForAnAgent(t *testing.T) {
	srv := newServer(t)
	stoppedTicket(t, srv)
	for _, c := range []httpCall{
		{http.MethodPost, "/api/p/acme/tickets/3/stop", `{}`},
		{http.MethodPost, "/api/p/acme/tickets/3/resume", `{}`},
	} {
		status, b := httpDoAs(t, srv, api.Agent, c)
		if e := errorJSON(t, b, api.ActionNotAllowed); status != httpapi.Status(api.ActionNotAllowed) || !strings.Contains(e.Message, "human action") {
			t.Fatalf("%s: status %d message %q, want a human action", c.path, status, e.Message)
		}
	}
}

// trueField checks the record's name is true.
func trueField(name string) func(*testing.T, map[string]any) {
	return func(t *testing.T, out map[string]any) {
		t.Helper()
		boolField(t, out, name, true)
	}
}
