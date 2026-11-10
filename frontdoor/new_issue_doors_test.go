package frontdoor_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/httpapi"
)

// New issue and Keep as Draft (code-central-287): with New issues need
// me on, an Incoming Issue the Integration creates on the Issues Spec
// waits in Needs Me as New issue until the human triages it.
// keep_as_draft, the human's, is the first triage action, so HTTP only.
func init() {
	cases["keep_as_draft"] = opCase{
		setup: newIssue,
		ok:    call{http: keepAsDraft("5")},
		check: ticketAcknowledgement("acme-5", "Draft", "needs_me_reason", "awaiting_triage"),
		full: func(t *testing.T, out map[string]any) {
			boolField(t, out, "awaiting_triage", false)
			field("id", "acme-5")(t, out)
		},
		bad:  call{http: keepAsDraft("2")},
		code: core.ActionNotAllowed,
	}
}

func keepAsDraft(n string) httpCall {
	return httpCall{"POST", "/api/p/acme/tickets/" + n + "/keep_as_draft", `{}`}
}

// issuesOn turns GitHub Issues on for acme, creating its Issues Spec,
// acme-4, with New issues need me as needMe.
func issuesOn(needMe bool) func(*testing.T, *codecentraltest.Server) {
	return func(t *testing.T, srv *codecentraltest.Server) {
		t.Helper()
		body := `{"on":true,"repository":"acme/web","new_issues_need_me":` + strconv.FormatBool(needMe) + `}`
		httpOK(t, srv, setGitHubIssues(body))
	}
}

// incomingIssue is the Integration's create_linked_ticket for issue n of
// acme/web, in Draft on spec.
func incomingIssue(t *testing.T, srv *codecentraltest.Server, spec string, n string) {
	t.Helper()
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/integration/create_linked_ticket",
		`{"spec":"` + spec + `","title":"Issue ` + n + `","link":{"integration":"github","id":"acme/web#` + n + `","url":"https://github.com/acme/web/issues/` + n + `"}}`})
}

// newIssue is New issues need me on, and issue 7 arrived as acme-5.
func newIssue(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	issuesOn(true)(t, srv)
	incomingIssue(t, srv, "acme-4", "7")
}

// ticket5 is acme-5 as get_ticket reads it.
func ticket5(t *testing.T, srv *codecentraltest.Server) map[string]any {
	t.Helper()
	return httpOK(t, srv, httpCall{"GET", "/api/p/acme/tickets/5", ""})
}

func TestANewIncomingIssueWaitsInNeedsMeAsNewIssue(t *testing.T) {
	srv := newServer(t, newIssue)
	row, ok := needsMeRowOf(t, srv, "acme-5")
	if !ok || row.Reason != core.ReasonNewIssue || !row.Since.Equal(codecentraltest.Start) {
		t.Fatalf("acme-5's row = %+v (%v), want New issue since its creation", row, ok)
	}
	if row.Link == nil || row.Link.URL != "https://github.com/acme/web/issues/7" {
		t.Fatalf("link = %+v, want the issue's", row.Link)
	}
	tk := ticket5(t, srv)
	field("needs_me_reason", "new_issue")(t, tk)
	boolField(t, tk, "awaiting_triage", true)
	if got := tk["triage_actions"]; !slices.Equal(strs(got), triageActions) {
		t.Fatalf("triage_actions = %v, want %v", got, triageActions)
	}
}

func TestANewIssueSinceIsItsCreationNotItsLastWrite(t *testing.T) {
	srv := newServer(t, newIssue)
	srv.Clock.Advance(time.Hour)
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/integration/tickets/5/update_linked_ticket", `{"title":"Issue 7, edited"}`})
	if row, _ := needsMeRowOf(t, srv, "acme-5"); row.Reason != core.ReasonNewIssue || !row.Since.Equal(codecentraltest.Start) {
		t.Fatalf("row = %+v, want New issue since its creation", row)
	}
}

func TestWithNewIssuesNeedMeOffAnIncomingIssueIsNotANewIssue(t *testing.T) {
	srv := newServer(t, issuesOn(false))
	incomingIssue(t, srv, "acme-4", "7")
	if row, ok := needsMeRowOf(t, srv, "acme-5"); ok {
		t.Fatalf("row = %+v, want none", row)
	}
	tk := ticket5(t, srv)
	boolField(t, tk, "awaiting_triage", false)
	// It is still an Incoming Issue, so the human may still triage it.
	if got := tk["triage_actions"]; !slices.Equal(strs(got), triageActions) {
		t.Fatalf("triage_actions = %v, want %v", got, triageActions)
	}
	// Turning New issues need me on later doesn't make it new.
	issuesOn(true)(t, srv)
	if row, ok := needsMeRowOf(t, srv, "acme-5"); ok {
		t.Fatalf("row after turning it on = %+v, want none", row)
	}
}

func TestALinkedTicketOnAnotherSpecIsNotANewIssue(t *testing.T) {
	srv := newServer(t, issuesOn(true))
	incomingIssue(t, srv, "acme-1", "7")
	if row, ok := needsMeRowOf(t, srv, "acme-5"); ok {
		t.Fatalf("row = %+v, want none", row)
	}
	tk := ticket5(t, srv)
	boolField(t, tk, "awaiting_triage", false)
	if _, ok := tk["triage_actions"]; ok {
		t.Fatalf("triage_actions = %v, want none off the Issues Spec", tk["triage_actions"])
	}
}

func TestClosedExternallyTakesOverFromNewIssue(t *testing.T) {
	srv := newServer(t, newIssue)
	setExternal := func(state string) {
		httpOK(t, srv, httpCall{"POST", "/api/p/acme/integration/set_external_state", `{"target":"5","external_state":"` + state + `"}`})
	}
	setExternal("closed")
	if row, _ := needsMeRowOf(t, srv, "acme-5"); row.Reason != core.ReasonClosedExternally {
		t.Fatalf("closed: reason %q, want closed externally", row.Reason)
	}
	// Open again, it is a New issue again: nobody triaged it.
	setExternal("open")
	if row, _ := needsMeRowOf(t, srv, "acme-5"); row.Reason != core.ReasonNewIssue {
		t.Fatalf("reopened: reason %q, want New issue", row.Reason)
	}
}

func TestNewIssueIsGroupedJustBeforeClosedExternally(t *testing.T) {
	order := workflowNeedsMe(t)
	n, c := slices.Index(order, core.ReasonNewIssue), slices.Index(order, core.ReasonClosedExternally)
	if n < 0 || c != n+1 {
		t.Fatalf("group order %v, want new_issue just before closed_externally", order)
	}
	// So in Needs Me a New issue row comes after a Blocked by a Cancelled
	// Ticket row and before a closed externally one.
	srv := newServer(t, newIssue)
	incomingIssue(t, srv, "acme-4", "8") // acme-6
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/integration/set_external_state", `{"target":"6","external_state":"closed"}`})
	nm := httpOK(t, srv, httpCall{"GET", "/api/needs-me", ""})
	var got []string
	for _, g := range nm["groups"].([]any) {
		got = append(got, g.(map[string]any)["reason"].(string))
	}
	if !slices.Equal(got, []string{"new_issue", "closed_externally"}) {
		t.Fatalf("groups %v, want new_issue then closed_externally", got)
	}
}

func TestKeepAsDraftTakesANewIssueOutOfNeedsMeForGood(t *testing.T) {
	srv := newServer(t, newIssue)
	httpOK(t, srv, keepAsDraft("5"))
	if row, ok := needsMeRowOf(t, srv, "acme-5"); ok {
		t.Fatalf("row after keep_as_draft = %+v, want none", row)
	}
	tk := ticket5(t, srv)
	field("state", "Draft")(t, tk)
	if _, ok := tk["needs_me_reason"]; ok {
		t.Fatalf("needs_me_reason = %v, want none", tk["needs_me_reason"])
	}
	// It stays an Incoming Issue, so it is still offered, and keeping it
	// again changes nothing.
	if got := tk["triage_actions"]; !slices.Equal(strs(got), triageActions) {
		t.Fatalf("triage_actions = %v, want %v", got, triageActions)
	}
	acknowledgement("acme-5")(t, httpOK(t, srv, keepAsDraft("5")))
	// Closed and reopened, edited, or New issues need me turned off and
	// on, it never comes back as a New issue.
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/integration/set_external_state", `{"target":"5","external_state":"closed"}`})
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/integration/set_external_state", `{"target":"5","external_state":"open"}`})
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/integration/tickets/5/update_linked_ticket", `{"title":"Issue 7, edited"}`})
	issuesOn(false)(t, srv)
	issuesOn(true)(t, srv)
	if row, ok := needsMeRowOf(t, srv, "acme-5"); ok {
		t.Fatalf("row = %+v, want none ever again", row)
	}
}

func TestKeepAsDraftIsRefusedOnAnythingButAnIncomingIssue(t *testing.T) {
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
		e := httpRejected(t, srv, keepAsDraft(n), core.ActionNotAllowed)
		if !strings.Contains(e.Message, "acme-"+n) || !strings.Contains(e.Message, why) {
			t.Errorf("acme-%s: message %q, want it named and %q", n, e.Message, why)
		}
	}
	httpRejected(t, srv, keepAsDraft("99"), core.NotFound)
}

func TestKeepAsDraftIsRejectedForAnAgent(t *testing.T) {
	srv := newServer(t, newIssue)
	status, b := httpDoAs(t, srv, core.Agent, keepAsDraft("5"))
	if e := errorJSON(t, b, core.ActionNotAllowed); status != httpapi.Status(core.ActionNotAllowed) || !strings.Contains(e.Message, "human") {
		t.Fatalf("status %d; message %q, want the human's", status, e.Message)
	}
	if row, _ := needsMeRowOf(t, srv, "acme-5"); row.Reason != core.ReasonNewIssue {
		t.Fatalf("reason %q, want still New issue", row.Reason)
	}
}

// triageActions are the triage actions offered on every Incoming Issue,
// in order.
var triageActions = []string{"keep_as_draft", "to_requirement", "wont_fix"}

// strs is a JSON array of strings.
func strs(v any) []string {
	var out []string
	a, _ := v.([]any)
	for _, s := range a {
		out = append(out, s.(string))
	}
	return out
}
