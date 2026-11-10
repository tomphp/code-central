package frontdoor_test

import (
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/httpapi"
)

// To Requirement and the outbound Link (code-central-290): the human's
// to_requirement turns an Incoming Issue into a Requirement whose Spec
// has an outbound Link to the issue. Code Central owns its definition
// from then on, and only writes to the issue. It is a triage action, so
// HTTP only.
func init() {
	cases["to_requirement"] = opCase{
		setup: newIssue,
		ok:    call{http: toRequirement("5", `{"context":"Seen on every login."}`)},
		check: ticketAcknowledgement("acme-5", "Cancelled", "state", "replacements", "comments", "moves", "needs_me_reason", "link", "awaiting_triage", "triage_actions"),
		full: func(t *testing.T, out map[string]any) {
			field("state", "Cancelled")(t, out)
			if got := strs(out["replacements"]); !slices.Equal(got, []string{"acme-7"}) {
				t.Fatalf("replacements = %v, want the grilling Stage, acme-7", out["replacements"])
			}
		},
		bad:  call{http: toRequirement("2", `{}`)},
		code: core.ActionNotAllowed,
	}
}

func toRequirement(n, body string) httpCall {
	return httpCall{"POST", "/api/p/acme/tickets/" + n + "/to_requirement", body}
}

// requirementFromIssue is New issues need me on, issue 7 arriving as
// acme-5 with a body, and To Requirement with context: Requirement acme-6,
// whose Stages are acme-7 (grilling), acme-8 (spec) and acme-9 (tickets).
func requirementFromIssue(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	newIssue(t, srv)
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/integration/tickets/5/update_linked_ticket", `{"description":"Login fails with a 500."}`})
	httpOK(t, srv, toRequirement("5", `{"context":"  Seen on every login since Monday.  "}`))
}

// spec6 is acme-6 as get_spec reads it.
func spec6(t *testing.T, srv *codecentraltest.Server) map[string]any {
	t.Helper()
	return httpOK(t, srv, httpCall{"GET", "/api/p/acme/specs/6", ""})
}

// findLinked is find_linked's rows for issue id of acme/web.
func findLinked(t *testing.T, srv *codecentraltest.Server, id string) []map[string]any {
	t.Helper()
	rows, _ := httpOK(t, srv, httpCall{"GET", "/api/p/acme/integration/linked?integration=github&id=" + url.QueryEscape(id), ""})["rows"].([]any)
	var out []map[string]any
	for _, r := range rows {
		out = append(out, r.(map[string]any))
	}
	return out
}

// To Requirement creates the Requirement with the issue's title as a
// heading, then its body, then the context under its own heading, and
// moves the Link to its Spec as an outbound Link.
func TestToRequirementCreatesARequirementWhoseSpecHasAnOutboundLink(t *testing.T) {
	srv := newServer(t, requirementFromIssue)
	sp := spec6(t, srv)
	field("title", "Issue 7")(t, sp)
	field("body", "# Issue 7\n\nLogin fails with a 500.\n\n## Context\n\nSeen on every login since Monday.\n")(t, sp)
	link := sp["link"].(map[string]any)
	field("id", "acme/web#7")(t, link)
	field("url", "https://github.com/acme/web/issues/7")(t, link)
	boolField(t, link, "outbound", true)
	boolField(t, link, "detached", false)
	r, _ := sp["requirement"].(map[string]any)
	stages, _ := r["stages"].([]any)
	if len(stages) != 3 || stages[0].(map[string]any)["ticket"] != "acme-7" {
		t.Fatalf("requirement %v, want three Stages from acme-7", sp["requirement"])
	}
}

func TestToRequirementWithoutContextOrBodyIsTheTitleAlone(t *testing.T) {
	srv := newServer(t, newIssue)
	httpOK(t, srv, toRequirement("5", `{"context":" "}`))
	field("body", "# Issue 7\n")(t, spec6(t, srv))
	if w := writesDue(t, srv)["acme-6"]; len(w.Comments) != 0 {
		t.Fatalf("comments due %v, want none without context", w.keys())
	}
}

// The Incoming Issue is Cancelled with the grilling Stage as its
// Replacement, its Link detached, and leaves Needs Me.
func TestToRequirementCancelsTheIncomingIssueWithTheGrillingStageAsItsReplacement(t *testing.T) {
	srv := newServer(t, requirementFromIssue)
	tk := ticket5(t, srv)
	field("state", "Cancelled")(t, tk)
	boolField(t, tk, "awaiting_triage", false)
	if got := strs(tk["replacements"]); !slices.Equal(got, []string{"acme-7"}) {
		t.Fatalf("replacements = %v, want acme-7", tk["replacements"])
	}
	boolField(t, tk["link"].(map[string]any), "detached", true)
	moves, _ := tk["moves"].([]any)
	if last := moves[len(moves)-1].(map[string]any); last["to"] != "Cancelled" || last["via"] != "to_requirement" {
		t.Fatalf("last move = %v, want to Cancelled via to_requirement", last)
	}
	if row, ok := needsMeRowOf(t, srv, "acme-5"); ok {
		t.Fatalf("row after to_requirement = %+v, want none", row)
	}
}

// An outbound Link doesn't own the definition: the human and agents edit
// the Spec as if it weren't linked, and the Integration can't.
func TestAnOutboundSpecIsEditedByTheHumanAndAgentsAndNotTheIntegration(t *testing.T) {
	srv := newServer(t, requirementFromIssue)
	for _, actor := range []core.Actor{core.Human, core.Agent} {
		status, b := httpDoAs(t, srv, actor, httpCall{"POST", "/api/p/acme/specs/6/edit_spec", `{"body":"Rewritten by ` + string(actor) + `"}`})
		if status != 200 {
			t.Fatalf("edit_spec as %s: status %d: %s", actor, status, b)
		}
	}
	e := httpRejected(t, srv, httpCall{"POST", "/api/p/acme/integration/specs/6/update_linked_spec", `{"title":"From GitHub"}`}, core.ActionNotAllowed)
	if !strings.Contains(e.Message, "acme-6") || !strings.Contains(e.Message, "outbound") {
		t.Fatalf("message %q, want acme-6 named as outbound", e.Message)
	}
	field("body", "Rewritten by agent")(t, spec6(t, srv))
}

// find_linked returns both of the issue's items: the detached Incoming
// Issue and the outbound Spec, with the flag.
func TestFindLinkedReturnsAnOutboundLinkWithTheFlag(t *testing.T) {
	srv := newServer(t, requirementFromIssue)
	rows := findLinked(t, srv, "acme/web#7")
	if len(rows) != 2 || rows[0]["id"] != "acme-5" || rows[1]["id"] != "acme-6" {
		t.Fatalf("rows = %v, want acme-5 and acme-6", rows)
	}
	boolField(t, rows[0]["link"].(map[string]any), "outbound", false)
	boolField(t, rows[1]["link"].(map[string]any), "outbound", true)
	field("kind", "spec")(t, rows[1])
}

// The context comment is due once, and the issue should carry
// code-central:requirement while the Stages aren't all Done; nothing is
// due on the Incoming Issue any more.
func TestAnOutboundSpecHasItsContextCommentAndRequirementLabelDue(t *testing.T) {
	srv := newServer(t, requirementFromIssue)
	got := writesDue(t, srv)
	wantDue(t, got, "acme-6", "code-central:requirement", "", "context")
	if w, ok := got["acme-5"]; ok {
		t.Fatalf("acme-5 has writes due %+v, want none once detached", w)
	}
	if w := got["acme-6"]; w.Link.ID != "acme/web#7" {
		t.Fatalf("acme-6's link %q, want acme/web#7", w.Link.ID)
	}
	want := "<!-- code-central acme-6 context -->\nSeen on every login since Monday.\n\n[acme-6 in Code Central](" + srv.URL + "/?view=inbox&list=requirements&requirement=acme-6)"
	if body := got["acme-6"].Comments[0].Body; body != want {
		t.Fatalf("context comment = %q, want %q", body, want)
	}

	// Later edits to the Spec don't change the context written.
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/specs/6/edit_spec", `{"body":"Rewritten"}`})
	if body := writesDue(t, srv)["acme-6"].Comments[0].Body; body != want {
		t.Fatalf("context comment after editing the Spec = %q, want %q", body, want)
	}

	httpOK(t, srv, reportIssueWritten(`{"target":"acme-6","key":"context"}`))
	wantDue(t, writesDue(t, srv), "acme-6", "code-central:requirement", "")

	// Once every Stage is Done, it no longer asks for the label.
	for _, n := range []string{"7", "8", "9"} {
		httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/" + n + "/move", `{"to_state":"Done"}`})
	}
	if w, ok := writesDue(t, srv)["acme-6"]; ok {
		t.Fatalf("acme-6 has writes due %+v once its Stages are Done, want none", w)
	}
}

// The comment keys already written for the issue carry over to the
// Spec's Link.
func TestToRequirementCarriesOverTheCommentKeysWritten(t *testing.T) {
	srv := newServer(t, newIssue)
	httpOK(t, srv, reportIssueWritten(`{"target":"acme-5","key":"context"}`))
	httpOK(t, srv, toRequirement("5", `{"context":"Again."}`))
	wantDue(t, writesDue(t, srv), "acme-6", "code-central:requirement", "")
}

// Unlinking the outbound Spec detaches it, and stops every write to the
// issue.
func TestUnlinkingAnOutboundSpecStopsEveryWrite(t *testing.T) {
	srv := newServer(t, requirementFromIssue)
	row := httpOK(t, srv, httpCall{"POST", "/api/p/acme/unlink", `{"target":"acme-6"}`})
	boolField(t, row["link"].(map[string]any), "detached", true)
	if w, ok := writesDue(t, srv)["acme-6"]; ok {
		t.Fatalf("acme-6 has writes due %+v once unlinked, want none", w)
	}
}

// An outbound Link never counts for closed externally.
func TestAnOutboundSpecsStagesAreNeverClosedExternally(t *testing.T) {
	srv := newServer(t, requirementFromIssue)
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/integration/set_external_state", `{"target":"acme-6","external_state":"closed"}`})
	rows, err := srv.Client.NeedsMe(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range rows.Groups {
		if g.Reason == core.ReasonClosedExternally {
			t.Fatalf("group %+v is closed externally, want none", g)
		}
	}
	// Nor is a label or closing due on an issue that isn't open.
	if w, ok := writesDue(t, srv)["acme-6"]; !ok || w.Label != "" {
		t.Fatalf("acme-6's writes due %+v, want only the context comment", w)
	}
}

func TestToRequirementIsRefusedOnAnythingButAnIncomingIssue(t *testing.T) {
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
		e := httpRejected(t, srv, toRequirement(n, `{}`), core.ActionNotAllowed)
		if !strings.Contains(e.Message, "acme-"+n) || !strings.Contains(e.Message, why) {
			t.Errorf("acme-%s: message %q, want it named and %q", n, e.Message, why)
		}
	}
	if specs, _ := httpOK(t, srv, httpCall{"GET", "/api/p/acme/specs", ""})["rows"].([]any); len(specs) != 2 {
		t.Fatalf("specs = %v, want no Requirement written", specs)
	}
}

func TestToRequirementIsRejectedForAnAgent(t *testing.T) {
	srv := newServer(t, newIssue)
	status, b := httpDoAs(t, srv, core.Agent, toRequirement("5", `{}`))
	if e := errorJSON(t, b, core.ActionNotAllowed); status != httpapi.Status(core.ActionNotAllowed) || !strings.Contains(e.Message, "human") {
		t.Fatalf("status %d; message %q, want the human's", status, e.Message)
	}
	field("state", "Draft")(t, ticket5(t, srv))
}

// The context stays due if the human moves the Incoming Issue on before
// the Issue Watcher writes it.
func TestTheContextStaysDueAfterTheIncomingIssueIsMovedOn(t *testing.T) {
	srv := newServer(t, requirementFromIssue)
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/5/move", `{"to_state":"Draft"}`})
	wantDue(t, writesDue(t, srv), "acme-6", "code-central:requirement", "", "context")
}
