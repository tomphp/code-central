package azdo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/tomoram/code-central/azdourl"
	"github.com/tomoram/code-central/watchers"
)

// Azure Boards: the Work Item Watcher's Work Item Tracking calls (Spec
// code-central-894). A query runs in the project; work items are read in
// the collection, which finds one moved to another of its projects.

// MaxBatch is how many work items one workitemsbatch call reads at most.
const MaxBatch = 200

// The work item fields the Work Item Watcher reads.
const (
	FieldTitle              = "System.Title"
	FieldType               = "System.WorkItemType"
	FieldState              = "System.State"
	FieldTeamProject        = "System.TeamProject"
	FieldRev                = "System.Rev"
	FieldDescription        = "System.Description"
	FieldReproSteps         = "Microsoft.VSTS.TCM.ReproSteps"
	FieldAcceptanceCriteria = "Microsoft.VSTS.Common.AcceptanceCriteria"
)

// WorkItemFields are the fields every work item read asks for. The team
// project is how the work item's web link is made, as Azure DevOps gives
// no links with a list of fields.
var WorkItemFields = []string{FieldTitle, FieldType, FieldState, FieldDescription, FieldReproSteps, FieldAcceptanceCriteria, FieldRev, FieldTeamProject}

// WorkItem is one work item as WorkItemFields read it: its fields hold
// HTML where the field is rich text, and are empty when the work item
// has no value.
type WorkItem struct {
	ID     int `json:"id"`
	Rev    int `json:"rev"`
	Fields struct {
		Title              string `json:"System.Title"`
		Type               string `json:"System.WorkItemType"`
		State              string `json:"System.State"`
		TeamProject        string `json:"System.TeamProject"`
		Description        string `json:"System.Description"`
		ReproSteps         string `json:"Microsoft.VSTS.TCM.ReproSteps"`
		AcceptanceCriteria string `json:"Microsoft.VSTS.Common.AcceptanceCriteria"`
	} `json:"fields"`
}

// QueryResult is a query's answer: its type, flat for a list of work
// items, and the work items it matched.
type QueryResult struct {
	QueryType string `json:"queryType"`
	WorkItems []struct {
		ID int `json:"id"`
	} `json:"workItems"`
}

// IDs are the IDs of the work items r matched, in its order.
func (r QueryResult) IDs() []int {
	ids := make([]int, len(r.WorkItems))
	for i, w := range r.WorkItems {
		ids[i] = w.ID
	}
	return ids
}

// RefusedError is Azure DevOps refusing a request it understood but won't
// act on (400 or 409), with its own message, such as a WIQL error's. It
// is ErrRefused.
type RefusedError struct {
	What   string
	Status int
	// Message is Azure DevOps' message, or the answer's first line when
	// it gave none.
	Message string
}

func (e *RefusedError) Error() string {
	return fmt.Sprintf("%v: %s answered %d: %s", ErrRefused, e.What, e.Status, e.Message)
}

// Unwrap makes e ErrRefused.
func (e *RefusedError) Unwrap() error { return ErrRefused }

// SplitQueryURL splits a saved query's URL, as azdourl.ParseQueryURL
// stores it, into the URL of the project it is in and the query's ID.
func SplitQueryURL(queryURL string) (projectURL, id string, err error) {
	stored, err := azdourl.ParseQueryURL(queryURL)
	if err != nil {
		return "", "", err
	}
	projectURL, id, _ = strings.Cut(stored, "/_queries/query/")
	return projectURL, id, nil
}

// collectionURL is the URL of the collection, or organization, the
// project is in.
func (c *Client) collectionURL() string {
	return c.projectURL[:strings.LastIndex(c.projectURL, "/")]
}

// WorkItemURL is the link to work item id, in project, in a browser.
func (c *Client) WorkItemURL(project string, id int) string {
	return fmt.Sprintf("%s/%s/_workitems/edit/%d", c.collectionURL(), url.PathEscape(project), id)
}

// SavedQuery runs the project's saved query id.
func (c *Client) SavedQuery(ctx context.Context, id string) (QueryResult, error) {
	var r QueryResult
	err := c.do(ctx, http.MethodGet, c.baseURL+"/_apis/wit/wiql/"+url.PathEscape(id), nil, nil, &r)
	return r, err
}

// WIQL runs query, a whole WIQL query, in the project, so @project names
// it.
func (c *Client) WIQL(ctx context.Context, query string) (QueryResult, error) {
	var r QueryResult
	err := c.do(ctx, http.MethodPost, c.baseURL+"/_apis/wit/wiql", nil, map[string]string{"query": query}, &r)
	return r, err
}

// WorkItems reads the work items ids, MaxBatch at a time, leaving out
// any Azure DevOps can't give, such as one deleted since it was queried.
func (c *Client) WorkItems(ctx context.Context, ids []int) ([]WorkItem, error) {
	var out []WorkItem
	for start := 0; start < len(ids); start += MaxBatch {
		batch := ids[start:min(start+MaxBatch, len(ids))]
		var list struct {
			Value []*WorkItem `json:"value"`
		}
		err := c.do(ctx, http.MethodPost, c.collectionURL()+"/_apis/wit/workitemsbatch", nil,
			map[string]any{"ids": batch, "fields": WorkItemFields, "errorPolicy": "omit"}, &list)
		if err != nil {
			return nil, err
		}
		for _, w := range list.Value {
			if w != nil {
				out = append(out, *w)
			}
		}
	}
	return out, nil
}

// WorkItem reads work item id. One deleted, or that the PAT can't read,
// is ErrNotFound, as Azure DevOps answers both alike.
func (c *Client) WorkItem(ctx context.Context, id int) (WorkItem, error) {
	var w WorkItem
	err := c.do(ctx, http.MethodGet, c.collectionURL()+"/_apis/wit/workitems/"+strconv.Itoa(id),
		url.Values{"fields": {strings.Join(WorkItemFields, ",")}}, nil, &w)
	return w, err
}

// messageOf is Azure DevOps' message in an error answer's JSON, or the
// answer's first line when it has none.
func messageOf(data []byte) string {
	var e struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &e) == nil && e.Message != "" {
		return e.Message
	}
	return watchers.FirstLine(string(data))
}
