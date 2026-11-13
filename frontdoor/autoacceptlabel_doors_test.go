package frontdoor_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/httpapi"
)

// The auto-accept Label (Spec code-central-536) is the human's: MCP and
// the CLI, which call as agent, are refused it, and the UI's add_labels
// accepts what it covers that is In Review.
func TestTheAutoAcceptLabelIsTheHumansOnEveryDoor(t *testing.T) {
	srv := newServer(t)
	ctx := context.Background()
	must(t, func() error {
		res, err := srv.Core.Claim(ctx, "acme", core.ClaimInput{Ticket: "acme-3"})
		if err != nil {
			return err
		}
		_, err = srv.Core.Finish(ctx, "acme", core.FinishInput{Ticket: "acme-3", ClaimID: res.ClaimID, Summary: "Done."})
		return err
	})
	reserved := func(t *testing.T, door string, e core.Error) {
		t.Helper()
		if !strings.Contains(e.Message, "reserved for the human") {
			t.Fatalf("%s: message %q, want the Label named as reserved for the human", door, e.Message)
		}
	}

	s := connect(t, srv, "acme")
	for _, tool := range []string{"add_labels", "remove_labels"} {
		reserved(t, "mcp "+tool, mcpRejected(t, s, tool, map[string]any{"target": "acme-3", "labels": []string{core.AutoAcceptLabel}}, core.Invalid))
	}
	reserved(t, "mcp create_ticket", mcpRejected(t, s, "create_ticket", map[string]any{"spec": "acme-1", "title": "Mine", "labels": []string{core.AutoAcceptLabel}}, core.Invalid))
	reserved(t, "cli add-labels", cliRejected(t, srv, []string{"add-labels", "acme-3", core.AutoAcceptLabel}, core.Invalid))
	status, b := httpDoAs(t, srv, core.Agent, httpCall{"POST", "/api/p/acme/add_labels", `{"target":"acme-3","labels":["auto-accept"]}`})
	if e := errorJSON(t, b, core.Invalid); status != httpapi.Status(core.Invalid) {
		t.Fatalf("http as agent: status %d, %q", status, e.Message)
	}

	httpOK(t, srv, httpCall{"POST", "/api/p/acme/add_labels", `{"target":"acme-3","labels":["auto-accept"]}`})
	out := httpOK(t, srv, httpCall{"GET", "/api/p/acme/tickets/acme-3", ""})
	moves, _ := out["moves"].([]any)
	last, _ := moves[len(moves)-1].(map[string]any)
	if out["state"] != "Done" || last["actor"] != "human" || last["cause"] != "ticket-label" {
		t.Fatalf("acme-3 is %v with last move %v, want Done by human with cause ticket-label", out["state"], last)
	}
}
