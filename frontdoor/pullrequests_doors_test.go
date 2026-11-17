package frontdoor_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Pull requests per Repository (Spec code-central-848): land's
// repository through every door, and the Factory's and the Pull Request
// Watcher's repository on report_pushed_commit and report_pull_request,
// over HTTP.

// acceptedInAPI is acme with the Repository api, under Pull Request, and
// acme-3 Accepted with work in acme and api.
func acceptedInAPI(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	ctx, human := context.Background(), core.WithActor(context.Background(), core.Human)
	withAPI(claimedTicket)(t, srv)
	must(t, func() error {
		_, err := srv.Client.ReportPushRemote(ctx, "acme", core.ReportPushRemoteInput{URL: "git@github.com:acme/acme.git", Base: "main"})
		return err
	})
	must(t, func() error {
		_, err := srv.Core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingPullRequest, Remote: "origin"})
		return err
	})
	acceptInTwo(t, srv)
}

// landedIn is the commit acme-3 landed at in the Repository name, as
// read over HTTP.
func landedIn(t *testing.T, srv *codecentraltest.Server, name string) (state, commit string) {
	t.Helper()
	tk := httpOK(t, srv, httpCall{"GET", "/api/p/acme/tickets/3", ""})
	for _, r := range tk["repositories"].([]any) {
		if r := r.(map[string]any); r["name"] == name {
			if l, ok := r["landed"].(map[string]any); ok {
				commit, _ = l["commit"].(string)
			}
		}
	}
	return tk["state"].(string), commit
}

// land takes repository through MCP, HTTP and the CLI alike: the commit
// is api's landed commit, and acme-3 stays Accepted, Partly landed.
func TestLandTakesARepositoryThroughEveryDoor(t *testing.T) {
	for door, land := range map[string]func(*testing.T, *codecentraltest.Server){
		"mcp": func(t *testing.T, srv *codecentraltest.Server) {
			mcpOK(t, connect(t, srv, "acme"), "land", map[string]any{"ticket": "acme-3", "commit": "m-api", "repository": "api"})
		},
		"http": func(t *testing.T, srv *codecentraltest.Server) {
			httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/3/land", `{"commit":"m-api","repository":"api"}`})
		},
		"cli": func(t *testing.T, srv *codecentraltest.Server) {
			cliOK(t, srv, []string{"land", "acme-3", "--commit", "m-api", "--repository", "api"})
		},
	} {
		t.Run(door, func(t *testing.T) {
			srv := newServer(t, acceptedInAPI)
			land(t, srv)
			if state, commit := landedIn(t, srv, "api"); state != "Accepted" || commit != "m-api" {
				t.Fatalf("acme-3 %s with api landed at %q, want Accepted with api at m-api", state, commit)
			}
		})
	}
}

// A land in a Repository the Ticket didn't change is refused through
// every door alike, naming its Changed Repositories.
func TestLandRefusesARepositoryTheTicketDidntChangeThroughEveryDoor(t *testing.T) {
	srv := newServer(t, acceptedInAPI)
	must(t, func() error {
		_, err := srv.Core.AddRepository(core.WithActor(context.Background(), core.Human), "acme", core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git"})
		return err
	})
	_, b := httpDo(t, srv, httpCall{"POST", "/api/p/acme/tickets/3/land", `{"commit":"x","repository":"web"}`})
	if e := errorJSON(t, b, core.Invalid); !strings.Contains(e.Message, "acme, api") {
		t.Fatalf("http refusal %+v, want it to name the Changed Repositories", e)
	}
	res, mb := mcpCall(t, connect(t, srv, "acme"), "land", map[string]any{"ticket": "acme-3", "commit": "x", "repository": "web"})
	if !res.IsError || !strings.Contains(string(mb), "acme, api") {
		t.Fatalf("mcp: %s, want the refusal naming the Changed Repositories", mb)
	}
	code, _, errOut := cliRun(t, srv, []string{"land", "acme-3", "--commit", "x", "--repository", "web"})
	if code == 0 || !strings.Contains(string(errOut), "acme, api") {
		t.Fatalf("cli: exit %d, %s, want the refusal naming the Changed Repositories", code, errOut)
	}
}

// The Factory's push and the Pull Request Watcher's pull request in api
// are recorded in api over HTTP, and an unknown Repository is refused.
func TestThePushAndThePullRequestOfARepositoryAreRecordedInIt(t *testing.T) {
	srv := newServer(t, acceptedInAPI)
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/3/report_pushed_commit", `{"commit":"def456","branch":"acme-3","repository":"api"}`})
	httpOK(t, srv, reportPullRequest(`{"repository":"api","branch":"acme-3","number":4,"link":"https://github.com/acme/api/pull/4","state":"open"}`))
	tk := httpOK(t, srv, httpCall{"GET", "/api/p/acme/tickets/3", ""})
	api := tk["repositories"].([]any)[1].(map[string]any)
	pushed, _ := api["pushed_commit"].(map[string]any)
	pr, _ := api["pull_request"].(map[string]any)
	if api["name"] != "api" || pushed["commit"] != "def456" || pr["number"] != float64(4) {
		t.Fatalf("api = %v, want its pushed commit and pull request", api)
	}

	for _, c := range []httpCall{
		{"POST", "/api/p/acme/tickets/3/report_pushed_commit", `{"commit":"x","repository":"nope"}`},
		reportPullRequest(`{"repository":"nope","branch":"acme-3","number":4,"link":"l","state":"open"}`),
	} {
		_, b := httpDo(t, srv, c)
		errorJSON(t, b, core.Invalid)
	}
}
