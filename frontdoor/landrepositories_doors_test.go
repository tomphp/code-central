package frontdoor_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
)

// Landing across Repositories (Spec code-central-848): land takes a
// Repository through every door, and report_base a Repository over HTTP.

// acceptedInTwo adds the Repository api, then claims acme-3, finishes it
// in the Home Repository and api, and accepts it.
func acceptedInTwo(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	withAPI(claimedTicket)(t, srv)
	must(t, func() error {
		_, err := srv.Core.Finish(context.Background(), "acme", core.FinishInput{Ticket: "3", ClaimID: "claim-1", Summary: "Done.", Branch: "acme-3", Commit: "abc123",
			Repositories: []core.RepositoryWork{{Name: "api", Branch: "acme-3", Commit: "def456"}}})
		return err
	})
	must(t, func() error {
		_, err := srv.Core.Accept(core.WithActor(context.Background(), core.Human), "acme", core.TicketInput{Ticket: "3"})
		return err
	})
}

// land records the landing in the Repository it names through MCP, HTTP
// and the CLI alike, leaving the Ticket Partly landed.
func TestLandTakesARepositoryThroughEveryDoor(t *testing.T) {
	for door, land := range map[string]func(*testing.T, *codecentraltest.Server){
		"mcp": func(t *testing.T, srv *codecentraltest.Server) {
			mcpOK(t, connect(t, srv, "acme"), "land", map[string]any{"ticket": "acme-3", "commit": "fff999", "repository": "api"})
		},
		"http": func(t *testing.T, srv *codecentraltest.Server) {
			httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/3/land", `{"commit":"fff999","repository":"api"}`})
		},
		"cli": func(t *testing.T, srv *codecentraltest.Server) {
			cliOK(t, srv, []string{"land", "acme-3", "--commit", "fff999", "--repository", "api"})
		},
	} {
		t.Run(door, func(t *testing.T) {
			srv := newServer(t, acceptedInTwo)
			land(t, srv)
			tk := httpOK(t, srv, httpCall{"GET", "/api/p/acme/tickets/3", ""})
			api, _ := tk["repositories"].([]any)[1].(map[string]any)
			landed, _ := api["landed"].(map[string]any)
			if tk["state"] != "Accepted" || tk["partly_landed"] != true || landed["commit"] != "fff999" {
				t.Fatalf("ticket %v, want it Accepted and Partly landed, landed in api at fff999", tk)
			}
		})
	}
}

// land naming a Repository the Project hasn't is refused through every
// door alike, saying which Repositories there are, and lands nothing.
func TestLandRefusesAnUnknownRepositoryThroughEveryDoor(t *testing.T) {
	srv := newServer(t, acceptedInTwo)
	_, b := httpDo(t, srv, httpCall{"POST", "/api/p/acme/tickets/3/land", `{"commit":"fff999","repository":"web"}`})
	if e := errorJSON(t, b, core.Invalid); !strings.Contains(e.Message, "acme, api") {
		t.Fatalf("http refusal %+v, want it to name the Repositories", e)
	}
	res, mb := mcpCall(t, connect(t, srv, "acme"), "land", map[string]any{"ticket": "acme-3", "commit": "fff999", "repository": "web"})
	if !res.IsError || !strings.Contains(string(mb), "acme, api") {
		t.Fatalf("mcp: %s, want the refusal naming the Repositories", mb)
	}
	code, _, errOut := cliRun(t, srv, []string{"land", "acme-3", "--commit", "fff999", "--repository", "web"})
	if code == 0 || !strings.Contains(string(errOut), "acme, api") {
		t.Fatalf("cli: exit %d, %s, want the refusal naming the Repositories", code, errOut)
	}
	tk := httpOK(t, srv, httpCall{"GET", "/api/p/acme/tickets/3", ""})
	if tk["state"] != "Accepted" || tk["partly_landed"] == true {
		t.Fatalf("ticket %v, want it Accepted and landed nowhere", tk)
	}
}

// report_base names the Repository whose base it is, and refuses one the
// Project hasn't.
func TestReportBaseTakesARepository(t *testing.T) {
	srv := newServer(t, withAPI(func(*testing.T, *codecentraltest.Server) {}))
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/report_base", `{"state":"missing","repository":"api"}`})
	_, b := httpDo(t, srv, httpCall{"POST", "/api/p/acme/report_base", `{"state":"missing","repository":"web"}`})
	if e := errorJSON(t, b, core.Invalid); !strings.Contains(e.Message, "acme, api") {
		t.Fatalf("refusal %+v, want it to name the Repositories", e)
	}
}
