package server_test

import (
	"context"
	"fmt"
	"github.com/tomoram/code-central/domain"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/httpapi"
	"github.com/tomoram/code-central/store"
)

// Azure Boards settings (Spec code-central-894): set_azure_boards is the
// human's, so HTTP only. Turning it on first creates the Project's Azure
// Boards Spec, acme-4 on the cross-door server.
func init() {
	cases["set_azure_boards"] = opCase{
		ok: call{http: setAzureBoards(`{"on":true,"mode":"saved_query","query_url":" ` + webQuery + ` ","new_issues_need_me":true}`)},
		check: func(t *testing.T, out map[string]any) {
			acknowledgement("acme", "azure_boards")(t, out)
		},
		full: func(t *testing.T, out map[string]any) {
			field("key", "acme")(t, out)
			azureBoards(t, out, boardsSettings{on: true, mode: "saved_query", queryURL: webQuery, patEnv: "ADO_PAT", needMe: true, spec: "acme-4"})
		},
		bad:  call{http: setAzureBoards(`{"on":true,"mode":"saved_query"}`)},
		code: core.Invalid,
	}
}

// webQuery and apiQuery are saved queries in contoso's web and api
// projects.
const (
	webQuery = "https://dev.azure.com/contoso/web/_queries/query/6b4a0c1e-9f1d-4a51-8f43-2a7d3c1b9e10"
	apiQuery = "https://dev.azure.com/contoso/api/_queries/query/0d3c2b1a-1111-4222-8333-944455566677"
)

func setAzureBoards(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/set_azure_boards", body}
}

// boardsSettings is a Project's azure_boards object.
type boardsSettings struct {
	on         bool
	mode       string
	queryURL   string
	projectURL string
	where      string
	patEnv     string
	needMe     bool
	hide       bool
	format     string
	spec       string
}

// boardsOff is a Project's azure_boards object before the human sets it.
var boardsOff = boardsSettings{mode: "saved_query", patEnv: "ADO_PAT"}

// azureBoards checks a Project's azure_boards object is want.
func azureBoards(t *testing.T, project map[string]any, want boardsSettings) {
	t.Helper()
	a, _ := project["azure_boards"].(map[string]any)
	str := func(k string) string { s, _ := a[k].(string); return s }
	got := boardsSettings{on: a["on"] == true, mode: str("mode"), queryURL: str("query_url"), projectURL: str("project_url"), where: str("where"),
		patEnv: str("pat_env"), needMe: a["new_issues_need_me"] == true, hide: a["hide_code_central"] == true, format: str("branch_format"), spec: str("spec")}
	if a == nil || got != want {
		t.Fatalf("azure_boards = %v, want %+v", project["azure_boards"], want)
	}
}

// acmeBoards checks acme's Azure Boards settings, as a Project read gives
// them.
func acmeBoards(t *testing.T, srv *codecentraltest.Server, want boardsSettings) {
	t.Helper()
	azureBoards(t, httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""}), want)
}

func TestEveryProjectReadCarriesAzureBoardsOff(t *testing.T) {
	srv := newServer(t)
	acmeBoards(t, srv, boardsOff)
	for _, row := range httpOK(t, srv, httpCall{"GET", "/api/projects", ""})["rows"].([]any) {
		azureBoards(t, row.(map[string]any), boardsOff)
	}
}

func TestSetAzureBoardsIsRejectedForAnAgent(t *testing.T) {
	srv := newServer(t)
	status, b := httpDoAs(t, srv, core.Agent, setAzureBoards(`{"on":true,"mode":"saved_query","query_url":"`+webQuery+`"}`))
	if e := errorJSON(t, b, core.ActionNotAllowed); status != httpapi.Status(core.ActionNotAllowed) || !strings.Contains(e.Message, "human") {
		t.Fatalf("status %d, want %d; message %q", status, httpapi.Status(core.ActionNotAllowed), e.Message)
	}
	acmeBoards(t, srv, boardsOff)
}

func TestAnArchivedProjectRejectsSetAzureBoards(t *testing.T) {
	srv := newServer(t)
	human := core.WithActor(context.Background(), core.Human)
	must(t, func() error { _, err := srv.Core.ArchiveProject(human, "other"); return err })
	httpRejected(t, srv, httpCall{"POST", "/api/p/other/set_azure_boards", `{"on":false}`}, core.ActionNotAllowed)
	httpRejected(t, srv, httpCall{"POST", "/api/p/nope/set_azure_boards", `{"on":false}`}, core.UnknownProject)
}

func TestTurningAzureBoardsOnNeedsTheChosenModesSettings(t *testing.T) {
	srv := newServer(t)
	for _, c := range []struct{ body, field, says string }{
		{`{"on":true}`, "query_url", "saved query"},
		{`{"on":true,"mode":"saved_query","query_url":" "}`, "query_url", "saved query"},
		{`{"on":true,"mode":"wiql","where":"[System.State] = 'Approved'"}`, "project_url", "project"},
		{`{"on":true,"mode":"wiql","project_url":"https://dev.azure.com/contoso/web","where":"  "}`, "where", "WHERE clause"},
		{`{"on":true,"mode":"query","query_url":"` + webQuery + `"}`, "mode", "saved_query"},
	} {
		e := httpRejected(t, srv, setAzureBoards(c.body), core.Invalid)
		if e.Field != c.field || !strings.Contains(e.Message, c.says) {
			t.Fatalf("%s: field %q message %q, want %s saying %q", c.body, e.Field, e.Message, c.field, c.says)
		}
	}
	acmeBoards(t, srv, boardsOff)
}

func TestAzureBoardsSettingsAreCheckedAndTidied(t *testing.T) {
	srv := newServer(t)
	for _, c := range []struct{ body, field, says string }{
		{`{"on":true,"query_url":"https://dev.azure.com/contoso/web"}`, "query_url", "_queries/query"},
		{`{"on":true,"mode":"wiql","project_url":"dev.azure.com/contoso","where":"[System.Id] > 0"}`, "project_url", "https://"},
		{`{"on":true,"mode":"wiql","project_url":"https://dev.azure.com/contoso/web","where":"SELECT [System.Id] FROM workitems"}`, "where", "only the WHERE clause"},
		{`{"on":true,"mode":"wiql","project_url":"https://dev.azure.com/contoso/web","where":"WHERE [System.Id] > 0"}`, "where", "only the WHERE clause"},
		{`{"on":true,"query_url":"` + webQuery + `","pat_env":"MY-PAT"}`, "pat_env", "environment variable"},
	} {
		e := httpRejected(t, srv, setAzureBoards(c.body), core.Invalid)
		if e.Field != c.field || !strings.Contains(e.Message, c.says) {
			t.Fatalf("%s: field %q message %q, want %s saying %q", c.body, e.Field, e.Message, c.field, c.says)
		}
	}
	// A browser's URLs are cut to the project, and the query's to its ID.
	out := httpOK(t, srv, setAzureBoards(`{"on":true,"mode":"wiql","query_url":"`+webQuery+`/?view=1","project_url":" https://dev.azure.com/contoso/web/_boards ",`+
		`"where":"  [System.State] = 'Approved'\nAND [System.AssignedTo] = @Me  ","pat_env":" BOARDS_PAT ","full":true}`))
	azureBoards(t, out, boardsSettings{on: true, mode: "wiql", queryURL: webQuery, projectURL: "https://dev.azure.com/contoso/web",
		where: "[System.State] = 'Approved'\nAND [System.AssignedTo] = @Me", patEnv: "BOARDS_PAT", spec: "acme-4"})
}

func TestTheSettingsOfTheModeNotChosenAreKept(t *testing.T) {
	srv := newServer(t)
	httpOK(t, srv, setAzureBoards(`{"on":true,"mode":"wiql","query_url":"`+webQuery+`","project_url":"https://dev.azure.com/contoso/api","where":"[System.Id] > 0"}`))
	httpOK(t, srv, setAzureBoards(`{"on":true,"mode":"saved_query","query_url":"`+webQuery+`","project_url":"https://dev.azure.com/contoso/api","where":"[System.Id] > 0"}`))
	acmeBoards(t, srv, boardsSettings{on: true, mode: "saved_query", queryURL: webQuery, projectURL: "https://dev.azure.com/contoso/api", where: "[System.Id] > 0", patEnv: "ADO_PAT", spec: "acme-4"})

	// Off, any mode's settings may be left empty, or kept for later.
	httpOK(t, srv, setAzureBoards(`{"on":false,"mode":"wiql"}`))
	acmeBoards(t, srv, boardsSettings{mode: "wiql", patEnv: "ADO_PAT", spec: "acme-4"})
}

func TestHideCodeCentralIsSavedShownAndKeptWhileAzureBoardsIsOff(t *testing.T) {
	srv := newServer(t)
	httpOK(t, srv, setAzureBoards(`{"on":true,"query_url":"`+webQuery+`","hide_code_central":true}`))
	on := boardsSettings{on: true, mode: "saved_query", queryURL: webQuery, patEnv: "ADO_PAT", hide: true, spec: "acme-4"}
	acmeBoards(t, srv, on)

	// Off, it is kept with the other settings, and it saves with them.
	httpOK(t, srv, setAzureBoards(`{"on":false,"query_url":"`+webQuery+`","hide_code_central":true}`))
	on.on = false
	acmeBoards(t, srv, on)
	httpOK(t, srv, setAzureBoards(`{"on":false,"query_url":"`+webQuery+`"}`))
	on.hide = false
	acmeBoards(t, srv, on)
}

func TestChangingHideCodeCentralAloneRunsNoCheckAndKeepsTheWatchersStatus(t *testing.T) {
	checks := &boardsChecks{}
	srv := newBoardsCheckServer(t, checks)
	project := func() map[string]any { return httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""}) }
	httpOK(t, srv, setAzureBoards(`{"on":true,"query_url":"`+webQuery+`"}`))
	httpOK(t, srv, reportWorkItemWatcherStatus(`{"state":"error","problem":"pat_rejected","message":"Azure DevOps rejected the PAT","fix":"Renew it."}`))

	httpOK(t, srv, setAzureBoards(`{"on":true,"query_url":"`+webQuery+`","hide_code_central":true}`))
	if n := len(checks.ran()); n != 1 {
		t.Fatalf("changing only Hide Code Central ran %d checks, want the 1 of turning on", n)
	}
	workItemWatcher(t, project(), "error", "Azure DevOps rejected the PAT", "Renew it.")
	azureBoards(t, project(), boardsSettings{on: true, mode: "saved_query", queryURL: webQuery, patEnv: "ADO_PAT", hide: true, spec: "acme-4"})
}

func TestChangingTheBranchFormatAloneRunsNoCheckAndKeepsTheWatchersStatus(t *testing.T) {
	checks := &boardsChecks{}
	srv := newBoardsCheckServer(t, checks)
	project := func() map[string]any { return httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""}) }
	httpOK(t, srv, setAzureBoards(`{"on":true,"query_url":"`+webQuery+`"}`))
	httpOK(t, srv, reportWorkItemWatcherStatus(`{"state":"error","problem":"pat_rejected","message":"Azure DevOps rejected the PAT","fix":"Renew it."}`))

	httpRejected(t, srv, setAzureBoards(`{"on":true,"query_url":"`+webQuery+`","branch_format":"feat/x"}`), core.Invalid)
	httpOK(t, srv, setAzureBoards(`{"on":true,"query_url":"`+webQuery+`","branch_format":"feat/{id}"}`))
	if n := len(checks.ran()); n != 1 {
		t.Fatalf("changing only the Branch format ran %d checks, want the 1 of turning on", n)
	}
	workItemWatcher(t, project(), "error", "Azure DevOps rejected the PAT", "Renew it.")
	azureBoards(t, project(), boardsSettings{on: true, mode: "saved_query", queryURL: webQuery, patEnv: "ADO_PAT", format: "feat/{id}", spec: "acme-4"})
}

func TestGitHubIssuesHasNoHideCodeCentral(t *testing.T) {
	srv := newServer(t)
	httpRejected(t, srv, httpCall{"POST", "/api/p/acme/set_github_issues", `{"on":false,"hide_code_central":true}`}, core.Invalid)
	if _, ok := httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""})["github_issues"].(map[string]any)["hide_code_central"]; ok {
		t.Fatal("github_issues carries hide_code_central")
	}
}

func TestTheAzureBoardsPATVariableDefaultsToTheCIOne(t *testing.T) {
	srv := newServer(t)
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/set_ci_tracking", `{"tracking":false,"provider":"azure_pipelines","pat_env":"PIPELINES_PAT"}`})
	acmeBoards(t, srv, boardsSettings{mode: "saved_query", patEnv: "PIPELINES_PAT"})
	httpOK(t, srv, setAzureBoards(`{"on":false,"pat_env":"BOARDS_PAT"}`))
	acmeBoards(t, srv, boardsSettings{mode: "saved_query", patEnv: "BOARDS_PAT"})
}

func TestTurningAzureBoardsOnCreatesTheAzureBoardsSpecOnce(t *testing.T) {
	srv := newServer(t)
	before := specCount(t, srv)
	out := httpOK(t, srv, setAzureBoards(`{"on":true,"query_url":"`+webQuery+`","full":true}`))
	azureBoards(t, out, boardsSettings{on: true, mode: "saved_query", queryURL: webQuery, patEnv: "ADO_PAT", spec: "acme-4"})
	s := issuesSpec(t, srv, 4)
	if s["title"] != "Azure Boards: contoso/web" || s["link"] != nil || fmt.Sprint(s["labels"]) != "[long-lived]" || s["requirement"] != nil || !strings.Contains(s["body"].(string), "Incoming Issue") {
		t.Fatalf("Azure Boards Spec = %v, want a long-lived Spec titled for contoso/web, with no Link and no Stages", s)
	}

	// Off and on again, and on with other settings, reuse it.
	httpOK(t, srv, setAzureBoards(`{"on":false,"query_url":"`+webQuery+`"}`))
	acmeBoards(t, srv, boardsSettings{mode: "saved_query", queryURL: webQuery, patEnv: "ADO_PAT", spec: "acme-4"})
	httpOK(t, srv, setAzureBoards(`{"on":true,"query_url":"`+webQuery+`","new_issues_need_me":true}`))
	acmeBoards(t, srv, boardsSettings{on: true, mode: "saved_query", queryURL: webQuery, patEnv: "ADO_PAT", needMe: true, spec: "acme-4"})
	if n := specCount(t, srv); n != before+1 {
		t.Fatalf("%d Specs, want %d: one Azure Boards Spec", n, before+1)
	}
}

func TestTheAzureBoardsSpecsTitleFollowsTheSettings(t *testing.T) {
	srv := newServer(t)
	title := func() any { return issuesSpec(t, srv, 4)["title"] }
	httpOK(t, srv, setAzureBoards(`{"on":true,"query_url":"`+webQuery+`"}`))
	httpOK(t, srv, setAzureBoards(`{"on":true,"query_url":"`+apiQuery+`"}`))
	if got := title(); got != "Azure Boards: contoso/api" {
		t.Fatalf("title %v, want it to follow the query", got)
	}
	// The chosen mode's project names it.
	httpOK(t, srv, setAzureBoards(`{"on":true,"mode":"wiql","query_url":"`+apiQuery+`","project_url":"https://fabrikam.visualstudio.com/My%20Site","where":"[System.Id] > 0"}`))
	if got := title(); got != "Azure Boards: fabrikam/My Site" {
		t.Fatalf("title %v, want it to follow the WIQL filter's project", got)
	}
	// Off, the settings may still change, and the title follows them.
	httpOK(t, srv, setAzureBoards(`{"on":false,"query_url":"`+webQuery+`"}`))
	if got := title(); got != "Azure Boards: contoso/web" {
		t.Fatalf("title %v, want it to follow the query while off", got)
	}
	// Off, they may be cleared; the next ones still retitle it.
	httpOK(t, srv, setAzureBoards(`{"on":false}`))
	httpOK(t, srv, setAzureBoards(`{"on":true,"query_url":"`+apiQuery+`"}`))
	if got := title(); got != "Azure Boards: contoso/api" {
		t.Fatalf("title %v, want it to follow a query set after clearing one", got)
	}
}

func TestChangingAzureBoardsKeepsTheHumansTitle(t *testing.T) {
	srv := newServer(t)
	httpOK(t, srv, setAzureBoards(`{"on":true,"query_url":"`+webQuery+`"}`))
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/specs/4/edit_spec", `{"title":"Approved PBIs"}`})
	httpOK(t, srv, setAzureBoards(`{"on":true,"query_url":"`+apiQuery+`"}`))
	if s := issuesSpec(t, srv, 4); s["title"] != "Approved PBIs" {
		t.Fatalf("title %v, want the human's kept", s["title"])
	}
}

func TestTurningAzureBoardsOffLeavesEveryLink(t *testing.T) {
	srv := newServer(t, linkedItems)
	httpOK(t, srv, setAzureBoards(`{"on":true,"query_url":"`+webQuery+`"}`))
	as := core.WithActor(t.Context(), core.Integration)
	must(t, func() error {
		_, err := srv.Core.CreateLinkedTicket(as, "acme", core.CreateLinkedTicketInput{Spec: "6", Title: "Work item", Link: core.LinkInput{Integration: "azure-boards", ID: "contoso#4512"}})
		return err
	})
	links := func() []any {
		return []any{issuesSpec(t, srv, 4)["link"], httpOK(t, srv, httpCall{"GET", "/api/p/acme/tickets/5", ""})["link"],
			httpOK(t, srv, httpCall{"GET", "/api/p/acme/tickets/7", ""})["link"]}
	}
	before := links()
	httpOK(t, srv, setAzureBoards(`{"on":false,"query_url":"`+webQuery+`"}`))
	after := links()
	for i := range before {
		if before[i] == nil || !mapsEqual(before[i].(map[string]any), after[i].(map[string]any)) {
			t.Fatalf("links after turning off = %v, want %v", after, before)
		}
	}
}

func TestGitHubIssuesAndAzureBoardsAreOnAtOnce(t *testing.T) {
	srv := newServer(t)
	httpOK(t, srv, setGitHubIssues(`{"on":true,"repositories":["acme/web"]}`))
	out := httpOK(t, srv, setAzureBoards(`{"on":true,"query_url":"`+webQuery+`","full":true}`))
	githubIssues(t, out, issuesSettings{on: true, repositories: "acme/web", spec: "acme-4"})
	azureBoards(t, out, boardsSettings{on: true, mode: "saved_query", queryURL: webQuery, patEnv: "ADO_PAT", spec: "acme-5"})
	if a, b := issuesSpec(t, srv, 4)["title"], issuesSpec(t, srv, 5)["title"]; a != "GitHub Issues: acme/web" || b != "Azure Boards: contoso/web" {
		t.Fatalf("titles %v and %v, want an Issues Spec and an Azure Boards Spec", a, b)
	}
	// Turning one off leaves the other on.
	out = httpOK(t, srv, setGitHubIssues(`{"on":false,"repositories":["acme/web"],"full":true}`))
	azureBoards(t, out, boardsSettings{on: true, mode: "saved_query", queryURL: webQuery, patEnv: "ADO_PAT", spec: "acme-5"})
}

func TestTheSameAzureBoardsSettingsWriteNothing(t *testing.T) {
	srv := newServer(t)
	if out := httpOK(t, srv, setAzureBoards(`{"on":false}`)); len(toStrings(out["changed"])) != 0 {
		t.Fatalf("the default settings changed %v", out["changed"])
	}
	var stored domain.AzureBoards
	must(t, func() error {
		return srv.Store.View(func(r store.Reader) error { p, _ := r.Project("acme"); stored = p.AzureBoards; return nil })
	})
	if stored != (domain.AzureBoards{}) {
		t.Fatalf("the default settings were stored as %+v, want nothing set", stored)
	}
	body := `{"on":true,"query_url":"` + webQuery + `"}`
	httpOK(t, srv, setAzureBoards(body))
	if out := httpOK(t, srv, setAzureBoards(body)); len(toStrings(out["changed"])) != 0 {
		t.Fatalf("acknowledgement = %v, want nothing changed", out)
	}
}

// boardsChecks is a fake Azure Boards check: it records each check and
// answers with result.
type boardsChecks struct {
	mu     sync.Mutex
	calls  []core.AzureBoardsCheckInput
	result core.WatcherCheckResult
}

func (c *boardsChecks) check(_ context.Context, in core.AzureBoardsCheckInput) core.WatcherCheckResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, in)
	return c.result
}

func (c *boardsChecks) ran() []core.AzureBoardsCheckInput {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.calls)
}

func newBoardsCheckServer(t *testing.T, checks *boardsChecks) *codecentraltest.Server {
	t.Helper()
	return newServerWith(t, []codecentraltest.Option{codecentraltest.WithAzureBoardsCheck(checks.check)})
}

func TestTurningAzureBoardsOnRunsTheCheckOnTheChosenModesSettings(t *testing.T) {
	checks := &boardsChecks{}
	srv := newBoardsCheckServer(t, checks)
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/set_ci_tracking", `{"tracking":false,"provider":"azure_pipelines","pat_env":"PIPELINES_PAT"}`})
	httpOK(t, srv, setAzureBoards(`{"on":true,"query_url":"`+webQuery+`","project_url":"https://dev.azure.com/contoso/api","where":"[System.Id] > 0"}`))
	want := []core.AzureBoardsCheckInput{{Mode: domain.AzureBoardsSavedQuery, QueryURL: webQuery, PATEnv: "PIPELINES_PAT"}}
	if got := checks.ran(); !reflect.DeepEqual(got, want) {
		t.Fatalf("checks = %+v, want %+v", got, want)
	}
	httpOK(t, srv, setAzureBoards(`{"on":true,"mode":"wiql","query_url":"`+webQuery+`","project_url":"https://dev.azure.com/contoso/api","where":"[System.Id] > 0","pat_env":"BOARDS_PAT"}`))
	want = append(want, core.AzureBoardsCheckInput{Mode: domain.AzureBoardsWIQL, ProjectURL: "https://dev.azure.com/contoso/api", Where: "[System.Id] > 0", PATEnv: "BOARDS_PAT"})
	if got := checks.ran(); !reflect.DeepEqual(got, want) {
		t.Fatalf("checks = %+v, want %+v", got, want)
	}
}

func TestAFailedAzureBoardsCheckRefusesWithItsProblemAndFix(t *testing.T) {
	checks := &boardsChecks{result: core.WatcherCheckResult{Problem: core.ProblemCheckFailed, Reason: "The environment variable ADO_PAT is not set.", Fix: "Set ADO_PAT for the service."}}
	srv := newBoardsCheckServer(t, checks)
	before := specCount(t, srv)
	e := httpRejected(t, srv, setAzureBoards(`{"on":true,"query_url":"`+webQuery+`"}`), core.Invalid)
	if e.Field != "on" || e.Message != "The environment variable ADO_PAT is not set." || e.Fix != "Set ADO_PAT for the service." {
		t.Fatalf("refusal = %+v, want the check's problem and fix on on", e)
	}
	acmeBoards(t, srv, boardsOff)
	if n := specCount(t, srv); n != before {
		t.Fatalf("a failed check made %d Specs", n-before)
	}

	// While on, a change that fails the check is refused too, and the
	// settings stay as they were.
	checks.result = core.WatcherCheckResult{}
	httpOK(t, srv, setAzureBoards(`{"on":true,"query_url":"`+webQuery+`"}`))
	checks.result = core.WatcherCheckResult{Problem: core.ProblemCheckFailed, Reason: "TF51005: The query references a field that does not exist.", Fix: "Fix the WHERE clause."}
	e = httpRejected(t, srv, setAzureBoards(`{"on":true,"mode":"wiql","project_url":"https://dev.azure.com/contoso/web","where":"[Nope] = 1"}`), core.Invalid)
	if !strings.Contains(e.Message, "TF51005") || e.Fix != "Fix the WHERE clause." {
		t.Fatalf("refusal = %+v, want the check's problem and fix", e)
	}
	acmeBoards(t, srv, boardsSettings{on: true, mode: "saved_query", queryURL: webQuery, patEnv: "ADO_PAT", spec: "acme-4"})
}

func TestOnlyTurningAzureBoardsOnOrChangingWhatItRunsRunsTheCheck(t *testing.T) {
	checks := &boardsChecks{}
	srv := newBoardsCheckServer(t, checks)
	httpOK(t, srv, setAzureBoards(`{"on":false,"query_url":"`+webQuery+`"}`))
	if n := len(checks.ran()); n != 0 {
		t.Fatalf("leaving Azure Boards off ran %d checks", n)
	}
	httpOK(t, srv, setAzureBoards(`{"on":true,"query_url":"`+webQuery+`"}`))
	httpOK(t, srv, setAzureBoards(`{"on":true,"query_url":"`+webQuery+`","new_issues_need_me":true,"project_url":"https://dev.azure.com/contoso/api"}`))
	if n := len(checks.ran()); n != 1 {
		t.Fatalf("turning on, then changing only New issues need me and the other mode's settings, ran %d checks, want 1", n)
	}
	httpOK(t, srv, setAzureBoards(`{"on":true,"query_url":"`+apiQuery+`"}`))
	httpOK(t, srv, setAzureBoards(`{"on":true,"query_url":"`+apiQuery+`","pat_env":"BOARDS_PAT"}`))
	if got := checks.ran(); len(got) != 3 || got[1].QueryURL != apiQuery || got[2].PATEnv != "BOARDS_PAT" {
		t.Fatalf("checks = %+v, want one for the new query and one for the new PAT variable", got)
	}

	// Turning off runs no check, even when the check would fail.
	checks.result = core.WatcherCheckResult{Problem: core.ProblemCheckFailed, Reason: "no PAT", Fix: "Set it."}
	acmeOff := httpOK(t, srv, setAzureBoards(`{"on":false,"query_url":"`+apiQuery+`","full":true}`))
	azureBoards(t, acmeOff, boardsSettings{mode: "saved_query", queryURL: apiQuery, patEnv: "ADO_PAT", spec: "acme-4"})
	if n := len(checks.ran()); n != 3 {
		t.Fatalf("turning off ran a check: %d checks", n)
	}
}
