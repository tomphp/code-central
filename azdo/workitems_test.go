package azdo_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/azdo"
	"github.com/tomoram/code-central/internal/fakeazdo"
)

const queryID = "0f1e2d3c-4b5a-6978-8a9b-0c1d2e3f4a5b"

func TestASavedQueryRunsByItsIDInTheProject(t *testing.T) {
	fake := fakeazdo.New(t)
	fake.SavedQuery(queryID, 3, 1, 2)
	r, err := client(t, fake).SavedQuery(context.Background(), queryID)
	if err != nil || r.QueryType != "flat" || !slices.Equal(r.IDs(), []int{3, 1, 2}) {
		t.Fatalf("SavedQuery = %+v, %v", r, err)
	}
	req := fake.Requests()[0]
	if req.Method != "GET" || req.Path != fakeazdo.ProjectPath+"/_apis/wit/wiql/"+queryID || req.Query["api-version"][0] != "7.1" {
		t.Fatalf("request = %+v, want GET wiql/<id> in the project at API 7.1", req)
	}
}

func TestAWIQLQueryIsPostedToTheProject(t *testing.T) {
	fake := fakeazdo.New(t)
	fake.Filter("[System.State] = 'Approved'", 7)
	r, err := client(t, fake).WIQL(context.Background(), "SELECT [System.Id] FROM workitems WHERE [System.State] = 'Approved'")
	if err != nil || !slices.Equal(r.IDs(), []int{7}) {
		t.Fatalf("WIQL = %+v, %v", r, err)
	}
	req := fake.Requests()[0]
	if req.Method != "POST" || req.Path != fakeazdo.ProjectPath+"/_apis/wit/wiql" || req.Query["api-version"][0] != "7.1" {
		t.Fatalf("request = %+v, want POST wiql in the project at API 7.1", req)
	}
}

func TestAWIQLErrorCarriesAzureDevOpsOwnMessage(t *testing.T) {
	fake := fakeazdo.New(t)
	_, err := client(t, fake).WIQL(context.Background(), "SELECT [System.Id] FROM workitems WHERE [Nope] = 1")
	var refused *azdo.RefusedError
	if !errors.As(err, &refused) || !errors.Is(err, azdo.ErrRefused) || refused.Message != "TF51005: The query references a field that does not exist." {
		t.Fatalf("err = %v, want refused with Azure DevOps' message alone", err)
	}
}

func TestWorkItemsAreReadInTheCollection200AtATimeLeavingOutMissingOnes(t *testing.T) {
	fake := fakeazdo.New(t)
	var ids []int
	for id := 1; id <= 450; id++ {
		ids = append(ids, id)
		if id != 300 {
			fake.PutWorkItems(fakeazdo.WorkItem{ID: id, Title: "Item", Type: "Bug", State: "New"})
		}
	}
	items, err := client(t, fake).WorkItems(context.Background(), ids)
	if err != nil || len(items) != 449 {
		t.Fatalf("WorkItems = %d items, %v; want 449, without the missing one", len(items), err)
	}
	if w := items[0]; w.ID != 1 || w.Fields.Title != "Item" || w.Fields.Type != "Bug" || w.Fields.State != "New" || w.Fields.TeamProject != "web" {
		t.Fatalf("first = %+v", w)
	}
	var sizes []int
	for _, req := range fake.Requests() {
		if req.Method != "POST" || req.Path != fakeazdo.CollectionPath+"/_apis/wit/workitemsbatch" || req.Query["api-version"][0] != "7.1" {
			t.Fatalf("request = %+v, want POST workitemsbatch in the collection at API 7.1", req)
		}
		var body struct {
			IDs    []int    `json:"ids"`
			Fields []string `json:"fields"`
		}
		if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(body.Fields, "Microsoft.VSTS.Common.AcceptanceCriteria") || !slices.Contains(body.Fields, "System.Rev") {
			t.Fatalf("fields = %v", body.Fields)
		}
		sizes = append(sizes, len(body.IDs))
	}
	if !slices.Equal(sizes, []int{200, 200, 50}) {
		t.Fatalf("batches of %v, want 200, 200 and 50", sizes)
	}
}

func TestAWorkItemIsReadAloneAndOneDeletedIsNotFound(t *testing.T) {
	fake := fakeazdo.New(t)
	fake.PutWorkItems(fakeazdo.WorkItem{ID: 5, Title: "Moved", Project: "api"})
	c := client(t, fake)
	w, err := c.WorkItem(context.Background(), 5)
	if err != nil || w.Fields.Title != "Moved" || w.Fields.TeamProject != "api" {
		t.Fatalf("WorkItem = %+v, %v", w, err)
	}
	if req := fake.Requests()[0]; req.Path != fakeazdo.CollectionPath+"/_apis/wit/workitems/5" || !strings.Contains(req.Query["fields"][0], "System.Title") {
		t.Fatalf("request = %+v", req)
	}
	if _, err := c.WorkItem(context.Background(), 6); !errors.Is(err, azdo.ErrNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestAWorkItemsWebLinkIsInItsOwnProject(t *testing.T) {
	fake := fakeazdo.New(t)
	if got, want := client(t, fake).WorkItemURL("api team", 5), fake.URL+fakeazdo.CollectionPath+"/api%20team/_workitems/edit/5"; got != want {
		t.Fatalf("WorkItemURL = %q, want %q", got, want)
	}
}

func TestASavedQueryURLSplitsIntoItsProjectAndID(t *testing.T) {
	project, id, err := azdo.SplitQueryURL("https://dev.azure.com/acme/web/_queries/query/" + queryID + "/")
	if err != nil || project != "https://dev.azure.com/acme/web" || id != queryID {
		t.Fatalf("SplitQueryURL = %q, %q, %v", project, id, err)
	}
}
