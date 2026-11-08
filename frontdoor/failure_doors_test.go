package frontdoor_test

import (
	"context"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
)

// failedTicket claims acme-3 as claim-1 and reports a failure on it.
func failedTicket(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	claimedTicket(t, srv)
	must(t, func() error {
		_, err := srv.Core.ReportFailure(context.Background(), "acme", core.ReportFailureInput{
			Ticket: "3", ClaimID: "claim-1", WhatBroke: "B", Why: "W", WhatToFix: "F", WhereWorkStands: "S",
		})
		return err
	})
}

func TestTheHumanSendsAFailedTicketBackOverHTTP(t *testing.T) {
	srv := newServer(t, failedTicket)
	field("state", "Ready")(t, httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/3/send_back", `{}`}))
}

// Failure Reports and giving up (ticket 30).
func init() {
	for name, c := range map[string]opCase{
		"report_failure": {
			setup: claimedTicket,
			ok: call{
				mcp: map[string]any{"ticket": "acme-3", "claim_id": "claim-1", "what_broke": "B", "why": "W", "what_to_fix": "F", "where_work_stands": "S"},
				http: httpCall{"POST", "/api/p/acme/tickets/3/report_failure",
					`{"claim_id":"claim-1","what_broke":"B","why":"W","what_to_fix":"F","where_work_stands":"S"}`},
				cli: []string{"acme-3", "--claim", "claim-1", "--what-broke", "B", "--why", "W", "--what-to-fix", "F", "--where-work-stands", "S"},
			},
			check: stopAcknowledgement("Failed", "state", "claims", "failure_reports", "moves", "needs_me_reason"),
			full: func(t *testing.T, out map[string]any) {
				field("next_step", "stop working this Ticket")(t, out)
				tk := out["ticket"].(map[string]any)
				field("state", "Failed")(t, tk)
				reports, _ := tk["failure_reports"].([]any)
				if len(reports) != 1 || reports[0].(map[string]any)["what_to_fix"] != "F" {
					t.Fatalf("failure_reports %v", tk["failure_reports"])
				}
			},
			bad: call{
				mcp:  map[string]any{"ticket": "acme-3", "claim_id": "claim-1", "what_broke": "B", "why": "W", "what_to_fix": "F", "where_work_stands": ""},
				http: httpCall{"POST", "/api/p/acme/tickets/3/report_failure", `{"claim_id":"claim-1","what_broke":"B","why":"W","what_to_fix":"F"}`},
				cli:  []string{"acme-3", "--claim", "claim-1", "--what-broke", "B", "--why", "W", "--what-to-fix", "F"},
			},
			code: core.Invalid,
		},
		"give_up": {
			setup: claimedTicket,
			ok: call{
				mcp:  map[string]any{"ticket": "acme-3", "claim_id": "claim-1", "note": "Out of time."},
				http: httpCall{"POST", "/api/p/acme/tickets/3/give_up", `{"claim_id":"claim-1","note":"Out of time."}`},
				cli:  []string{"acme-3", "--claim", "claim-1", "--note", "Out of time."},
			},
			check: stopAcknowledgement("Ready", "state", "claims", "comments", "moves", "claimable"),
			full: func(t *testing.T, out map[string]any) {
				field("next_step", "stop working this Ticket")(t, out)
				field("state", "Ready")(t, out["ticket"].(map[string]any))
			},
			bad: call{
				mcp:  map[string]any{"ticket": "acme-3", "claim_id": "claim-9", "note": "Out of time."},
				http: httpCall{"POST", "/api/p/acme/tickets/3/give_up", `{"claim_id":"claim-9","note":"Out of time."}`},
				cli:  []string{"acme-3", "--claim", "claim-9", "--note", "Out of time."},
			},
			code: core.StaleClaim,
		},
		// send_back as the agent sends Accepted work back on a conflict
		// (ticket 72); the human's send_back from Failed is tested below.
		"send_back": {
			setup: acceptedTicket,
			ok: call{
				mcp:  map[string]any{"ticket": "acme-3", "note": "Conflicts with main at 9f8e7d in core/claim.go."},
				http: httpCall{"POST", "/api/p/acme/tickets/3/send_back", `{"note":"Conflicts with main at 9f8e7d in core/claim.go."}`},
				cli:  []string{"acme-3", "--note", "Conflicts with main at 9f8e7d in core/claim.go."},
			},
			check: ticketAcknowledgement("acme-3", "Ready", "state", "comments", "moves", "claimable"),
			full: func(t *testing.T, out map[string]any) {
				field("state", "Ready")(t, out)
				cs, _ := out["comments"].([]any)
				if len(cs) != 2 || cs[1].(map[string]any)["body"] != "Conflicts with main at 9f8e7d in core/claim.go." {
					t.Fatalf("comments %v, want the finish summary then the note", out["comments"])
				}
			},
			// acme-3 is Ready by now; acme-2 is Draft.
			bad: call{
				mcp:  map[string]any{"ticket": "acme-2", "note": "N"},
				http: httpCall{"POST", "/api/p/acme/tickets/2/send_back", `{"note":"N"}`},
				cli:  []string{"acme-2", "--note", "N"},
			},
			code: core.ActionNotAllowed,
		},
		"to_draft": {
			setup: failedTicket,
			ok:    call{http: httpCall{"POST", "/api/p/acme/tickets/3/to_draft", `{}`}},
			check: ticketAcknowledgement("acme-3", "Draft", "state", "moves", "needs_me_reason"),
			full:  field("state", "Draft"),
			bad:   call{http: httpCall{"POST", "/api/p/acme/tickets/2/to_draft", `{}`}},
			code:  core.ActionNotAllowed,
		},
	} {
		cases[name] = c
	}
}
