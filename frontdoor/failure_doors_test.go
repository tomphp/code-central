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
			check: func(t *testing.T, out map[string]any) {
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
			check: func(t *testing.T, out map[string]any) {
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
		"send_back": {
			setup: failedTicket,
			ok:    call{http: httpCall{"POST", "/api/p/acme/tickets/3/send_back", `{"note":"Fixed the migration."}`}},
			check: func(t *testing.T, out map[string]any) {
				field("state", "Ready")(t, out)
				if cs, _ := out["comments"].([]any); len(cs) != 1 {
					t.Fatalf("comments %v", out["comments"])
				}
			},
			bad:  call{http: httpCall{"POST", "/api/p/acme/tickets/2/send_back", `{}`}},
			code: core.ActionNotAllowed,
		},
		"to_draft": {
			setup: failedTicket,
			ok:    call{http: httpCall{"POST", "/api/p/acme/tickets/3/to_draft", `{}`}},
			check: field("state", "Draft"),
			bad:   call{http: httpCall{"POST", "/api/p/acme/tickets/2/to_draft", `{}`}},
			code:  core.ActionNotAllowed,
		},
	} {
		cases[name] = c
	}
}
