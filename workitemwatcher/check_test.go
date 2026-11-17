package workitemwatcher_test

import (
	"errors"
	"net/http"
	"runtime"
	"strings"
	"testing"

	"github.com/tomoram/code-central/ciwatcher/ciwatchertest"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/internal/fakeazdo"
	"github.com/tomoram/code-central/workitemwatcher"
)

// checking is a real Code Central whose Azure Boards check runs the test
// binary as code-central, as the service runs its own executable, with
// the PAT in ADO_PAT, over a fake Azure DevOps whose saved query queryID
// matches work item 7.
type checking struct {
	t    *testing.T
	fake *fakeazdo.Server
	srv  *codecentraltest.Server
}

func newChecking(t *testing.T, environ ...string) *checking {
	t.Helper()
	h := ciwatchertest.New(t)
	srv := codecentraltest.NewServer(t, codecentraltest.WithAzureBoardsCheck(workitemwatcher.Checker(h.CodeCentral, append(h.Environ(), environ...))))
	if _, err := srv.Core.CreateProject(human, core.CreateProjectInput{Key: "acme"}); err != nil {
		t.Fatal(err)
	}
	c := &checking{t: t, fake: fakeazdo.New(t), srv: srv}
	c.fake.PutWorkItems(fakeazdo.WorkItem{ID: 7, Title: "Seven", Type: "Bug", State: "New"})
	c.fake.SavedQuery(queryID, 7)
	return c
}

// turnOn is the human turning Azure Boards on for the saved query.
func (c *checking) turnOn() error {
	_, err := c.srv.Core.SetAzureBoards(human, "acme", core.SetAzureBoardsInput{On: true, QueryURL: c.fake.ProjectURL() + "/_queries/query/" + queryID})
	return err
}

// refused is err as the check's refusal.
func refused(t *testing.T, err error) *core.Error {
	t.Helper()
	var e *core.Error
	if !errors.As(err, &e) || e.Code != core.Invalid || e.Field != "on" {
		t.Fatalf("err = %v, want Azure Boards refused by its check", err)
	}
	return e
}

func (c *checking) boards() core.AzureBoardsView {
	c.t.Helper()
	p, err := c.srv.Client.GetProject(human, "acme")
	if err != nil {
		c.t.Fatal(err)
	}
	return p.AzureBoards
}

func TestAzureBoardsTurnsOnWhenTheCheckPassesAndWritesNothing(t *testing.T) {
	c := newChecking(t, "ADO_PAT="+pat)
	if err := c.turnOn(); err != nil {
		t.Fatalf("turning on: %v", err)
	}
	if b := c.boards(); !b.On || b.Spec == "" {
		t.Fatalf("settings = %+v, want on with its Azure Boards Spec", b)
	}
	var methods []string
	for _, r := range c.fake.Requests() {
		methods = append(methods, r.Method+" "+strings.TrimPrefix(r.Path, fakeazdo.CollectionPath))
	}
	want := []string{"GET /" + fakeazdo.Project + "/_apis/wit/wiql/" + queryID, "PATCH /_apis/wit/workitems/7"}
	if strings.Join(methods, ",") != strings.Join(want, ",") {
		t.Fatalf("requests = %q, want %q", methods, want)
	}
	patch := c.fake.Requests()[1]
	if patch.Query["validateOnly"][0] != "true" || patch.ContentType != "application/json-patch+json" ||
		!strings.Contains(patch.Body, "/fields/System.Tags") || patch.Authorization == "" {
		t.Fatalf("write check = %+v, want a validateOnly JSON Patch of System.Tags with the PAT", patch)
	}
	if got := c.fake.WorkItem(7); got.Title != "Seven" || got.Rev != 0 {
		t.Fatalf("work item 7 = %+v, want it untouched", got)
	}
}

func TestWithNoResultsTheWriteCheckIsLeftToTheFirstWrite(t *testing.T) {
	c := newChecking(t, "ADO_PAT="+pat)
	c.fake.SavedQuery(queryID)
	if err := c.turnOn(); err != nil {
		t.Fatalf("turning on: %v", err)
	}
	for _, r := range c.fake.Requests() {
		if r.Method == http.MethodPatch {
			t.Fatalf("request %+v, want no write check with no results", r)
		}
	}
}

func TestEachFailedAzureBoardsCheckRefusesWithItsProblemAndFix(t *testing.T) {
	for _, tc := range []struct {
		name        string
		environ     []string
		setup       func(c *checking)
		problem     string
		reason, fix string
	}{
		{"no PAT", nil, func(*checking) {}, "pat_missing", "ADO_PAT is not set",
			map[string]string{"darwin": "launchctl setenv ADO_PAT", "linux": "systemctl --user set-environment", "windows": "user environment variable"}[runtime.GOOS]},
		{"empty PAT", []string{"ADO_PAT="}, func(*checking) {}, "pat_missing", "ADO_PAT is not set", "Work Items: Read & write"},
		{"rejected PAT", []string{"ADO_PAT=" + pat}, func(c *checking) {
			c.fake.Script(fakeazdo.JSON("", fakeazdo.ProjectPath+"/_apis/wit/wiql", http.StatusUnauthorized, map[string]string{}))
		}, "pat_rejected", "rejected the PAT in ADO_PAT", "new PAT"},
		{"missing scope", []string{"ADO_PAT=" + pat}, func(c *checking) {
			c.fake.Script(fakeazdo.JSON(http.MethodPatch, fakeazdo.CollectionPath+"/_apis/wit/workitems/", http.StatusForbidden, map[string]string{}))
		}, "pat_scope", "can read work items but not write them", "Work Items: Read & write"},
		{"saved query missing", []string{"ADO_PAT=" + pat}, func(c *checking) { c.fake.DeleteSavedQuery(queryID) },
			"query_unusable", "can't find the saved query", "saved query's URL"},
		{"tree query", []string{"ADO_PAT=" + pat}, func(c *checking) {
			c.fake.Script(fakeazdo.JSON("GET", fakeazdo.ProjectPath+"/_apis/wit/wiql/", 0, map[string]any{"queryType": "tree", "workItemRelations": []any{}}))
		}, "query_unusable", "is a tree query", "flat list"},
		{"one-hop query", []string{"ADO_PAT=" + pat}, func(c *checking) {
			c.fake.Script(fakeazdo.JSON("GET", fakeazdo.ProjectPath+"/_apis/wit/wiql/", 0, map[string]any{"queryType": "oneHop", "workItemRelations": []any{}}))
		}, "query_unusable", "is a links (one-hop) query", "flat list"},
		{"query refused", []string{"ADO_PAT=" + pat}, func(c *checking) {
			c.fake.Script(fakeazdo.JSON("GET", fakeazdo.ProjectPath+"/_apis/wit/wiql/", http.StatusBadRequest,
				map[string]string{"message": "TF51005: The query references a field that does not exist."}))
		}, "query_unusable", "TF51005: The query references a field that does not exist.", "Fix the query"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newChecking(t, tc.environ...)
			tc.setup(c)
			e := refused(t, c.turnOn())
			if !strings.Contains(e.Message, tc.reason) || !strings.Contains(e.Fix, tc.fix) {
				t.Fatalf("refusal %q, fix %q; want %q and %q", e.Message, e.Fix, tc.reason, tc.fix)
			}
			if b := c.boards(); b.On || b.Spec != "" {
				t.Fatalf("a failed check left %+v, want Azure Boards off with no Spec", b)
			}
		})
	}
}

func TestAWIQLErrorRefusesWithAzureDevOpsOwnMessage(t *testing.T) {
	c := newChecking(t, "ADO_PAT="+pat)
	_, err := c.srv.Core.SetAzureBoards(human, "acme", core.SetAzureBoardsInput{On: true, Mode: "wiql", ProjectURL: c.fake.ProjectURL(), Where: "[Nope] = 1"})
	e := refused(t, err)
	if !strings.Contains(e.Message, "TF51005: The query references a field that does not exist.") || !strings.Contains(e.Fix, "WHERE clause") {
		t.Fatalf("refusal %q, fix %q; want Azure DevOps' own message and the WHERE clause to fix", e.Message, e.Fix)
	}
}

func TestAWIQLFilterPassesTheCheckAndTheWriteCheckGoesToItsFirstResult(t *testing.T) {
	c := newChecking(t, "ADO_PAT="+pat)
	c.fake.PutWorkItems(fakeazdo.WorkItem{ID: 9, Title: "Nine", Type: "Bug", State: "Approved"})
	c.fake.Filter("[System.State] = 'Approved'", 9, 7)
	_, err := c.srv.Core.SetAzureBoards(human, "acme", core.SetAzureBoardsInput{On: true, Mode: "wiql", ProjectURL: c.fake.ProjectURL(), Where: "[System.State] = 'Approved'"})
	if err != nil {
		t.Fatalf("turning on: %v", err)
	}
	reqs := c.fake.Requests()
	if last := reqs[len(reqs)-1]; last.Method != http.MethodPatch || !strings.HasSuffix(last.Path, "/workitems/9") {
		t.Fatalf("last request = %+v, want the write check on the first result, 9", last)
	}
}

func TestAChangeWhileOnIsCheckedAndRefusedLeavingTheSettingsAsTheyWere(t *testing.T) {
	c := newChecking(t, "ADO_PAT="+pat)
	if err := c.turnOn(); err != nil {
		t.Fatal(err)
	}
	_, err := c.srv.Core.SetAzureBoards(human, "acme", core.SetAzureBoardsInput{On: true, QueryURL: c.fake.ProjectURL() + "/_queries/query/00000000-0000-0000-0000-000000000000"})
	if e := refused(t, err); !strings.Contains(e.Message, "can't find the saved query") {
		t.Fatalf("refusal %q, want the saved query not found", e.Message)
	}
	if b := c.boards(); !strings.HasSuffix(b.QueryURL, queryID) {
		t.Fatalf("settings = %+v, want the old query kept", b)
	}
}

func TestThePATVariableDefaultsToADOPATAndCanBeNamed(t *testing.T) {
	c := newChecking(t, "OTHER_PAT="+pat)
	if e := refused(t, c.turnOn()); !strings.Contains(e.Message, "ADO_PAT") {
		t.Fatalf("refusal %q, want ADO_PAT, the default, named", e.Message)
	}
	_, err := c.srv.Core.SetAzureBoards(human, "acme", core.SetAzureBoardsInput{On: true, PATEnv: "OTHER_PAT", QueryURL: c.fake.ProjectURL() + "/_queries/query/" + queryID})
	if err != nil {
		t.Fatalf("with the variable named: %v", err)
	}
}
