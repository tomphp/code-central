package server_test

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
)

// Hide Code Central (Spec code-central-1050): while it is on,
// get_work_item_watch lists each comment in the hidden form, with an
// opaque marker and no link back, and no status tag; the same comments
// come due at the same points under the same keys. get_issue_watch
// doesn't change.

// opaque is the opaque marker of the comment key on target: the first 12
// hex digits of the SHA-256 of the two joined by a space. It is worked
// out here, apart from core.OpaqueMarker, so the tests check the Spec's
// definition rather than the code against itself.
func opaque(target, key string) string {
	sum := sha256.Sum256([]byte(target + " " + key))
	return "<!-- " + hex.EncodeToString(sum[:])[:12] + " -->"
}

// The opaque marker is the same in every release: acme-5's grilling
// comment always carries this hash.
func TestTheOpaqueMarkerIsAStableHashOfTheTargetAndKey(t *testing.T) {
	want := "<!-- 6bfc160dceca -->"
	if got := core.OpaqueMarker("acme-5", "grilling"); got != want || opaque("acme-5", "grilling") != want {
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

	httpOK(t, srv, wontFix(strings.TrimPrefix(issue, "acme-"), `{"reason":"No."}`))
	httpOK(t, srv, wontFix(strings.TrimPrefix(item, "acme-"), `{"reason":"  Not ours.  "}`))
	itemDue := workItemWritesDue(t, srv)
	wantTagDue(t, itemDue, item, "", "wont-fix")
	wantHidden(t, itemDue[item])
	if body, want := itemDue[item].Comments[0].Body, opaque(item, "wont-fix")+"\nNot ours."; body != want {
		t.Fatalf("wont-fix comment = %q, want %q", body, want)
	}

	issueDue := writesDue(t, srv)
	wantDue(t, issueDue, issue, "code-central:wont-fix", "not_planned", "wont-fix")
	want := "<!-- code-central " + issue + " wont-fix -->\nNo.\n\n[" + issue + " in Code Central](" + srv.URL + "/?view=inbox&ticket=" + issue + ")"
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
