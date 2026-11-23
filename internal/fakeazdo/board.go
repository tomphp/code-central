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
	// Tags is System.Tags: the tags joined by "; ".
	Tags string
}

// Comment is a comment in a work item's Discussion, as the fake stores it:
// its text, and the format it was posted in.
type Comment struct {
	Text   string
	Format string
}

// CommentsAPIVersion is the only API version that has work item comments.
const CommentsAPIVersion = "7.1-preview.4"

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
	// comments are each work item's Discussion, oldest first.
	comments map[int][]Comment
	// refused are the States a work item can't be moved to, each with the
	// message it is refused with.
	refused map[string]string
}

func newBoard() board {
	return board{items: map[int]WorkItem{}, saved: map[string][]int{}, filters: map[string][]int{}, comments: map[int][]Comment{}, refused: map[string]string{}}
}

// RefuseState makes a move of any work item to state refused with a 400
// and message, as a State unknown to its type, or a workflow rule, does.
func (s *Server) RefuseState(state, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.board.refused[state] = message
}

// PutComment adds a comment, as a human or an earlier run would have,
// to work item id's Discussion.
func (s *Server) PutComment(id int, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.board.comments[id] = append(s.board.comments[id], Comment{Text: text, Format: "markdown"})
}

// Comments is work item id's Discussion, oldest first.
func (s *Server) Comments(id int) []Comment {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.board.comments[id])
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
	case req.Method == http.MethodPatch && strings.HasPrefix(req.Path, workItemsPath):
		return b.patch(serverURL, req)
	}
	if id, ok := commentsOf(req.Path); ok {
		return b.discuss(id, req)
	}
	return nil
}

// patch is the board's answer to a JSON Patch of a work item: a test on
// /rev that holds, and an add of its tags or State, which it makes unless the
// request only validates. Anything else is refused, as are a stale /rev
// and a patch that isn't sent as a JSON Patch.
func (b board) patch(serverURL string, req Request) *Answer {
	id, err := strconv.Atoi(strings.TrimPrefix(req.Path, workItemsPath))
	w, ok := b.items[id]
	if err != nil || !ok {
		return refusal(http.StatusNotFound, fmt.Sprintf("TF401232: Work item %d does not exist, or you do not have permissions to update it.", id))
	}
	if req.ContentType != "application/json-patch+json" {
		return refusal(http.StatusUnsupportedMediaType, "The content type of a work item update must be application/json-patch+json.")
	}
	var ops []struct {
		Op    string          `json:"op"`
		Path  string          `json:"path"`
		Value json.RawMessage `json:"value"`
	}
	if json.Unmarshal([]byte(req.Body), &ops) != nil {
		return refusal(http.StatusBadRequest, "The JSON Patch can't be read.")
	}
	for _, op := range ops {
		switch {
		case op.Op == "test" && op.Path == "/rev":
			var rev int
			if json.Unmarshal(op.Value, &rev) != nil || rev != w.Rev {
				return refusal(http.StatusPreconditionFailed, fmt.Sprintf("The work item %d has been updated: its revision is %d.", id, w.Rev))
			}
		case op.Op == "add" && op.Path == "/fields/System.Tags":
			if json.Unmarshal(op.Value, &w.Tags) != nil {
				return refusal(http.StatusBadRequest, "System.Tags must be a string.")
			}
		case op.Op == "add" && op.Path == "/fields/System.State":
			if json.Unmarshal(op.Value, &w.State) != nil {
				return refusal(http.StatusBadRequest, "System.State must be a string.")
			}
			if message, ok := b.refused[w.State]; ok {
				return refusal(http.StatusBadRequest, message)
			}
		default:
			return refusal(http.StatusBadRequest, fmt.Sprintf("The fake doesn't take the operation %s %s.", op.Op, op.Path))
		}
	}
	if !slices.Contains(req.Query["validateOnly"], "true") {
		w.Rev++
		b.items[id] = w
	}
	a := JSON("", "", 0, w.json(serverURL))
	return &a
}

// commentsOf is the ID of the work item whose Discussion path is, as
// {project}/_apis/wit/workItems/{id}/comments in the collection.
func commentsOf(path string) (id int, ok bool) {
	rest, ok := strings.CutPrefix(path, CollectionPath+"/")
	if !ok {
		return 0, false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 6 || parts[1] != "_apis" || parts[2] != "wit" || parts[3] != "workItems" || parts[5] != "comments" {
		return 0, false
	}
	id, err := strconv.Atoi(parts[4])
	return id, err == nil
}

// discuss is the board's answer to listing or adding to work item id's
// Discussion, which only API 7.1-preview.4 has, and adding takes only
// Markdown or HTML.
func (b board) discuss(id int, req Request) *Answer {
	if !slices.Contains(req.Query["api-version"], CommentsAPIVersion) {
		return refusal(http.StatusBadRequest, "The comments API is only in version "+CommentsAPIVersion+".")
	}
	if _, ok := b.items[id]; !ok {
		return refusal(http.StatusNotFound, fmt.Sprintf("TF401232: Work item %d does not exist, or you do not have permissions to read it.", id))
	}
	if req.Method == http.MethodPost {
		var in struct {
			Text string `json:"text"`
		}
		format := strings.Join(req.Query["format"], "")
		if json.Unmarshal([]byte(req.Body), &in) != nil || (format != "markdown" && format != "html") {
			return refusal(http.StatusBadRequest, "A comment needs its text and a format, markdown or html.")
		}
		b.comments[id] = append(b.comments[id], Comment{Text: in.Text, Format: format})
		a := JSON("", "", 0, map[string]any{"id": len(b.comments[id]), "workItemId": id, "text": in.Text})
		return &a
	}
	list := make([]map[string]any, len(b.comments[id]))
	for i, c := range b.comments[id] {
		list[i] = map[string]any{"id": i + 1, "workItemId": id, "text": c.Text}
	}
	a := JSON("", "", 0, map[string]any{"totalCount": len(list), "count": len(list), "comments": list})
	return &a
}

// json is w as Azure DevOps gives it, leaving out the fields it has no
// value for, as Azure DevOps does.
func (w WorkItem) json(serverURL string) map[string]any {
	fields := map[string]any{"System.TeamProject": w.project(), "System.Rev": w.Rev}
	for name, v := range map[string]string{
		"System.Tags":         w.Tags,
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
