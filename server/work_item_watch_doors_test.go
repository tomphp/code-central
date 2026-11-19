package server_test

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/httpapi"
)

// The Work Item Watcher's contract and triage on the Azure Boards Spec
// (Spec code-central-894): an azure-boards linked Ticket on the Azure
// Boards Spec is an Incoming Issue, triaged as a GitHub issue is.
// get_work_item_watch, the read the Work Item Watcher polls, and
// report_work_item_watcher_status are the integration Actor's, over HTTP
// only.
func init() {
	cases["get_work_item_watch"] = opCase{
		setup: boardsOn(false),
		ok:    call{http: getWorkItemWatch("acme")},
		check: func(t *testing.T, out map[string]any) {
			field("project", "acme")(t, out)
			field("query_url", webQuery)(t, out)
		},
		bad:  call{http: getWorkItemWatch("nope")},
		code: core.UnknownProject,
	}
	cases["report_work_item_watcher_status"] = opCase{
		setup: boardsOn(false),
		ok:    call{http: reportWorkItemWatcherStatus(`{"state":"watching"}`)},
		check: func(t *testing.T, out map[string]any) {
			acknowledgement("acme", "azure_boards")(t, out)
		},
		full: func(t *testing.T, out map[string]any) {
			workItemWatcher(t, out, "watching", "", "")
		},
		bad:  call{http: reportWorkItemWatcherStatus(`{"state":"asleep"}`)},
		code: core.Invalid,
	}
}

func getWorkItemWatch(key string) httpCall {
	return httpCall{"GET", "/api/p/" + key + "/integration/work-item-watch", ""}
}

func reportWorkItemWatcherStatus(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/integration/report_work_item_watcher_status", body}
}

// boardsOn turns Azure Boards on for acme with contoso/web's saved query,
// and New issues need me as needMe. On its own it creates the Azure
// Boards Spec as acme-4.
func boardsOn(needMe bool) func(*testing.T, *codecentraltest.Server) {
	return boardsHidden(needMe, false)
}

// boardsHidden turns Azure Boards on for acme as boardsOn does, with
// Hide Code Central as hide.
func boardsHidden(needMe, hide bool) func(*testing.T, *codecentraltest.Server) {
	return func(t *testing.T, srv *codecentraltest.Server) {
		t.Helper()
		httpOK(t, srv, setAzureBoards(`{"on":true,"query_url":"`+webQuery+`","new_issues_need_me":`+strconv.FormatBool(needMe)+
			`,"hide_code_central":`+strconv.FormatBool(hide)+`}`))
	}
}

// workItem creates a Draft Ticket on spec linked to contoso's work item
// id, as the Work Item Watcher does, and returns its ID.
func workItem(t *testing.T, srv *codecentraltest.Server, spec string, id int) string {
	t.Helper()
	body := fmt.Sprintf(`{"spec":%q,"title":"Work item %d","description":"Body %d","full":true,"link":{"integration":"azure-boards","id":"contoso#%d","url":"https://dev.azure.com/contoso/web/_workitems/edit/%d"}}`,
		spec, id, id, id, id)
	return httpOK(t, srv, httpCall{"POST", "/api/p/acme/integration/create_linked_ticket", body})["id"].(string)
}

// workItemWatcher checks the Work Item Watcher's status in a Project's
// azure_boards object.
func workItemWatcher(t *testing.T, project map[string]any, state, message, fix string) {
	t.Helper()
	watcherIn(t, project, "azure_boards", state, message, fix)
}

// workItemWrites is an item whose work item has writes due, as
// get_work_item_watch lists it, with the keys it carries.
type workItemWrites struct {
	issueWrites
	Tag  string
	Keys []string
}

// workItemWritesDue is acme's items with work item writes due, by target.
func workItemWritesDue(t *testing.T, srv *codecentraltest.Server) map[string]workItemWrites {
	t.Helper()
	out := map[string]workItemWrites{}
	for _, raw := range httpOK(t, srv, getWorkItemWatch("acme"))["writes"].([]any) {
		m := raw.(map[string]any)
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		var w workItemWrites
		if err := json.Unmarshal(b, &w.issueWrites); err != nil {
			t.Fatal(err)
		}
		w.Tag = str(m["tag"])
		for k := range m {
			w.Keys = append(w.Keys, k)
		}
		slices.Sort(w.Keys)
		out[w.Target] = w
	}
	return out
}

// wantTagDue checks one item's work item writes due: its tag and its
// comment keys, and that it has no label or closing, which a work item
// never has.
func wantTagDue(t *testing.T, got map[string]workItemWrites, target, tag string, keys ...string) {
	t.Helper()
	w, ok := got[target]
	if !ok {
		t.Fatalf("%s has no work item writes due, want %s, comments %v", target, tag, keys)
	}
	if w.Tag != tag || !slices.Equal(w.keys(), keys) {
		t.Fatalf("%s's work item writes due = %+v, want %s, comments %v", target, w, tag, keys)
	}
	if slices.Contains(w.Keys, "close") || slices.Contains(w.Keys, "label") {
		t.Fatalf("%s's work item writes have %v, want no close or label", target, w.Keys)
	}
}

// watchTickets are the ticket IDs of a watch's Incoming Issues.
func watchTickets(watch map[string]any) []string {
	var out []string
	for _, i := range watch["issues"].([]any) {
		out = append(out, i.(map[string]any)["ticket"].(string))
	}
	return out
}

func TestGetWorkItemWatchHasTheAzureBoardsSettings(t *testing.T) {
	srv := newServer(t)
	got := httpOK(t, srv, getWorkItemWatch("acme"))
	want := map[string]any{"project": "acme", "on": false, "mode": "saved_query", "query_url": "", "project_url": "", "where": "", "pat_env": "ADO_PAT"}
	noWorkItems(t, got)
	if !mapsEqual(got, want) {
		t.Fatalf("get_work_item_watch while off = %v, want %v", got, want)
	}

	httpOK(t, srv, setAzureBoards(`{"on":true,"mode":"wiql","project_url":"https://dev.azure.com/contoso/web","where":"[System.State] = 'Approved'","pat_env":"BOARDS_PAT"}`))
	got = httpOK(t, srv, getWorkItemWatch("acme"))
	want = map[string]any{"project": "acme", "on": true, "mode": "wiql", "query_url": "", "project_url": "https://dev.azure.com/contoso/web",
		"where": "[System.State] = 'Approved'", "pat_env": "BOARDS_PAT", "spec": "acme-4"}
	noWorkItems(t, got)
	if !mapsEqual(got, want) {
		t.Fatalf("get_work_item_watch = %v, want %v", got, want)
	}
}

// noWorkItems checks get_work_item_watch's out lists no Incoming Issues,
// no Links and no writes due, and takes them out of it, leaving the
// settings to compare.
func noWorkItems(t *testing.T, out map[string]any) {
	t.Helper()
	for _, list := range []string{"issues", "linked", "writes"} {
		if items, ok := out[list].([]any); !ok || len(items) != 0 {
			t.Fatalf("get_work_item_watch %s = %v, want none", list, out[list])
		}
		delete(out, list)
	}
}

// get_work_item_watch lists the Incoming Issues on the Azure Boards Spec,
// each with its Link, title and description, and every azure-boards Link
// ID in the Project, detached and outbound ones too, once each.
func TestGetWorkItemWatchListsTheIncomingIssuesAndEveryAzureBoardsLink(t *testing.T) {
	srv := newServer(t, boardsOn(false))
	waiting := workItem(t, srv, "acme-4", 1) // acme-5
	toReq := workItem(t, srv, "acme-4", 2)   // acme-6, to acme-7
	httpOK(t, srv, toRequirement(strings.TrimPrefix(toReq, "acme-"), `{}`))
	cancelled := workItem(t, srv, "acme-4", 3)
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/" + cancelled + "/cancel", `{}`})
	unlinked := workItem(t, srv, "acme-4", 4)
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/unlink", `{"target":"` + unlinked + `"}`})
	workItem(t, srv, "1", 5) // on another Spec
	ready := workItem(t, srv, "acme-4", 6)
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/" + ready + "/approve", `{}`})
	incomingIssue(t, srv, "1", "7") // a GitHub issue

	watch := httpOK(t, srv, getWorkItemWatch("acme"))
	var got []string
	for _, i := range watch["issues"].([]any) {
		i := i.(map[string]any)
		l := i["link"].(map[string]any)
		got = append(got, fmt.Sprintf("%s %s %s %s %s %s", i["ticket"], i["title"], i["description"], l["integration"], l["id"], l["url"]))
	}
	want := []string{
		waiting + " Work item 1 Body 1 azure-boards contoso#1 https://dev.azure.com/contoso/web/_workitems/edit/1",
		ready + " Work item 6 Body 6 azure-boards contoso#6 https://dev.azure.com/contoso/web/_workitems/edit/6",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("get_work_item_watch issues = %q, want %q", got, want)
	}
	linked := toStrings(watch["linked"])
	if want := []string{"contoso#1", "contoso#2", "contoso#3", "contoso#4", "contoso#5", "contoso#6"}; !slices.Equal(linked, want) {
		t.Fatalf("get_work_item_watch linked = %v, want %v", linked, want)
	}
}

// With both Integrations on, each watch lists only its own Integration's
// Incoming Issues and writes: a work item's status label is a tag, and it
// is never closed.
func TestEachWatchListsOnlyItsOwnIntegrationsIncomingIssuesAndWrites(t *testing.T) {
	srv := newServer(t, issuesOn(false), boardsOn(false)) // acme-4 and acme-5
	issue := createIncomingIssue(t, srv, "acme-4", 7)
	item := workItem(t, srv, "acme-5", 12)

	if got := watchTickets(httpOK(t, srv, getIssueWatch("acme"))); !slices.Equal(got, []string{issue}) {
		t.Fatalf("get_issue_watch issues = %v, want only %s", got, issue)
	}
	if got := watchTickets(httpOK(t, srv, getWorkItemWatch("acme"))); !slices.Equal(got, []string{item}) {
		t.Fatalf("get_work_item_watch issues = %v, want only %s", got, item)
	}
	if got := toStrings(httpOK(t, srv, getWorkItemWatch("acme"))["linked"]); !slices.Equal(got, []string{"contoso#12"}) {
		t.Fatalf("get_work_item_watch linked = %v, want only contoso#12", got)
	}
	issueDue, itemDue := writesDue(t, srv), workItemWritesDue(t, srv)
	wantDue(t, issueDue, issue, "code-central:draft", "")
	wantTagDue(t, itemDue, item, "code-central:draft")
	if len(issueDue) != 1 || len(itemDue) != 1 {
		t.Fatalf("writes due %v and %v, want one each", issueDue, itemDue)
	}

	httpOK(t, srv, wontFix(strings.TrimPrefix(issue, "acme-"), `{"reason":"No."}`))
	httpOK(t, srv, wontFix(strings.TrimPrefix(item, "acme-"), `{"reason":"  Not ours.  "}`))
	issueDue, itemDue = writesDue(t, srv), workItemWritesDue(t, srv)
	wantDue(t, issueDue, issue, "code-central:wont-fix", "not_planned", "wont-fix")
	wantTagDue(t, itemDue, item, "code-central:wont-fix", "wont-fix")
	if _, ok := issueDue[item]; ok {
		t.Fatalf("get_issue_watch has %s's writes, want only GitHub's", item)
	}
	// The comment is GitHub's, with the same marker and link back.
	want := "<!-- code-central " + item + " wont-fix -->\nNot ours.\n\n[" + item + " in Code Central](" + srv.URL + "/?view=inbox&ticket=" + item + ")"
	if body := itemDue[item].Comments[0].Body; body != want {
		t.Fatalf("wont-fix comment = %q, want %q", body, want)
	}

	// Once written, the comment is never due again.
	httpOK(t, srv, reportIssueWritten(`{"target":"`+item+`","key":"wont-fix"}`))
	wantTagDue(t, workItemWritesDue(t, srv), item, "code-central:wont-fix")
}

// A Requirement made from a work item has its comments and tag due on the
// work item as GitHub's would, and never a close, even once Finished;
// get_issue_watch lists none of it.
func TestARequirementFromAWorkItemIsNeverClosed(t *testing.T) {
	srv := newServer(t, boardsOn(true)) // acme-4
	workItem(t, srv, "acme-4", 12)      // acme-5
	httpOK(t, srv, toRequirement("5", `{"context":"Seen on Monday."}`))
	wantTagDue(t, workItemWritesDue(t, srv), "acme-6", "code-central:requirement", "context")

	implementationTicket(t, srv, "Fix the login") // acme-10
	for _, n := range []string{"7", "8", "9", "10"} {
		httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/" + n + "/move", `{"to_state":"Done"}`})
	}
	wantTagDue(t, workItemWritesDue(t, srv), "acme-6", "code-central:done", "context", "spec", "landed:acme-10", "closed")
	if len(writesDue(t, srv)) != 0 {
		t.Fatalf("get_issue_watch writes = %v, want none for a work item", writesDue(t, srv))
	}
}

// Finished with nothing landed, a GitHub issue would be closed as not
// planned: a work item is only tagged.
func TestAWorkItemWhoseRequirementLandedNothingIsOnlyTagged(t *testing.T) {
	srv := newServer(t, boardsOn(false))
	workItem(t, srv, "acme-4", 12)
	httpOK(t, srv, toRequirement("5", `{}`))
	for _, n := range []string{"7", "8", "9"} {
		httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/" + n + "/cancel", `{}`})
	}
	wantTagDue(t, workItemWritesDue(t, srv), "acme-6", "code-central:wont-fix")
}

// Keep as Draft, To Requirement and Won't fix each work on an Incoming
// Issue on the Azure Boards Spec.
func TestTheTriageActionsWorkOnTheAzureBoardsSpec(t *testing.T) {
	srv := newServer(t, boardsOn(true)) // acme-4
	kept := workItem(t, srv, "acme-4", 1)
	toReq := workItem(t, srv, "acme-4", 2)
	wont := workItem(t, srv, "acme-4", 3)
	for _, id := range []string{kept, toReq, wont} {
		tk := httpOK(t, srv, httpCall{"GET", "/api/p/acme/tickets/" + strings.TrimPrefix(id, "acme-"), ""})
		if got := tk["triage_actions"]; !slices.Equal(strs(got), triageActions) {
			t.Fatalf("%s's triage_actions = %v, want %v", id, got, triageActions)
		}
	}

	httpOK(t, srv, keepAsDraft(strings.TrimPrefix(kept, "acme-")))
	if row, ok := needsMeRowOf(t, srv, kept); ok {
		t.Fatalf("%s's row after keep_as_draft = %+v, want none", kept, row)
	}

	httpOK(t, srv, toRequirement(strings.TrimPrefix(toReq, "acme-"), `{"context":"Urgent."}`))
	spec := httpOK(t, srv, httpCall{"GET", "/api/p/acme/specs/8", ""})
	link, _ := spec["link"].(map[string]any)
	if link["integration"] != "azure-boards" || link["id"] != "contoso#2" || link["outbound"] != true {
		t.Fatalf("Requirement acme-8's link = %v, want an outbound Link to contoso#2", spec["link"])
	}

	tk := httpOK(t, srv, wontFix(strings.TrimPrefix(wont, "acme-"), `{"reason":"Not ours.","full":true}`))
	field("state", "Cancelled")(t, tk)
	if l := tk["link"].(map[string]any); l["detached"] == true {
		t.Fatalf("link = %v, want it still attached", l)
	}
}

// Each triage action's note, which the UI shows before the human
// confirms, follows the Incoming Issue's Integration: Won't fix leaves a
// work item's State for the human to move.
func TestTheTriageNotesFollowTheIncomingIssuesIntegration(t *testing.T) {
	srv := newServer(t, issuesOn(true), boardsOn(true)) // acme-4 and acme-5
	issue := createIncomingIssue(t, srv, "acme-4", 7)
	item := workItem(t, srv, "acme-5", 12)
	notes := func(id string) map[string]any {
		tk := httpOK(t, srv, httpCall{"GET", "/api/p/acme/tickets/" + strings.TrimPrefix(id, "acme-"), ""})
		n, _ := tk["triage_notes"].(map[string]any)
		if len(n) != len(triageActions) {
			t.Fatalf("%s's triage_notes = %v, want one for each of %v", id, tk["triage_notes"], triageActions)
		}
		return n
	}
	gh, ab := notes(issue), notes(item)
	for _, c := range []struct {
		notes  map[string]any
		action string
		says   []string
	}{
		{gh, "wont_fix", []string{"closes the issue", "not planned"}},
		{gh, "keep_as_draft", []string{"adds nothing to the issue on GitHub"}},
		{ab, "wont_fix", []string{"code-central:wont-fix", "State", "Azure DevOps"}},
		{ab, "keep_as_draft", []string{"adds nothing to the work item in Azure Boards"}},
		{ab, "to_requirement", []string{"work item"}},
	} {
		for _, s := range c.says {
			if note := str(c.notes[c.action]); !strings.Contains(note, s) {
				t.Errorf("%s note %q, want it to say %q", c.action, note, s)
			}
		}
	}
	if note := str(ab["wont_fix"]); strings.Contains(note, "closes") {
		t.Errorf("a work item's wont_fix note %q, want it never to say it closes", note)
	}
	// On a Ticket that isn't an Incoming Issue there are none.
	httpOK(t, srv, keepAsDraft(strings.TrimPrefix(item, "acme-")))
	httpOK(t, srv, wontFix(strings.TrimPrefix(item, "acme-"), `{}`))
	if n, ok := httpOK(t, srv, httpCall{"GET", "/api/p/acme/tickets/" + strings.TrimPrefix(item, "acme-"), ""})["triage_notes"]; ok {
		t.Fatalf("triage_notes once Cancelled = %v, want none", n)
	}
}

// Awaiting triage follows the New issues need me of the Integration whose
// inbox the Incoming Issue arrives on.
func TestAwaitingTriageFollowsEachIntegrationsOwnNewIssuesNeedMe(t *testing.T) {
	for _, c := range []struct {
		issuesNeedMe, boardsNeedMe bool
	}{{false, true}, {true, false}} {
		srv := newServer(t, issuesOn(c.issuesNeedMe), boardsOn(c.boardsNeedMe)) // acme-4 and acme-5
		issue := createIncomingIssue(t, srv, "acme-4", 7)
		item := workItem(t, srv, "acme-5", 12)
		for id, want := range map[string]bool{issue: c.issuesNeedMe, item: c.boardsNeedMe} {
			row, ok := needsMeRowOf(t, srv, id)
			if ok != want || ok && row.Reason != core.ReasonNewIssue {
				t.Fatalf("issues %v, boards %v: %s's row = %+v (%v), want New issue %v", c.issuesNeedMe, c.boardsNeedMe, id, row, ok, want)
			}
			tk := httpOK(t, srv, httpCall{"GET", "/api/p/acme/tickets/" + strings.TrimPrefix(id, "acme-"), ""})
			boolField(t, tk, "awaiting_triage", want)
		}
	}
}

// The integration Actor follows a work item as it does a GitHub issue:
// edits, leaving the query's results and coming back.
func TestTheIntegrationActorFollowsAWorkItem(t *testing.T) {
	srv := newServer(t, boardsOn(true))
	item := workItem(t, srv, "acme-4", 12) // acme-5
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/integration/tickets/5/update_linked_ticket", `{"title":"Work item 12, edited"}`})
	field("title", "Work item 12, edited")(t, ticket5(t, srv))

	httpOK(t, srv, httpCall{"POST", "/api/p/acme/integration/set_external_state", `{"target":"5","external_state":"out_of_scope"}`})
	if row, _ := needsMeRowOf(t, srv, item); row.Reason != core.ReasonClosedExternally {
		t.Fatalf("out of scope: reason %q, want closed externally", row.Reason)
	}
	if got := workItemWritesDue(t, srv); len(got) != 0 {
		t.Fatalf("writes due out of scope = %v, want none", got)
	}
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/integration/set_external_state", `{"target":"5","external_state":"open"}`})
	if row, _ := needsMeRowOf(t, srv, item); row.Reason != core.ReasonNewIssue {
		t.Fatalf("back in scope: reason %q, want New issue", row.Reason)
	}
}

// The status report is the Work Item Watcher's alone: neither an agent
// nor the human may make it.
func TestReportWorkItemWatcherStatusIsRefusedFromAnyoneButTheIntegrationActor(t *testing.T) {
	srv := newServer(t, boardsOn(false))
	for _, actor := range []core.Actor{core.Agent, core.Human} {
		status, b := httpDoAs(t, srv, actor, reportWorkItemWatcherStatus(`{"state":"watching"}`))
		if e := errorJSON(t, b, core.ActionNotAllowed); status != httpapi.Status(core.ActionNotAllowed) || !strings.Contains(e.Message, "integration") {
			t.Fatalf("as %s: status %d, want %d; message %q", actor, status, httpapi.Status(core.ActionNotAllowed), e.Message)
		}
	}
	workItemWatcher(t, httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""}), "not_reported", "", "")
}

func TestTheWorkItemWatchersStatusShowsInAzureBoards(t *testing.T) {
	srv := newServer(t, boardsOn(false))
	project := func() map[string]any { return httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""}) }
	workItemWatcher(t, project(), "not_reported", "", "")

	httpOK(t, srv, reportWorkItemWatcherStatus(`{"state":"error","problem":"pat_rejected","message":"Azure DevOps rejected the PAT","fix":"Renew it."}`))
	workItemWatcher(t, project(), "error", "Azure DevOps rejected the PAT", "Renew it.")
	// The Issue Watcher's status is its own.
	issueWatcher(t, project(), "not_reported", "", "")

	httpOK(t, srv, reportWorkItemWatcherStatus(`{"state":"watching","rate_limited_until":"2026-01-05T10:15:00Z"}`))
	w := project()["azure_boards"].(map[string]any)["watcher"].(map[string]any)
	if w["state"] != "watching" || w["rate_limited_until"] != "2026-01-05T10:15:00Z" {
		t.Fatalf("azure_boards.watcher = %v, want watching, rate-limited until 10:15", w)
	}

	// Changing the query forgets it, as it was about the old one.
	httpOK(t, srv, reportWorkItemWatcherStatus(`{"state":"watching"}`))
	httpOK(t, srv, setAzureBoards(`{"on":true,"query_url":"`+apiQuery+`"}`))
	workItemWatcher(t, project(), "not_reported", "", "")
	// Turning Azure Boards off forgets it too.
	httpOK(t, srv, reportWorkItemWatcherStatus(`{"state":"watching"}`))
	httpOK(t, srv, setAzureBoards(`{"on":false,"query_url":"`+apiQuery+`"}`))
	workItemWatcher(t, project(), "not_reported", "", "")
}

// Only a Work Item Watcher's problems are accepted, and only while Azure
// Boards is on.
func TestReportWorkItemWatcherStatusRefusesWhatIsntTheWorkItemWatchers(t *testing.T) {
	srv := newServer(t)
	status, b := httpDo(t, srv, reportWorkItemWatcherStatus(`{"state":"watching"}`))
	if e := errorJSON(t, b, core.ActionNotAllowed); status != httpapi.Status(core.ActionNotAllowed) || !strings.Contains(e.Message, "Azure Boards is off") {
		t.Fatalf("status %d; message %q, want Azure Boards off", status, e.Message)
	}
	boardsOn(false)(t, srv)
	for _, body := range []string{
		`{"state":"error","problem":"crashed","message":"crashed"}`,
		`{"state":"error","problem":"logged_out","message":"gh is logged out"}`,
		`{"state":"error","message":"no","repositories":[{"repository":"web","message":"no"}]}`,
	} {
		status, b := httpDo(t, srv, reportWorkItemWatcherStatus(body))
		if errorJSON(t, b, core.Invalid); status != httpapi.Status(core.Invalid) {
			t.Fatalf("%s: status %d, want invalid", body, status)
		}
	}
	for _, problem := range []string{"pat_missing", "pat_rejected", "pat_scope", "project_unreadable", "query_unusable", "too_many_work_items", "check_failed"} {
		httpOK(t, srv, reportWorkItemWatcherStatus(`{"state":"error","problem":"`+problem+`","message":"no"}`))
	}
}

// The service runs a Work Item Watcher for each Project with Azure Boards
// on, and its crash is an error like any other kind's.
func TestTheServiceRunsAWorkItemWatcherWhileAzureBoardsIsOn(t *testing.T) {
	srv := newServer(t)
	if got := srv.Core.WatchedProjects(t.Context(), core.WatcherWorkItems); len(got) != 0 {
		t.Fatalf("watched while off = %v, want none", got)
	}
	boardsOn(false)(t, srv)
	if got := srv.Core.WatchedProjects(t.Context(), core.WatcherWorkItems); !slices.Equal(got, []string{"acme"}) {
		t.Fatalf("watched = %v, want acme", got)
	}
	if core.WatcherWorkItems.Name() != "Work Item Watcher" || core.WatcherWorkItems.Setting() != "Azure Boards" {
		t.Fatalf("kind %q named %q, set by %q", core.WatcherWorkItems, core.WatcherWorkItems.Name(), core.WatcherWorkItems.Setting())
	}
	srv.Core.ReportWatcherCrash(t.Context(), core.WatcherWorkItems, "acme", core.WatcherCrash{Restarts: 1, RestartAt: codecentraltest.Start})
	w := httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""})["azure_boards"].(map[string]any)["watcher"].(map[string]any)
	if w["state"] != "error" || w["problem"] != "crashed" || !strings.Contains(str(w["message"]), "Work Item Watcher crashed") {
		t.Fatalf("azure_boards.watcher = %v, want the crash", w)
	}
}
