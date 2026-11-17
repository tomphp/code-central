package workitemwatcher_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/internal/fakeazdo"
	"github.com/tomoram/code-central/store"
)

// org is the fake's organization, its collection, as Link IDs carry it:
// lowercased.
const org = "defaultcollection"

// linkID is work item id's Link ID.
func linkID(id int) string { return fmt.Sprintf("%s#%d", org, id) }

// item is the approved Product Backlog Item id, with a description.
func item(id int) fakeazdo.WorkItem {
	return fakeazdo.WorkItem{ID: id, Rev: 1, Type: "Product Backlog Item", State: "Approved",
		Title: fmt.Sprintf("Item %d", id), Description: fmt.Sprintf("<div>Body %d</div>", id)}
}

// body is the Incoming Issue's body for item(id) in project.
func (w *watching) body(id int, project string) string {
	return fmt.Sprintf("**Product Backlog Item** · Approved · [View in Azure Boards](%s)\n\nBody %d\n", w.url(id, project), id)
}

// url is the web link of work item id in project.
func (w *watching) url(id int, project string) string {
	return fakeazdo.WorkItem{ID: id, Project: project}.WebURL(w.fake.URL)
}

// watch is acme's get_work_item_watch.
func (w *watching) watch() core.WorkItemWatch {
	w.t.Helper()
	watch, err := w.srv.Core.GetWorkItemWatch(context.Background(), "acme")
	if err != nil {
		w.t.Fatal(err)
	}
	return watch
}

// incoming is acme's Incoming Issues as get_work_item_watch lists them,
// each as its Link's ID and URL, title and description.
func (w *watching) incoming() []string {
	w.t.Helper()
	var out []string
	for _, i := range w.watch().Issues {
		out = append(out, fmt.Sprintf("%s %s %s: %s", i.Link.ID, i.Link.URL, i.Title, i.Description))
	}
	return out
}

// line is item(id) as incoming lists it, in project.
func (w *watching) line(id int, project string) string {
	return fmt.Sprintf("%s %s Item %d: %s", linkID(id), w.url(id, project), id, w.body(id, project))
}

// linked is every Spec and Ticket in acme linked by azure-boards.
func (w *watching) linked() []core.LinkedRow {
	w.t.Helper()
	rows, err := w.srv.Core.FindLinked(context.Background(), "acme", core.FindLinkedInput{Integration: core.IntegrationAzureBoards})
	if err != nil {
		w.t.Fatal(err)
	}
	return rows
}

// wantIncoming checks acme's Incoming Issues are item's ids, in the
// fake's project, and that nothing else is linked: each arrived once.
func wantIncoming(t *testing.T, w *watching, ids ...int) {
	t.Helper()
	var want []string
	for _, id := range ids {
		want = append(want, w.line(id, fakeazdo.Project))
	}
	if got := w.incoming(); !slices.Equal(got, want) {
		t.Fatalf("Incoming Issues %q, want %q", got, want)
	}
	if got := len(w.linked()); got != len(ids) {
		t.Fatalf("%d linked items, want %d: one per work item", got, len(ids))
	}
}

// arrived starts the Work Item Watcher with item's ids matching the
// query, and waits for them to arrive.
func (w *watching) arrived(ids ...int) {
	w.t.Helper()
	for _, id := range ids {
		w.fake.PutWorkItems(item(id))
	}
	w.fake.SavedQuery(queryID, ids...)
	w.start()
	w.status(core.WatcherWatching)
	wantIncoming(w.t, w, ids...)
}

// ticketOf is the Ticket linked to work item id.
func (w *watching) ticketOf(id int) core.TicketView {
	w.t.Helper()
	for _, row := range w.linked() {
		if row.Link.ID == linkID(id) {
			got, err := w.srv.Core.GetTicket(context.Background(), "acme", core.GetTicketInput{Ticket: core.Ref(row.ID), Parts: []string{core.PartEdits}})
			if err != nil {
				w.t.Fatal(err)
			}
			return got
		}
	}
	w.t.Fatalf("no Ticket linked to %s", linkID(id))
	return core.TicketView{}
}

// wantExternal checks the Incoming Issue linked to work item id is still
// Draft, linked and not detached, with external state want.
func wantExternal(t *testing.T, w *watching, id int, want store.ExternalState) {
	t.Helper()
	got := w.ticketOf(id)
	if got.State != store.Draft || got.Link == nil || got.Link.Detached || got.Link.ExternalState != want {
		t.Fatalf("work item %d's Ticket is %s with Link %+v, want Draft, linked, %s", id, got.State, got.Link, want)
	}
}

// read is the IDs of the work items the fake was asked for since
// request from, together or alone, in order.
func (w *watching) read(from int) []int {
	w.t.Helper()
	var ids []int
	for _, r := range w.fake.Requests()[from:] {
		switch {
		case strings.HasSuffix(r.Path, "/_apis/wit/workitemsbatch"):
			var body struct {
				IDs []int `json:"ids"`
			}
			if err := json.Unmarshal([]byte(r.Body), &body); err != nil {
				w.t.Fatal(err)
			}
			ids = append(ids, body.IDs...)
		case strings.Contains(r.Path, "/_apis/wit/workitems/"):
			var id int
			fmt.Sscanf(r.Path[strings.LastIndex(r.Path, "/")+1:], "%d", &id)
			ids = append(ids, id)
		}
	}
	return ids
}

// Every work item the query matches arrives as a Draft Ticket on the
// Azure Boards Spec, linked as org#id to its web link; one that matches
// later arrives on the next poll.
func TestMatchingWorkItemsArriveAsDraftIncomingIssuesOnTheAzureBoardsSpec(t *testing.T) {
	w := newWatching(t)
	w.arrived(1, 2)
	watch := w.watch()
	for _, i := range watch.Issues {
		got, err := w.srv.Core.GetTicket(context.Background(), "acme", core.GetTicketInput{Ticket: core.Ref(i.Ticket)})
		if err != nil {
			t.Fatal(err)
		}
		if got.State != store.Draft || got.Spec.ID != watch.Spec || got.Link.Integration != core.IntegrationAzureBoards || got.Link.ExternalState != store.ExternalOpen {
			t.Fatalf("%s is %s on %s, linked %+v, want Draft on the Azure Boards Spec %s, open on azure-boards", i.Ticket, got.State, got.Spec.ID, got.Link, watch.Spec)
		}
	}

	w.fake.PutWorkItems(item(3))
	w.fake.SavedQuery(queryID, 1, 2, 3)
	w.polled()
	wantIncoming(t, w, 1, 2, 3)
}

// The body is the type and State line linking to the work item, then
// the Description, Repro Steps and Acceptance Criteria as Markdown, each
// section only when it has something in it. Images and attachments stay
// links into Azure DevOps.
func TestTheBodyIsTheTypeAndStateLineThenEachFilledInFieldAsMarkdown(t *testing.T) {
	w := newWatching(t)
	attachment := "https://dev.azure.com/acme/_apis/wit/attachments/9f1c?fileName=screen.png"
	bug := fakeazdo.WorkItem{ID: 7, Type: "Bug", State: "Committed", Title: "Login fails",
		Description:        `<div>The <b>login</b> form fails.</div><div><img src="` + attachment + `" alt="screen"></div><div>See <a href="https://dev.azure.com/acme/_apis/wit/attachments/1a2b?fileName=log.txt">log.txt</a>.</div>`,
		ReproSteps:         "<ol><li>Open the page</li><li>Log in</li></ol>",
		AcceptanceCriteria: "<div><br></div>"}
	w.fake.PutWorkItems(bug)
	w.fake.SavedQuery(queryID, 7)
	w.start()
	w.status(core.WatcherWatching)

	want := "**Bug** · Committed · [View in Azure Boards](" + w.url(7, fakeazdo.Project) + ")\n\n" +
		"The **login** form fails.\n\n[screen](" + attachment + ")\n\nSee [log.txt](https://dev.azure.com/acme/_apis/wit/attachments/1a2b?fileName=log.txt).\n\n" +
		"## Repro Steps\n\n1. Open the page\n2. Log in\n"
	if got := w.ticketOf(7); got.Title != "Login fails" || got.Description != want {
		t.Fatalf("Incoming Issue %q:\n%s\nwant:\n%s", got.Title, got.Description, want)
	}

	// With every field empty, the body is the line alone; Acceptance
	// Criteria with text has its section.
	bug.Description, bug.ReproSteps, bug.AcceptanceCriteria = "", "", "<p>It logs in.</p>"
	w.fake.PutWorkItems(bug)
	w.polled()
	want = "**Bug** · Committed · [View in Azure Boards](" + w.url(7, fakeazdo.Project) + ")\n\n## Acceptance Criteria\n\nIt logs in.\n"
	if got := w.ticketOf(7).Description; got != want {
		t.Fatalf("body:\n%s\nwant:\n%s", got, want)
	}
}

// An edit to a work item, its State included, updates its Incoming
// Issue, through its edit history; an unchanged one isn't edited.
func TestEditsToAWorkItemFlowIntoItsIncomingIssue(t *testing.T) {
	w := newWatching(t)
	w.arrived(1, 2)
	edited := item(1)
	edited.Rev, edited.Title, edited.State, edited.Description = 2, "Item 1, renamed", "Committed", "<div>Body 1, edited</div>"
	w.fake.PutWorkItems(edited)
	w.polled()

	got := w.ticketOf(1)
	want := "**Product Backlog Item** · Committed · [View in Azure Boards](" + w.url(1, fakeazdo.Project) + ")\n\nBody 1, edited\n"
	if got.Title != "Item 1, renamed" || got.Description != want {
		t.Fatalf("Incoming Issue %q: %q, want %q: %q", got.Title, got.Description, "Item 1, renamed", want)
	}
	if len(got.Edits) != 1 || got.Edits[0].Actor != "integration" || got.Edits[0].Previous["title"] != "Item 1" {
		t.Fatalf("edits %+v, want one by the integration", got.Edits)
	}
	if got := w.ticketOf(2); len(got.Edits) != 0 {
		t.Fatalf("unchanged work item's edits %+v, want none", got.Edits)
	}
	w.polled()
	if got := w.ticketOf(1); len(got.Edits) != 1 {
		t.Fatalf("edits %+v after a poll with no change, want still one", got.Edits)
	}
}

// A work item that leaves the query's results is out of scope, and
// restored when it comes back; one deleted is deleted. Only those leaving
// are read alone, and only new matches and Incoming Issues are read.
func TestAWorkItemLeavingTheResultsIsOutOfScopeAndRestoredWhenItReturns(t *testing.T) {
	w := newWatching(t)
	w.arrived(1, 2, 3)
	w.fake.DeleteWorkItem(3)
	w.fake.SavedQuery(queryID, 1)
	from := len(w.fake.Requests())
	w.polled()
	wantExternal(t, w, 1, store.ExternalOpen)
	wantExternal(t, w, 2, store.ExternalOutOfScope)
	wantExternal(t, w, 3, store.ExternalDeleted)
	if got := w.read(from); !slices.Equal(got, []int{1, 2, 3}) {
		t.Fatalf("read %v, want 1 with the matches, then 2 and 3 alone", got)
	}

	w.fake.SavedQuery(queryID, 1, 2)
	w.polled()
	wantExternal(t, w, 2, store.ExternalOpen)
	wantExternal(t, w, 3, store.ExternalDeleted)
	if got := len(w.linked()); got != 3 {
		t.Fatalf("%d linked items, want 3: nothing arrives twice", got)
	}
}

// Changing the query, or the mode, behaves as the work items changing:
// those no longer matching are out of scope, and new matches arrive.
func TestChangingTheQueryOrTheModeMovesWorkItemsInAndOutOfScope(t *testing.T) {
	w := newWatching(t)
	w.arrived(1, 2)
	const otherQuery = "11111111-2222-3333-4444-555555555555"
	w.fake.PutWorkItems(item(3))
	w.fake.SavedQuery(otherQuery, 2, 3)
	w.setBoards(core.SetAzureBoardsInput{On: true, QueryURL: w.queryURL(otherQuery)})
	w.polled()
	wantExternal(t, w, 1, store.ExternalOutOfScope)
	wantExternal(t, w, 2, store.ExternalOpen)
	wantExternal(t, w, 3, store.ExternalOpen)

	w.fake.Filter("[System.AssignedTo] = @Me", 1)
	w.setBoards(core.SetAzureBoardsInput{On: true, Mode: "wiql", QueryURL: w.queryURL(otherQuery), ProjectURL: w.fake.ProjectURL(), Where: "[System.AssignedTo] = @Me"})
	w.polled()
	wantExternal(t, w, 1, store.ExternalOpen)
	wantExternal(t, w, 2, store.ExternalOutOfScope)
	wantExternal(t, w, 3, store.ExternalOutOfScope)
	if got := len(w.linked()); got != 3 {
		t.Fatalf("%d linked items, want 3", got)
	}
	posted := w.fake.Requests()[len(w.fake.Requests())-1]
	for _, r := range w.fake.Requests() {
		if r.Method == "POST" && strings.HasSuffix(r.Path, "/_apis/wit/wiql") {
			posted = r
		}
	}
	if !strings.Contains(posted.Body, `SELECT [System.Id] FROM workitems WHERE [System.AssignedTo] = @Me`) {
		t.Fatalf("WIQL posted %s, want the WHERE clause wrapped", posted.Body)
	}
}

// A work item moved to another project of the organization keeps its
// Link ID, so it doesn't arrive again, and its Link and body follow it.
func TestAWorkItemMovedToAnotherProjectIsNotADuplicate(t *testing.T) {
	w := newWatching(t)
	w.arrived(1)
	moved := item(1)
	moved.Project = "api"
	w.fake.PutWorkItems(moved)
	w.polled()
	if got, want := w.incoming(), []string{w.line(1, "api")}; !slices.Equal(got, want) {
		t.Fatalf("Incoming Issues %q, want %q", got, want)
	}
	if got := len(w.linked()); got != 1 {
		t.Fatalf("%d linked items after the move, want 1", got)
	}
}

// A work item that became a Requirement is no longer the Work Item
// Watcher's: it isn't read, and leaving the results changes nothing.
func TestAWorkItemThatBecameARequirementIsUnaffectedByTheQuery(t *testing.T) {
	w := newWatching(t)
	w.arrived(1, 2)
	if _, err := w.srv.Core.ToRequirement(human, "acme", core.ToRequirementInput{Ticket: core.Ref(w.ticketOf(1).ID)}); err != nil {
		t.Fatal(err)
	}
	from := len(w.fake.Requests())
	w.polled()
	if got := w.read(from); !slices.Equal(got, []int{2}) {
		t.Fatalf("read %v, want only the Incoming Issue's work item", got)
	}

	w.fake.SavedQuery(queryID, 2)
	from = len(w.fake.Requests())
	w.polled()
	if got := w.read(from); !slices.Equal(got, []int{2}) {
		t.Fatalf("read %v, want only the Incoming Issue's work item", got)
	}
	for _, row := range w.linked() {
		if row.Link.ID == linkID(1) && row.Link.ExternalState != store.ExternalOpen {
			t.Fatalf("the Requirement's work item is %s, want open still", row.Link.ExternalState)
		}
	}
	if got := len(w.linked()); got != 3 {
		t.Fatalf("%d linked items, want the two Tickets and the Requirement: nothing new", got)
	}
}
