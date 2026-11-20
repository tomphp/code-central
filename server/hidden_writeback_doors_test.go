package server_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/tomoram/code-central/domain"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/codecentraltest"
)

// Hide Code Central (Spec code-central-1050): while it is on,
// get_work_item_watch lists each comment in the hidden form, with an
// opaque marker and no link back, and no status tag; the same comments
// come due at the same points under the same keys. get_issue_watch
// doesn't change.

// opaque is the opaque marker of the comment key on target: the first 12
// hex digits of the SHA-256 of the two joined by a space. It is worked
// out here, apart from domain.OpaqueMarker, so the tests check the Spec's
// definition rather than the code against itself.
func opaque(target, key string) string {
	sum := sha256.Sum256([]byte(target + " " + key))
	return "<!-- " + hex.EncodeToString(sum[:])[:12] + " -->"
}

// The opaque marker is the same in every release: acme-5's grilling
// comment always carries this hash.
func TestTheOpaqueMarkerIsAStableHashOfTheTargetAndKey(t *testing.T) {
	want := "<!-- 6bfc160dceca -->"
	if got := domain.OpaqueMarker("acme-5", "grilling"); got != want || opaque("acme-5", "grilling") != want {
		t.Fatalf("opaque marker = %q, want %q", got, want)
	}
}

// wantHidden checks w's comments are each in the hidden form: the opaque
// marker for its key, a newline and the text, with nothing naming Code
// Central.
func wantHidden(t *testing.T, w workItemWrites) {
	t.Helper()
	if slices.Contains(w.Keys, "tag") {
		t.Fatalf("%s's work item writes have a tag %q, want none", w.Target, w.Tag)
	}
	for _, c := range w.Comments {
		if !strings.HasPrefix(c.Body, opaque(w.Target, c.Key)+"\n") {
			t.Fatalf("%s comment %q, want it to start with %s", c.Key, c.Body, opaque(w.Target, c.Key))
		}
		for _, never := range []string{"code-central", "Code Central", "?view=inbox"} {
			if strings.Contains(c.Body, never) {
				t.Fatalf("%s comment %q has %q in it", c.Key, c.Body, never)
			}
		}
	}
}

// A Won't fix comment on an Incoming Issue is the opaque marker and the
// reason alone, with no tag; get_issue_watch's GitHub writes stay as they
// are.
func TestHiddenWontFixIsTheMarkerAndTheReasonAloneAndGitHubIsUnchanged(t *testing.T) {
	srv := newServer(t, issuesOn(false), boardsHidden(false, true)) // acme-4 and acme-5
	issue := createIncomingIssue(t, srv, "acme-4", 7)
	item := workItem(t, srv, "acme-5", 12)
	if got := workItemWritesDue(t, srv); len(got) != 0 {
		t.Fatalf("work item writes due on a Draft = %+v, want none: no tag and no comment", got)
	}
	wantDue(t, writesDue(t, srv), issue, "code-central:draft", "")

	httpOK(t, srv, wontFix(strings.TrimPrefix(issue, "acme-"), `{"reason":"No, see acme-5."}`))
	httpOK(t, srv, wontFix(strings.TrimPrefix(item, "acme-"), `{"reason":"  Not ours.  "}`))
	itemDue := workItemWritesDue(t, srv)
	wantTagDue(t, itemDue, item, "", "wont-fix")
	wantHidden(t, itemDue[item])
	if body, want := itemDue[item].Comments[0].Body, opaque(item, "wont-fix")+"\nNot ours."; body != want {
		t.Fatalf("wont-fix comment = %q, want %q", body, want)
	}

	issueDue := writesDue(t, srv)
	wantDue(t, issueDue, issue, "code-central:wont-fix", "not_planned", "wont-fix")
	want := "<!-- code-central " + issue + " wont-fix -->\nNo, see acme-5.\n\n[" + issue + " in Code Central](" + srv.URL + "/?view=inbox&ticket=" + issue + ")"
	if body := issueDue[issue].Comments[0].Body; body != want {
		t.Fatalf("GitHub's wont-fix comment = %q, want %q", body, want)
	}

	httpOK(t, srv, reportIssueWritten(`{"target":"`+item+`","key":"wont-fix"}`))
	if got := workItemWritesDue(t, srv); len(got) != 0 {
		t.Fatalf("work item writes due once written = %+v, want none", got)
	}
}

// A Requirement from a work item has the same comments due as it would
// branded, each in the hidden form, and never a tag.
func TestAHiddenRequirementHasEveryCommentInTheHiddenFormAndNoTag(t *testing.T) {
	srv := newServer(t, boardsHidden(true, true)) // acme-4
	workItem(t, srv, "acme-4", 12)                // acme-5
	httpOK(t, srv, toRequirement("5", `{"context":"  Seen on Monday.  "}`))
	got := workItemWritesDue(t, srv)
	wantTagDue(t, got, "acme-6", "", "context")
	wantHidden(t, got["acme-6"])
	if body, want := got["acme-6"].Comments[0].Body, opaque("acme-6", "context")+"\nSeen on Monday."; body != want {
		t.Fatalf("context comment = %q, want %q", body, want)
	}

	implementationTicket(t, srv, "Fix the login") // acme-10
	for _, n := range []string{"7", "8", "9", "10"} {
		httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/" + n + "/move", `{"to_state":"Done"}`})
	}
	got = workItemWritesDue(t, srv)
	wantTagDue(t, got, "acme-6", "", "context", "spec", "landed:acme-10", "closed")
	wantHidden(t, got["acme-6"])
	for _, c := range got["acme-6"].Comments {
		httpOK(t, srv, reportIssueWritten(`{"target":"acme-6","key":"`+c.Key+`"}`))
	}
	if got := workItemWritesDue(t, srv); len(got) != 0 {
		t.Fatalf("work item writes due once written = %+v, want none", got)
	}
}

// Turning Hide Code Central on part-way changes only the comments not yet
// written, and never makes one due again; turning it off brings the tag
// back.
func TestTurningHideCodeCentralOnPartWayChangesOnlyWhatIsNotYetWritten(t *testing.T) {
	srv := newServer(t, boardsOn(true)) // acme-4
	workItem(t, srv, "acme-4", 12)      // acme-5
	httpOK(t, srv, toRequirement("5", `{"context":"Seen on Monday."}`))
	got := workItemWritesDue(t, srv)
	wantTagDue(t, got, "acme-6", "code-central:requirement", "context")
	if body := got["acme-6"].Comments[0].Body; !strings.HasPrefix(body, "<!-- code-central acme-6 context -->\n") {
		t.Fatalf("context comment before the setting = %q, want it branded", body)
	}
	httpOK(t, srv, reportIssueWritten(`{"target":"acme-6","key":"context"}`))

	boardsHidden(true, true)(t, srv)
	if got := workItemWritesDue(t, srv); len(got) != 0 {
		t.Fatalf("work item writes due once hidden = %+v, want none: the context stays written", got)
	}
	for _, n := range []string{"7", "8"} {
		httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/" + n + "/move", `{"to_state":"Done"}`})
	}
	got = workItemWritesDue(t, srv)
	wantTagDue(t, got, "acme-6", "", "spec")
	wantHidden(t, got["acme-6"])
	httpOK(t, srv, reportIssueWritten(`{"target":"acme-6","key":"spec"}`))

	boardsHidden(true, false)(t, srv)
	wantTagDue(t, workItemWritesDue(t, srv), "acme-6", "code-central:requirement")
}

// hiddenRequirementWithItems is Hide Code Central on, work item 12 as
// acme-5 ("Work item 12") taken To Requirement with context as acme-6,
// Stages acme-7 to acme-9, then Spec acme-10 "Add the export button" and
// Ticket acme-11 "Fix the login" for the written text to name.
func hiddenRequirementWithItems(t *testing.T, context string) *codecentraltest.Server {
	t.Helper()
	srv := newServer(t, boardsHidden(true, true)) // acme-4
	workItem(t, srv, "acme-4", 12)                // acme-5
	httpOK(t, srv, toRequirement("5", `{"context":`+quote(context)+`}`))
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/create_spec", `{"title":"Add the export button","body":"Export."}`})
	implementationTicket(t, srv, "Fix the login")
	return srv
}

// The Spec's body names this Project's existing Specs and Tickets by
// quoted title: a bare ID, an ID Link whose text is the ID, and the text
// alone of an ID Link with other text, with IDs in that text rewritten
// too. Other keys, missing numbers, IDs inside longer words, IDs or URLs,
// and code spans and blocks, fenced and indented, are left as written.
func TestAHiddenSpecBodyNamesItsSpecsAndTicketsByTitle(t *testing.T) {
	srv := hiddenRequirementWithItems(t, "")
	body := "## Plan\n\n" +
		"Builds on acme-10, after [acme-11](acme-11). See [the export work](acme-10) and [acme-11 then acme-10](acme-11).\n" +
		"(acme-10) acme-11: ok; acme-11!\n" +
		"Left: other-10, acme-99, [gone](acme-99), acme-10x, xacme-10, acme-10-2, my-acme-10, acme-10_b, https://dev.azure.com/acme-10, acme/acme-10, acme-10.md, `acme-10` and ``a `acme-11` b``.\n\n" +
		"```sh\ngit switch acme-10\n```\n\n" +
		"    acme-10 indented\n\n" +
		"~~~~\nacme-11\n~~~\nstill acme-11\n~~~~\n\n" +
		"Ends with acme-11, _acme-10_, [a [b] acme-11](acme-11), [acme-10](<acme-10> \"t\") and ![shot of acme-11](acme-11).\n\n" +
		"- A list item\n\n  continued with acme-10\n\n      acme-10 indented in the list\n\n" +
		"Back out at acme-11"
	want := "## Plan\n\n" +
		"Builds on \"Add the export button\", after \"Fix the login\". See the export work and \"Fix the login\" then \"Add the export button\".\n" +
		"(\"Add the export button\") \"Fix the login\": ok; \"Fix the login\"!\n" +
		"Left: other-10, acme-99, [gone](acme-99), acme-10x, xacme-10, acme-10-2, my-acme-10, acme-10_b, https://dev.azure.com/acme-10, acme/acme-10, acme-10.md, `acme-10` and ``a `acme-11` b``.\n\n" +
		"```sh\ngit switch acme-10\n```\n\n" +
		"    acme-10 indented\n\n" +
		"~~~~\nacme-11\n~~~\nstill acme-11\n~~~~\n\n" +
		"Ends with \"Fix the login\", _\"Add the export button\"_, a [b] \"Fix the login\", \"Add the export button\" and shot of \"Fix the login\".\n\n" +
		"- A list item\n\n  continued with \"Add the export button\"\n\n      acme-10 indented in the list\n\n" +
		"Back out at \"Fix the login\""
	asAgent(t, srv, httpCall{"POST", "/api/p/acme/specs/6/edit_spec", `{"body":` + quote(body) + `}`})
	for _, n := range []string{"7", "8"} {
		httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/" + n + "/move", `{"to_state":"Done"}`})
	}
	got := workItemWritesDue(t, srv)
	wantTagDue(t, got, "acme-6", "", "spec")
	if body, want := got["acme-6"].Comments[0].Body, opaque("acme-6", "spec")+"\n"+want; body != want {
		t.Fatalf("spec comment =\n%s\nwant\n%s", body, want)
	}
}

// The To Requirement context, a Won't fix reason and the grilling
// resolution are rewritten the same way, and the rest of the prose,
// "Code Central" included, is posted as written. With the setting off
// they are posted as written, IDs and all.
func TestHiddenContextWontFixReasonAndResolutionNameItemsByTitle(t *testing.T) {
	srv := hiddenRequirementWithItems(t, "Seen with acme-11, not `acme-11`.")
	item := workItem(t, srv, "acme-4", 13) // acme-12
	httpOK(t, srv, wontFix(strings.TrimPrefix(item, "acme-"), `{"reason":"Done in [acme-10](acme-10) and [the login fix](acme-11)."}`))
	agentComments(t, srv, "7", "## Resolution\n\n- Use acme-10:\n\n    after acme-11, in Code Central\n- Not acme-12345.")
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/7/move", `{"to_state":"Done"}`})

	got := workItemWritesDue(t, srv)
	wantTagDue(t, got, "acme-6", "", "context", "grilling")
	wantTagDue(t, got, item, "", "wont-fix")
	for _, c := range []struct{ target, key, want string }{
		{"acme-6", "context", "Seen with \"Fix the login\", not `acme-11`."},
		{"acme-6", "grilling", "## Resolution\n\n- Use \"Add the export button\":\n\n    after \"Fix the login\", in Code Central\n- Not acme-12345."},
		{item, "wont-fix", "Done in \"Add the export button\" and the login fix."},
	} {
		if body, want := dueWorkItemComment(t, got, c.target, c.key), opaque(c.target, c.key)+"\n"+c.want; body != want {
			t.Fatalf("%s %s comment = %q, want %q", c.target, c.key, body, want)
		}
	}

	boardsHidden(true, false)(t, srv)
	if body := dueWorkItemComment(t, workItemWritesDue(t, srv), "acme-6", "context"); !strings.Contains(body, "\nSeen with acme-11, not `acme-11`.\n") {
		t.Fatalf("context comment with the setting off = %q, want the context as written", body)
	}
}

// dueWorkItemComment is the body of the comment key due on target's work
// item.
func dueWorkItemComment(t *testing.T, got map[string]workItemWrites, target, key string) string {
	t.Helper()
	return dueComment(t, map[string]issueWrites{target: got[target].issueWrites}, target, key)
}

// hiddenLanded makes acme-6 a hidden Requirement with its Stages Done and
// one implementation Ticket, acme-10, "Follow acme-5", finished with the
// finish body fields, accepted and landed by each of lands.
func hiddenLanded(t *testing.T, srv *codecentraltest.Server, finish string, lands ...string) workItemWrites {
	t.Helper()
	workItem(t, srv, "acme-4", 12) // acme-5
	httpOK(t, srv, toRequirement("5", `{"context":"Seen."}`))
	httpOK(t, srv, reportIssueWritten(`{"target":"acme-6","key":"context"}`))
	for _, n := range []string{"7", "8", "9"} {
		srv.Clock.Advance(time.Minute)
		httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/" + n + "/move", `{"to_state":"Done"}`})
	}
	implementationTicket(t, srv, "Follow acme-5") // acme-10
	claim := claimedByAgent(t, srv, "10")
	srv.Clock.Advance(time.Minute)
	asAgent(t, srv, httpCall{"POST", "/api/p/acme/tickets/10/finish", `{"claim_id":` + quote(claim) + `,"summary":"Fixed.",` + finish + `}`})
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/10/accept", `{}`})
	for _, l := range lands {
		srv.Clock.Advance(time.Minute)
		httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/10/land", l})
	}
	w := workItemWritesDue(t, srv)["acme-6"]
	wantHidden(t, w)
	return w
}

func commentBody(w workItemWrites, key string) string {
	for _, c := range w.Comments {
		if c.Key == key {
			return c.Body
		}
	}
	return ""
}

// Hidden, a landed line has no Ticket ID and its title names other items
// by title, in each "Landed" comment and in the Finished list.
func TestAHiddenLandedLineHasNoTicketID(t *testing.T) {
	srv := newServer(t, boardsHidden(true, true)) // acme-4
	w := hiddenLanded(t, srv, `"branch":"acme-10","commit":"h1"`, `{"commit":"hh"}`)
	if got, want := commentBody(w, "landed:acme-10"), opaque("acme-6", "landed:acme-10")+"\nLanded **Follow \"Work item 12\"** in hh."; got != want {
		t.Fatalf("landed comment = %q, want %q", got, want)
	}
	if got, want := commentBody(w, "closed"), opaque("acme-6", "closed")+"\nFinished. What landed:\n\n- **Follow \"Work item 12\"** in hh"; got != want {
		t.Fatalf("closed comment = %q, want %q", got, want)
	}
}

// With several Repositories the hidden line is Name@sha per Changed
// Repository, and the Changed Repositories line is as branded.
func TestAHiddenLandedLineWithSeveralRepositoriesHasNoTicketID(t *testing.T) {
	srv := newServer(t, boardsHidden(true, true)) // acme-4
	human := api.WithActor(context.Background(), api.Human)
	if _, err := srv.Core.AddRepository(human, "acme", api.AddRepositoryInput{Name: "docs", CloneURL: "https://dev.azure.com/org/proj/_git/docs"}); err != nil {
		t.Fatal(err)
	}
	w := hiddenLanded(t, srv, `"branch":"acme-10","commit":"h1","repositories":[{"name":"docs","branch":"acme-10","commit":"d1"}]`,
		`{"commit":"dd","repository":"docs"}`, `{"commit":"hh","repository":"acme"}`)
	if got, want := commentBody(w, "landed:acme-10"), opaque("acme-6", "landed:acme-10")+"\nLanded **Follow \"Work item 12\"** in acme@hh, docs@dd."; got != want {
		t.Fatalf("landed comment = %q, want %q", got, want)
	}
	if got, want := commentBody(w, "closed"), opaque("acme-6", "closed")+"\nFinished. What landed:\n\n- **Follow \"Work item 12\"** in acme@hh, docs@dd\n\nChanged Repositories: acme, docs"; got != want {
		t.Fatalf("closed comment = %q, want %q", got, want)
	}
}
