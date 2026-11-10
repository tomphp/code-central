package frontdoor_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
)

// Unblock on push (code-central-298): a Blocker the CI gate holds stops
// blocking while its CI result is running or passed, and a failed result
// holds its dependants again, which claim names.

// unblockOnPushWithADependant turns Unblock on push on and adds acme-4, Ready and
// blocked by acme-3.
func unblockOnPushWithADependant(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	httpOK(t, srv, setCITracking(`{"tracking":true,"repository":"acme/web","ignored_workflows":["nightly.yml"],"unblock_on_push":true}`))
	ctx := context.Background()
	must(t, func() error {
		_, err := srv.Core.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "Login page", State: "Ready"})
		return err
	})
	must(t, func() error {
		_, err := srv.Core.AddBlocker(ctx, "acme", core.BlockerInput{Ticket: "acme-4", Blocker: "acme-3"})
		return err
	})
}

func TestClaimingATicketHeldByACIFailedBlockerIsRefusedNamingItsCIResult(t *testing.T) {
	srv := newServer(t, failedCI, unblockOnPushWithADependant)
	e := httpRejected(t, srv, httpCall{"POST", "/api/p/acme/claim", `{"ticket":"acme-4","session_id":"sess-1"}`}, core.ActionNotAllowed)
	if !strings.Contains(e.Message, "acme-3 (Accepted, CI failed)") {
		t.Fatalf("message %q, want it to name acme-3 as Accepted with its CI failed", e.Message)
	}
}

func TestATicketHeldByABlockerWhoseCIRunsCanBeClaimed(t *testing.T) {
	srv := newServer(t, pushedAccepted, unblockOnPushWithADependant)
	out := httpOK(t, srv, httpCall{"POST", "/api/p/acme/claim", `{"ticket":"acme-4","session_id":"sess-1"}`})
	if tk, _ := out["ticket"].(map[string]any); tk["id"] != "acme-4" || tk["state"] != "In Progress" {
		t.Fatalf("claimed %v, want acme-4 claimed while acme-3's CI runs", out)
	}
}

func TestWithUnblockOnPushOffTheRefusalStillNamesAGatedBlockersCIResult(t *testing.T) {
	srv := newServer(t, failedCI, unblockOnPushWithADependant)
	httpOK(t, srv, setCITracking(`{"tracking":true,"repository":"acme/web","ignored_workflows":["nightly.yml"],"unblock_on_push":false}`))
	e := httpRejected(t, srv, httpCall{"POST", "/api/p/acme/claim", `{"ticket":"acme-4","session_id":"sess-1"}`}, core.ActionNotAllowed)
	if !strings.Contains(e.Message, "acme-3 (Accepted, CI failed)") {
		t.Fatalf("message %q, want it to name acme-3 as Accepted with its CI failed", e.Message)
	}
}
