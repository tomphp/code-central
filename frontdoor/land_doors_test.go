package frontdoor_test

import (
	"context"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
)

// Accepted and land (ticket 71).
func init() {
	cases["land"] = opCase{
		setup: acceptedTicket,
		ok: call{
			mcp:  map[string]any{"ticket": "acme-3", "commit": "def456"},
			http: httpCall{"POST", "/api/p/acme/tickets/3/land", `{"commit":"def456"}`},
			cli:  []string{"acme-3", "--commit", "def456"},
		},
		check: ticketAcknowledgement("acme-3", "Done", "state", "moves"),
		full: func(t *testing.T, out map[string]any) {
			field("state", "Done")(t, out)
			moves, _ := out["moves"].([]any)
			last, _ := moves[len(moves)-1].(map[string]any)
			field("from", "Accepted")(t, last)
			field("via", "merge")(t, last)
			field("commit", "def456")(t, last)
		},
		bad: call{
			mcp:  map[string]any{"ticket": "acme-3", "commit": " "},
			http: httpCall{"POST", "/api/p/acme/tickets/3/land", `{"commit":" "}`},
			cli:  []string{"acme-3", "--commit", " "},
		},
		code: core.Invalid,
	}
}

// acceptedTicket claims acme-3, finishes it on branch acme-3 and accepts
// it, so it is Accepted.
func acceptedTicket(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	claimedTicket(t, srv)
	must(t, func() error {
		_, err := srv.Core.Finish(context.Background(), "acme", core.FinishInput{Ticket: "3", ClaimID: "claim-1", Summary: "Done.", Branch: "acme-3", Commit: "abc123"})
		return err
	})
	must(t, func() error {
		_, err := srv.Core.Accept(core.WithActor(context.Background(), core.Human), "acme", core.TicketInput{Ticket: "3"})
		return err
	})
}
