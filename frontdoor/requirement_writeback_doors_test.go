package frontdoor_test

import (
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
)

// A Requirement's progress written back to its issue (code-central-291):
// as the Requirement from an issue goes on, get_issue_watch makes due the
// grilling resolution, the Spec's body, each landed Ticket and what
// landed, and the label follows the work; once the Spec is Finished, the
// issue is closed. requirementFromIssue makes acme-6 from issue 7, with
// Stages acme-7 (grilling), acme-8 (spec) and acme-9 (tickets).

// asAgent is c made as the agent, which must succeed.
func asAgent(t *testing.T, srv *codecentraltest.Server, c httpCall) map[string]any {
	t.Helper()
	status, b := httpDoAs(t, srv, core.Agent, c)
	if status != 200 {
		t.Fatalf("%s %s as the agent: status %d: %s", c.method, c.path, status, b)
	}
	return object(t, b)
}

// claimedByAgent is the agent's claim on acme-n, returning its Claim ID.
func claimedByAgent(t *testing.T, srv *codecentraltest.Server, n string) string {
	t.Helper()
	srv.Clock.Advance(time.Minute)
	return asAgent(t, srv, httpCall{"POST", "/api/p/acme/claim", `{"ticket":"acme-` + n + `"}`})["claim_id"].(string)
}

// agentComments is the agent's comment on acme-n.
func agentComments(t *testing.T, srv *codecentraltest.Server, n, body string) {
	t.Helper()
	srv.Clock.Advance(time.Minute)
	asAgent(t, srv, httpCall{"POST", "/api/p/acme/tickets/" + n + "/comment", `{"body":` + quote(body) + `}`})
}

// finishedAndAccepted is the agent's finish of acme-n under claim with
// summary, and the human's accept, to Done or, with a branch, Accepted.
func finishedAndAccepted(t *testing.T, srv *codecentraltest.Server, n, claim, summary, branch string) {
	t.Helper()
	srv.Clock.Advance(time.Minute)
	asAgent(t, srv, httpCall{"POST", "/api/p/acme/tickets/" + n + "/finish",
		`{"claim_id":` + quote(claim) + `,"summary":` + quote(summary) + `,"branch":` + quote(branch) + `}`})
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/" + n + "/accept", `{}`})
}

// landedWith is the land of acme-n, Accepted, with commit.
func landedWith(t *testing.T, srv *codecentraltest.Server, n, commit string) {
	t.Helper()
	srv.Clock.Advance(time.Minute)
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/" + n + "/land", `{"commit":` + quote(commit) + `}`})
}

// implementationTicket is the agent's Ready implementation Ticket title
// on acme-6.
func implementationTicket(t *testing.T, srv *codecentraltest.Server, title string) {
	t.Helper()
	asAgent(t, srv, httpCall{"POST", "/api/p/acme/create_ticket", `{"spec":"acme-6","title":` + quote(title) + `,"state":"Ready"}`})
}

// dueComment is the body of the comment key due on target.
func dueComment(t *testing.T, got map[string]issueWrites, target, key string) string {
	t.Helper()
	for _, c := range got[target].Comments {
		if c.Key == key {
			return c.Body
		}
	}
	t.Fatalf("%s has no %s comment due: %+v", target, key, got[target])
	return ""
}

// backTo is the link back to acme-6 a comment ends with.
func backTo(srv *codecentraltest.Server) string {
	return "\n\n[acme-6 in Code Central](" + srv.URL + "/?view=inbox&list=requirements&requirement=acme-6)"
}

// Each key comes due at its state of the work, with its body: the
// grilling resolution once the grilling Stage is Done, skipping its finish
// summary and never a Round, an answer or a Failure Report; the Spec's
// body as the spec Stage left it; each implementation Ticket once it is
// Done, with its land commit; and what landed once the Spec is Finished,
// closing the issue as completed. The label follows the work.
func TestARequirementsProgressComesDueOnItsIssue(t *testing.T) {
	srv := newServer(t, requirementFromIssue)
	httpOK(t, srv, reportIssueWritten(`{"target":"acme-6","key":"context"}`))
	wantDue(t, writesDue(t, srv), "acme-6", "code-central:requirement", "")

	// The grilling Stage: a Round answered, a Failure Report, the
	// resolution, a human's comment and the finish summary.
	claim := claimedByAgent(t, srv, "7")
	asAgent(t, srv, httpCall{"POST", "/api/p/acme/tickets/7/post_round", `{"claim_id":` + quote(claim) + `,"questions":[{"title":"DB?","body":"Which one?"}]}`})
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/7/answer", `{"question":1,"text":"Postgres"}`})
	asAgent(t, srv, httpCall{"POST", "/api/p/acme/tickets/7/pick_up_answers", `{"claim_id":` + quote(claim) + `}`})
	agentComments(t, srv, "7", "An early thought.")
	asAgent(t, srv, httpCall{"POST", "/api/p/acme/tickets/7/report_failure",
		`{"claim_id":` + quote(claim) + `,"what_broke":"The broken build","why":"A missing tool","what_to_fix":"Install the tool","where_work_stands":"Half grilled"}`})
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/7/to_draft", `{}`})
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/7/approve", `{}`})
	claim = claimedByAgent(t, srv, "7")
	agentComments(t, srv, "7", "## Resolution\n\nUse Postgres.")
	srv.Clock.Advance(time.Minute)
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/7/comment", `{"body":"Thanks."}`})
	if w, ok := writesDue(t, srv)["acme-6"]; ok && len(w.Comments) != 0 {
		t.Fatalf("comments due before the grilling Stage is Done: %+v, want none", w.Comments)
	}
	finishedAndAccepted(t, srv, "7", claim, "Grilled: see the resolution.", "")
	got := writesDue(t, srv)
	wantDue(t, got, "acme-6", "code-central:requirement", "", "grilling")
	if body, want := dueComment(t, got, "acme-6", "grilling"), "<!-- code-central acme-6 grilling -->\n## Resolution\n\nUse Postgres."+backTo(srv); body != want {
		t.Fatalf("grilling comment = %q, want %q", body, want)
	}
	httpOK(t, srv, reportIssueWritten(`{"target":"acme-6","key":"grilling"}`))

	// The spec Stage: the Spec's body as it was when the Stage was Done,
	// not after a later edit.
	claim = claimedByAgent(t, srv, "8")
	asAgent(t, srv, httpCall{"POST", "/api/p/acme/specs/6/edit_spec", `{"body":"## Problem\n\nLogin fails.\n"}`})
	finishedAndAccepted(t, srv, "8", claim, "Spec written.", "")
	srv.Clock.Advance(time.Minute)
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/specs/6/edit_spec", `{"body":"Edited later."}`})
	got = writesDue(t, srv)
	wantDue(t, got, "acme-6", "code-central:requirement", "", "spec")
	if body, want := dueComment(t, got, "acme-6", "spec"), "<!-- code-central acme-6 spec -->\n## Problem\n\nLogin fails."+backTo(srv); body != want {
		t.Fatalf("spec comment = %q, want %q", body, want)
	}
	httpOK(t, srv, reportIssueWritten(`{"target":"acme-6","key":"spec"}`))

	// The tickets Stage: the build starts.
	claim = claimedByAgent(t, srv, "9")
	implementationTicket(t, srv, "Fix the login")  // acme-10
	implementationTicket(t, srv, "Test the login") // acme-11
	finishedAndAccepted(t, srv, "9", claim, "Tickets made.", "")
	wantDue(t, writesDue(t, srv), "acme-6", "code-central:building", "")

	// An implementation Ticket Accepted hasn't landed: nothing is due for
	// it until it is Done.
	claim = claimedByAgent(t, srv, "10")
	finishedAndAccepted(t, srv, "10", claim, "Fixed.", "acme-10")
	wantDue(t, writesDue(t, srv), "acme-6", "code-central:building", "")
	landedWith(t, srv, "10", "abc123")
	got = writesDue(t, srv)
	wantDue(t, got, "acme-6", "code-central:building", "", "landed:acme-10")
	if body, want := dueComment(t, got, "acme-6", "landed:acme-10"), "<!-- code-central acme-6 landed:acme-10 -->\nLanded **Fix the login** (acme-10) in abc123."+backTo(srv); body != want {
		t.Fatalf("landed comment = %q, want %q", body, want)
	}

	// Once the last lands, the Spec is Finished: what landed, in the order
	// it landed, and closing as completed.
	claim = claimedByAgent(t, srv, "11")
	finishedAndAccepted(t, srv, "11", claim, "Tested.", "acme-11")
	landedWith(t, srv, "11", "def456")
	got = writesDue(t, srv)
	wantDue(t, got, "acme-6", "code-central:done", "completed", "landed:acme-10", "landed:acme-11", "closed")
	want := "<!-- code-central acme-6 closed -->\nFinished. What landed:\n\n- **Fix the login** (acme-10) in abc123\n- **Test the login** (acme-11) in def456" + backTo(srv)
	if body := dueComment(t, got, "acme-6", "closed"); body != want {
		t.Fatalf("closed comment = %q, want %q", body, want)
	}
	for _, c := range got["acme-6"].Comments {
		for _, never := range []string{"Postgres\"", "An early thought", "Thanks", "Grilled", "broken build", "missing tool", "Install the tool", "Half grilled", "Which one?"} {
			if strings.Contains(c.Body, never) {
				t.Fatalf("%s comment %q has %q in it", c.Key, c.Body, never)
			}
		}
	}
}

// A Spec Finished with nothing landed closes its issue as not planned,
// with code-central:wont-fix and no comment.
func TestASpecFinishedWithNothingLandedClosesItsIssueAsNotPlanned(t *testing.T) {
	srv := newServer(t, requirementFromIssue)
	httpOK(t, srv, reportIssueWritten(`{"target":"acme-6","key":"context"}`))
	for _, n := range []string{"7", "8", "9"} {
		httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/" + n + "/move", `{"to_state":"Cancelled"}`})
	}
	wantDue(t, writesDue(t, srv), "acme-6", "code-central:wont-fix", "not_planned")
}

// A Cancelled implementation Ticket doesn't count as landed, but the
// Spec is Finished once it and the rest are: as completed when another
// one landed.
func TestACancelledImplementationTicketIsLeftOutOfWhatLanded(t *testing.T) {
	srv := newServer(t, requirementFromIssue)
	httpOK(t, srv, reportIssueWritten(`{"target":"acme-6","key":"context"}`))
	implementationTicket(t, srv, "Fix the login")  // acme-10
	implementationTicket(t, srv, "Test the login") // acme-11
	for _, n := range []string{"7", "8", "9", "10"} {
		srv.Clock.Advance(time.Minute)
		httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/" + n + "/move", `{"to_state":"Done"}`})
	}
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/11/move", `{"to_state":"Cancelled"}`})
	got := writesDue(t, srv)
	// The grilling Stage left no resolution, so none is due; moved to
	// Done by hand, acme-10 has no land commit.
	wantDue(t, got, "acme-6", "code-central:done", "completed", "spec", "landed:acme-10", "closed")
	if body := dueComment(t, got, "acme-6", "closed"); !strings.Contains(body, "- **Fix the login** (acme-10)\n") || strings.Contains(body, "acme-11") {
		t.Fatalf("closed comment = %q, want only acme-10, with no commit", body)
	}
}
