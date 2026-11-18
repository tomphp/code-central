package server_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
)

// A Ticket's Changed Repositories (Spec code-central-848): the Factory's
// report_changed_repositories and fail_finish, over HTTP only, and
// finish's repositories through every door.
func init() {
	cases["report_changed_repositories"] = opCase{
		setup: withAPI(claimedTicket),
		ok:    call{http: reportChanged(`{"claim_id":"claim-1","repositories":[{"name":"api","branch":"acme-3","commit":"abc123"}]}`)},
		check: ticketAcknowledgement("acme-3", "In Progress", "repositories"),
		full:  func(t *testing.T, out map[string]any) { changedNames(t, out, "api") },
		bad:   call{http: reportChanged(`{"claim_id":"claim-9","repositories":[]}`)},
		code:  core.StaleClaim,
	}
	cases["fail_finish"] = opCase{
		setup: withAPI(ticketInReview),
		ok:    call{http: failFinishCall("3")},
		check: ticketAcknowledgement("acme-3", "Failed", "state", "failure_reports", "moves", "needs_me_reason"),
		full: func(t *testing.T, out map[string]any) {
			reports, _ := out["failure_reports"].([]any)
			if len(reports) != 1 || reports[0].(map[string]any)["claim"] != float64(1) {
				t.Fatalf("failure reports %v, want one on the finishing Claim 1", out["failure_reports"])
			}
		},
		bad:  call{http: failFinishCall("2")},
		code: core.ActionNotAllowed,
	}
}

func reportChanged(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/tickets/3/report_changed_repositories", body}
}

func failFinishCall(n string) httpCall {
	return httpCall{"POST", "/api/p/acme/tickets/" + n + "/fail_finish", `{"what_broke":"api left out","why":"api has commits","what_to_fix":"finish with api","where_work_stands":"api at abc123"}`}
}

// withAPI adds the Repository api to acme, then runs then.
func withAPI(then func(*testing.T, *codecentraltest.Server)) func(*testing.T, *codecentraltest.Server) {
	return func(t *testing.T, srv *codecentraltest.Server) {
		t.Helper()
		withRepository(`{"name":"api","clone_url":"git@github.com:acme/api.git"}`)(t, srv)
		then(t, srv)
	}
}

// changedNames checks a Ticket's Changed Repositories are named want, in
// order.
func changedNames(t *testing.T, ticket map[string]any, want ...string) {
	t.Helper()
	list, _ := ticket["repositories"].([]any)
	var got []string
	for _, r := range list {
		got = append(got, r.(map[string]any)["name"].(string))
	}
	if !slices.Equal(got, want) {
		t.Fatalf("repositories = %v, want %v", got, want)
	}
}

// finish takes repositories through MCP, HTTP and the CLI alike, and the
// Ticket then reads with the Home Repository's work and api's, in
// landing order.
func TestFinishTakesRepositoriesThroughEveryDoor(t *testing.T) {
	const list = `[{"name":"api","branch":"acme-3","commit":"def456"}]`
	for door, finish := range map[string]func(*testing.T, *codecentraltest.Server){
		"mcp": func(t *testing.T, srv *codecentraltest.Server) {
			mcpOK(t, connect(t, srv, "acme"), "finish", map[string]any{"ticket": "acme-3", "claim_id": "claim-1", "summary": "Done.",
				"branch": "acme-3", "commit": "abc123", "repositories": []map[string]any{{"name": "api", "branch": "acme-3", "commit": "def456"}}})
		},
		"http": func(t *testing.T, srv *codecentraltest.Server) {
			httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/3/finish", `{"claim_id":"claim-1","summary":"Done.","branch":"acme-3","commit":"abc123","repositories":` + list + `}`})
		},
		"cli": func(t *testing.T, srv *codecentraltest.Server) {
			cliOK(t, srv, []string{"finish", "acme-3", "--claim", "claim-1", "--summary", "Done.", "--branch", "acme-3", "--commit", "abc123", "--repositories", list})
		},
	} {
		t.Run(door, func(t *testing.T) {
			srv := newServer(t, withAPI(claimedTicket))
			finish(t, srv)
			tk := httpOK(t, srv, httpCall{"GET", "/api/p/acme/tickets/3", ""})
			changedNames(t, tk, "acme", "api")
			api := tk["repositories"].([]any)[1].(map[string]any)
			if api["branch"] != "acme-3" || api["commit"] != "def456" || tk["branch"] != "acme-3" || tk["commit"] != "abc123" {
				t.Fatalf("ticket %v, want the Home Repository's work in branch and commit and api's in repositories", tk)
			}
		})
	}
}

// A finish naming a Repository the Project hasn't is refused through
// every door alike, saying which Repositories there are.
func TestFinishRefusesAnUnknownRepositoryThroughEveryDoor(t *testing.T) {
	srv := newServer(t, withAPI(claimedTicket))
	_, b := httpDo(t, srv, httpCall{"POST", "/api/p/acme/tickets/3/finish", `{"claim_id":"claim-1","summary":"Done.","repositories":[{"name":"web","branch":"b","commit":"c"}]}`})
	if e := errorJSON(t, b, core.Invalid); !strings.Contains(e.Message, "acme, api") {
		t.Fatalf("http refusal %+v, want it to name the Repositories", e)
	}
	res, mb := mcpCall(t, connect(t, srv, "acme"), "finish", map[string]any{"ticket": "acme-3", "claim_id": "claim-1", "summary": "Done.",
		"repositories": []map[string]any{{"name": "web", "branch": "b", "commit": "c"}}})
	if !res.IsError || !strings.Contains(string(mb), "acme, api") {
		t.Fatalf("mcp: %s, want the refusal naming the Repositories", mb)
	}
	code, _, errOut := cliRun(t, srv, []string{"finish", "acme-3", "--claim", "claim-1", "--summary", "Done.", "--repositories", `[{"name":"web","branch":"b","commit":"c"}]`})
	if code == 0 || !strings.Contains(string(errOut), "acme, api") {
		t.Fatalf("cli: exit %d, %s, want the refusal naming the Repositories", code, errOut)
	}
	// The Claim is still the agent's, to finish again.
	tk, err := srv.Core.GetTicket(context.Background(), "acme", core.GetTicketInput{Ticket: "acme-3"})
	if err != nil || tk.State != "In Progress" {
		t.Fatalf("ticket %s (%v), want it still In Progress", tk.State, err)
	}
}
