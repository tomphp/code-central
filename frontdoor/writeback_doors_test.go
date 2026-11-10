package frontdoor_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/httpapi"
)

// Write-backs and Won't fix (code-central-289): get_issue_watch lists
// what is due on each linked issue, worked out from the work's state;
// report_issue_written, the integration Actor's, records a comment
// written; wont_fix, the human's, is a triage action, so HTTP only.
func init() {
	cases["wont_fix"] = opCase{
		setup: newIssue,
		ok:    call{http: wontFix("5", `{"reason":"Works as intended."}`)},
		check: ticketAcknowledgement("acme-5", "Cancelled", "state", "comments", "moves", "needs_me_reason", "awaiting_triage", "triage_actions"),
		full: func(t *testing.T, out map[string]any) {
			field("state", "Cancelled")(t, out)
			boolField(t, out, "awaiting_triage", false)
		},
		bad:  call{http: wontFix("2", `{}`)},
		code: core.ActionNotAllowed,
	}
	cases["report_issue_written"] = opCase{
		setup: newIssue,
		ok:    call{http: reportIssueWritten(`{"target":"acme-5","key":"wont-fix"}`)},
		check: func(t *testing.T, out map[string]any) {
			field("id", "acme-5")(t, out)
			field("id", "acme/web#7")(t, out["link"].(map[string]any))
		},
		bad:  call{http: reportIssueWritten(`{"target":"acme-2","key":"wont-fix"}`)},
		code: core.Invalid,
	}
}

func wontFix(n, body string) httpCall {
	return httpCall{"POST", "/api/p/acme/tickets/" + n + "/wont_fix", body}
}

func reportIssueWritten(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/integration/report_issue_written", body}
}

// issueWrites is an item whose issue has writes due, as get_issue_watch
// lists it.
type issueWrites struct {
	Target string `json:"target"`
	Link   struct {
		ID string `json:"id"`
	} `json:"link"`
	Comments []struct {
		Key  string `json:"key"`
		Body string `json:"body"`
	} `json:"comments"`
	Label string `json:"label"`
	Close string `json:"close"`
}

// writesDue is acme's items with writes due, by target.
func writesDue(t *testing.T, srv *codecentraltest.Server) map[string]issueWrites {
	t.Helper()
	b, err := json.Marshal(httpOK(t, srv, getIssueWatch("acme"))["writes"])
	if err != nil {
		t.Fatal(err)
	}
	var list []issueWrites
	if err := json.Unmarshal(b, &list); err != nil {
		t.Fatal(err)
	}
	out := map[string]issueWrites{}
	for _, w := range list {
		out[w.Target] = w
	}
	return out
}

// keys are w's comment keys, in order.
func (w issueWrites) keys() []string {
	var out []string
	for _, c := range w.Comments {
		out = append(out, c.Key)
	}
	return out
}

// wantDue checks one item's writes due: its label, its closing and its
// comment keys.
func wantDue(t *testing.T, got map[string]issueWrites, target, label, close string, keys ...string) {
	t.Helper()
	w, ok := got[target]
	if !ok {
		t.Fatalf("%s has no writes due, want %s, close %q, comments %v", target, label, close, keys)
	}
	if w.Label != label || w.Close != close || !slices.Equal(w.keys(), keys) {
		t.Fatalf("%s's writes due = %+v, want %s, close %q, comments %v", target, w, label, close, keys)
	}
}

// What is due comes from each item's state: an Incoming Issue should
// carry code-central:draft; one the human chose Won't fix for should
// carry code-central:wont-fix and be closed as not planned, with the
// reason as a comment if given. Nothing is due on a plain Cancel, a
// detached Link or a deleted issue, and on any other issue that isn't
// open, only a comment is.
func TestGetIssueWatchListsTheWritesDueForEachState(t *testing.T) {
	srv := newServer(t, newIssue)         // acme-5 is issue 7, Draft
	incomingIssue(t, srv, "acme-4", "8")  // acme-6, approved to Ready
	incomingIssue(t, srv, "acme-4", "9")  // acme-7, Won't fix with a reason
	incomingIssue(t, srv, "acme-4", "10") // acme-8, Won't fix with none
	incomingIssue(t, srv, "acme-4", "11") // acme-9, cancelled plainly
	incomingIssue(t, srv, "acme-4", "12") // acme-10, unlinked
	incomingIssue(t, srv, "acme-4", "13") // acme-11, closed on GitHub
	incomingIssue(t, srv, "acme-4", "14") // acme-12, Won't fix, then closed
	incomingIssue(t, srv, "acme-4", "15") // acme-13, Won't fix, then deleted
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/6/approve", `{}`})
	httpOK(t, srv, wontFix("7", `{"reason":"  Works as intended.  "}`))
	httpOK(t, srv, wontFix("8", `{}`))
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/9/cancel", `{"reason":"Duplicate."}`})
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/unlink", `{"target":"acme-10"}`})
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/integration/set_external_state", `{"target":"11","external_state":"closed"}`})
	httpOK(t, srv, wontFix("12", `{"reason":"Out of scope."}`))
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/integration/set_external_state", `{"target":"12","external_state":"closed"}`})
	httpOK(t, srv, wontFix("13", `{"reason":"Gone."}`))
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/integration/set_external_state", `{"target":"13","external_state":"deleted"}`})
	// A comment on an Incoming Issue is the Ticket's, never the issue's.
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/5/comment", `{"body":"Looks like a duplicate."}`})

	got := writesDue(t, srv)
	wantDue(t, got, "acme-5", "code-central:draft", "")
	wantDue(t, got, "acme-6", "code-central:draft", "")
	wantDue(t, got, "acme-7", "code-central:wont-fix", "not_planned", "wont-fix")
	wantDue(t, got, "acme-8", "code-central:wont-fix", "not_planned")
	wantDue(t, got, "acme-12", "", "", "wont-fix")
	for _, none := range []string{"acme-9", "acme-10", "acme-11", "acme-13"} {
		if w, ok := got[none]; ok {
			t.Errorf("%s has writes due %+v, want none", none, w)
		}
	}
	if w := got["acme-7"]; w.Link.ID != "acme/web#9" {
		t.Fatalf("acme-7's link %q, want acme/web#9", w.Link.ID)
	}

	// The comment is the reason, with the hidden marker naming the item
	// and key, and ends with a link back to the Ticket.
	body := got["acme-7"].Comments[0].Body
	want := "<!-- code-central acme-7 wont-fix -->\nWorks as intended.\n\n[acme-7 in Code Central](" + srv.URL + "/?view=inbox&ticket=acme-7)"
	if body != want {
		t.Fatalf("wont-fix comment = %q, want %q", body, want)
	}
}

// Turning GitHub Issues off doesn't stop work coming due: it is due when
// it is turned back on.
func TestWritesComeDueWhileGitHubIssuesIsOff(t *testing.T) {
	srv := newServer(t, newIssue)
	issuesOn(true)(t, srv)
	httpOK(t, srv, setGitHubIssues(`{"on":false,"repository":"acme/web"}`))
	httpOK(t, srv, wontFix("5", `{"reason":"No."}`))
	issuesOn(true)(t, srv)
	wantDue(t, writesDue(t, srv), "acme-5", "code-central:wont-fix", "not_planned", "wont-fix")
}

// Once reported written, a comment is never due again; reporting it
// again changes nothing.
func TestReportIssueWrittenStopsACommentBeingDue(t *testing.T) {
	srv := newServer(t, newIssue)
	httpOK(t, srv, wontFix("5", `{"reason":"No."}`))
	for range 2 {
		httpOK(t, srv, reportIssueWritten(`{"target":"acme-5","key":"wont-fix"}`))
		wantDue(t, writesDue(t, srv), "acme-5", "code-central:wont-fix", "not_planned")
	}
	// The Ticket's own history doesn't count it as an edit.
	if edits, _ := ticket5(t, srv)["edits"].([]any); len(edits) != 0 {
		t.Fatalf("edits = %v, want none", edits)
	}
}

func TestReportIssueWrittenIsRefusedFromAnyoneButTheIntegrationActor(t *testing.T) {
	srv := newServer(t, newIssue)
	httpOK(t, srv, wontFix("5", `{"reason":"No."}`))
	for _, actor := range []core.Actor{core.Agent, core.Human} {
		status, b := httpDoAs(t, srv, actor, reportIssueWritten(`{"target":"acme-5","key":"wont-fix"}`))
		if e := errorJSON(t, b, core.ActionNotAllowed); status != httpapi.Status(core.ActionNotAllowed) || !strings.Contains(e.Message, "integration") {
			t.Fatalf("as %s: status %d; message %q, want the integration Actor's", actor, status, e.Message)
		}
	}
	httpRejected(t, srv, reportIssueWritten(`{"target":"acme-5","key":" "}`), core.Invalid)
	wantDue(t, writesDue(t, srv), "acme-5", "code-central:wont-fix", "not_planned", "wont-fix")
}

// Won't fix Cancels the Incoming Issue, with the reason as its comment,
// and takes it out of Needs Me; its Link stays attached.
func TestWontFixCancelsAnIncomingIssueWithItsReason(t *testing.T) {
	srv := newServer(t, newIssue)
	httpOK(t, srv, wontFix("5", `{"reason":"Works as intended."}`))
	if row, ok := needsMeRowOf(t, srv, "acme-5"); ok {
		t.Fatalf("row after wont_fix = %+v, want none", row)
	}
	tk := ticket5(t, srv)
	field("state", "Cancelled")(t, tk)
	boolField(t, tk, "awaiting_triage", false)
	if _, ok := tk["triage_actions"]; ok {
		t.Fatalf("triage_actions = %v, want none once Cancelled", tk["triage_actions"])
	}
	comments, _ := tk["comments"].([]any)
	if len(comments) != 1 || comments[0].(map[string]any)["body"] != "Works as intended." || comments[0].(map[string]any)["actor"] != "human" {
		t.Fatalf("comments = %v, want the human's reason", comments)
	}
	moves, _ := tk["moves"].([]any)
	if last := moves[len(moves)-1].(map[string]any); last["to"] != "Cancelled" || last["via"] != "wont_fix" {
		t.Fatalf("last move = %v, want to Cancelled via wont_fix", last)
	}
	link := tk["link"].(map[string]any)
	if link["detached"] == true {
		t.Fatalf("link = %v, want it still attached", link)
	}
}

func TestWontFixIsRefusedOnAnythingButAnIncomingIssue(t *testing.T) {
	srv := newServer(t, newIssue)
	incomingIssue(t, srv, "acme-1", "8")  // acme-6, linked but on another Spec
	incomingIssue(t, srv, "acme-4", "9")  // acme-7, detached below
	incomingIssue(t, srv, "acme-4", "10") // acme-8, cancelled below
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/unlink", `{"target":"acme-7"}`})
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/8/move", `{"to_state":"Cancelled"}`})
	for n, why := range map[string]string{
		"2": "not an Incoming Issue",
		"6": "not an Incoming Issue",
		"7": "unlinked",
		"8": "Cancelled",
	} {
		e := httpRejected(t, srv, wontFix(n, `{"reason":"No."}`), core.ActionNotAllowed)
		if !strings.Contains(e.Message, "acme-"+n) || !strings.Contains(e.Message, why) {
			t.Errorf("acme-%s: message %q, want it named and %q", n, e.Message, why)
		}
	}
	if w, ok := writesDue(t, srv)["acme-8"]; ok {
		t.Fatalf("acme-8 has writes due %+v, want none: it was cancelled, not Won't fix", w)
	}
}

func TestWontFixIsRejectedForAnAgent(t *testing.T) {
	srv := newServer(t, newIssue)
	status, b := httpDoAs(t, srv, core.Agent, wontFix("5", `{}`))
	if e := errorJSON(t, b, core.ActionNotAllowed); status != httpapi.Status(core.ActionNotAllowed) || !strings.Contains(e.Message, "human") {
		t.Fatalf("status %d; message %q, want the human's", status, e.Message)
	}
	field("state", "Draft")(t, ticket5(t, srv))
}
