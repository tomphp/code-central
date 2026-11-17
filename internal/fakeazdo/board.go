// The fake's board: the work items, saved queries and WIQL filters it
// answers Work Item Tracking calls from, as Azure Boards would, unless a
// scripted Answer matches first.

package fakeazdo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
)

// WorkItem is a work item on the fake's board. Its rich text fields hold
// HTML, as Azure DevOps keeps them.
type WorkItem struct {
	ID  int
	Rev int
	// Project is the team project it is in; Project when empty.
	Project            string
	Type               string
	State              string
	Title              string
	Description        string
	ReproSteps         string
	AcceptanceCriteria string
}

// WebURL is w's link in a browser, on the fake at serverURL.
func (w WorkItem) WebURL(serverURL string) string {
	return fmt.Sprintf("%s%s/%s/_workitems/edit/%d", serverURL, CollectionPath, w.project(), w.ID)
}

func (w WorkItem) project() string {
	if w.Project == "" {
		return Project
	}
	return w.Project
}

// board is what the fake's Work Item Tracking answers come from.
type board struct {
	items map[int]WorkItem
	// saved are the saved queries, by ID, and filters the WIQL filters, by
	// WHERE clause, each with the IDs it matches.
	saved   map[string][]int
	filters map[string][]int
}

func newBoard() board {
	return board{items: map[int]WorkItem{}, saved: map[string][]int{}, filters: map[string][]int{}}
}

// PutWorkItems adds work items to the board, or replaces those with the
// same IDs.
func (s *Server) PutWorkItems(items ...WorkItem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, w := range items {
		s.board.items[w.ID] = w
	}
}

// WorkItem is the board's work item id.
func (s *Server) WorkItem(id int) WorkItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.board.items[id]
}

// DeleteWorkItem takes work item id off the board, as deleting it does.
func (s *Server) DeleteWorkItem(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.board.items, id)
}

// SavedQuery makes the project's saved query id a flat one that matches
// ids, in that order, whether or not they are on the board.
func (s *Server) SavedQuery(id string, ids ...int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.board.saved[id] = ids
}

// DeleteSavedQuery takes the project's saved query id away.
func (s *Server) DeleteSavedQuery(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.board.saved, id)
}

// Filter makes a WIQL query of the project whose WHERE clause is where
// match ids, in that order. A WHERE clause the board has no filter for is
// a WIQL error.
func (s *Server) Filter(where string, ids ...int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.board.filters[where] = ids
}

// The board's paths, below the server's root.
const (
	wiqlPath      = ProjectPath + "/_apis/wit/wiql"
	batchPath     = CollectionPath + "/_apis/wit/workitemsbatch"
	workItemsPath = CollectionPath + "/_apis/wit/workitems/"
)

// wiqlPrefix is what a WIQL query is up to its WHERE clause.
const wiqlPrefix = "SELECT [System.Id] FROM workitems WHERE "

// answer is the board's answer to req on the fake at serverURL, or nil
// when req isn't a Work Item Tracking call.
func (b board) answer(serverURL string, req Request) *Answer {
	switch {
	case req.Method == http.MethodGet && strings.HasPrefix(req.Path, wiqlPath+"/"):
		ids, ok := b.saved[strings.TrimPrefix(req.Path, wiqlPath+"/")]
		if !ok {
			return refusal(http.StatusNotFound, "TF401243: The query does not exist, or you do not have permission to read it.")
		}
		return flat(ids)
	case req.Method == http.MethodPost && req.Path == wiqlPath:
		var q struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal([]byte(req.Body), &q)
		where, ok := strings.CutPrefix(q.Query, wiqlPrefix)
		ids, known := b.filters[where]
		if !ok || !known {
			return refusal(http.StatusBadRequest, "TF51005: The query references a field that does not exist.")
		}
		return flat(ids)
	case req.Method == http.MethodPost && req.Path == batchPath:
		var in struct {
			IDs         []int  `json:"ids"`
			ErrorPolicy string `json:"errorPolicy"`
		}
		_ = json.Unmarshal([]byte(req.Body), &in)
		if len(in.IDs) > 200 {
			return refusal(http.StatusBadRequest, "VS403474: You can read at most 200 work items at a time.")
		}
		value := make([]any, 0, len(in.IDs))
		for _, id := range in.IDs {
			w, ok := b.items[id]
			switch {
			case ok:
				value = append(value, w.json(serverURL))
			case in.ErrorPolicy == "omit":
				value = append(value, nil)
			default:
				return refusal(http.StatusNotFound, fmt.Sprintf("TF401232: Work item %d does not exist, or you do not have permissions to read it.", id))
			}
		}
		a := JSON("", "", 0, map[string]any{"count": len(value), "value": value})
		return &a
	case req.Method == http.MethodPatch && strings.HasPrefix(req.Path, workItemsPath) && slices.Contains(req.Query["validateOnly"], "true"):
		// A validated write is checked and answered, never saved.
		id, err := strconv.Atoi(strings.TrimPrefix(req.Path, workItemsPath))
		w, ok := b.items[id]
		if err != nil || !ok {
			return refusal(http.StatusNotFound, fmt.Sprintf("TF401232: Work item %d does not exist, or you do not have permissions to read it.", id))
		}
		a := JSON("", "", 0, w.json(serverURL))
		return &a
	case req.Method == http.MethodGet && strings.HasPrefix(req.Path, workItemsPath):
		id, err := strconv.Atoi(strings.TrimPrefix(req.Path, workItemsPath))
		w, ok := b.items[id]
		if err != nil || !ok {
			return refusal(http.StatusNotFound, fmt.Sprintf("TF401232: Work item %d does not exist, or you do not have permissions to read it.", id))
		}
		a := JSON("", "", 0, w.json(serverURL))
		return &a
	}
	return nil
}

// json is w as Azure DevOps gives it, leaving out the fields it has no
// value for, as Azure DevOps does.
func (w WorkItem) json(serverURL string) map[string]any {
	fields := map[string]any{"System.TeamProject": w.project(), "System.Rev": w.Rev}
	for name, v := range map[string]string{
		"System.WorkItemType": w.Type, "System.State": w.State, "System.Title": w.Title, "System.Description": w.Description,
		"Microsoft.VSTS.TCM.ReproSteps": w.ReproSteps, "Microsoft.VSTS.Common.AcceptanceCriteria": w.AcceptanceCriteria,
	} {
		if v != "" {
			fields[name] = v
		}
	}
	return map[string]any{"id": w.ID, "rev": w.Rev, "fields": fields,
		"url": fmt.Sprintf("%s%s/_apis/wit/workItems/%d", serverURL, CollectionPath, w.ID)}
}

// flat is a flat query's answer, matching ids.
func flat(ids []int) *Answer {
	refs := make([]map[string]int, len(ids))
	for i, id := range ids {
		refs[i] = map[string]int{"id": id}
	}
	a := JSON("", "", 0, map[string]any{"queryType": "flat", "workItems": refs})
	return &a
}

// refusal is an error answer with Azure DevOps' message.
func refusal(status int, message string) *Answer {
	a := JSON("", "", status, map[string]string{"message": message})
	return &a
}
