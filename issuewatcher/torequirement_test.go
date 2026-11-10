package issuewatcher_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/tomoram/code-central/core"
)

// toRequirement is the human's To Requirement on ticket with context,
// returning the Requirement's Spec.
func (w *watching) toRequirement(ticket, context string) string {
	w.t.Helper()
	res, err := w.srv.Core.ToRequirement(human, "acme", core.ToRequirementInput{Ticket: core.Ref(ticket), Context: context, Full: true})
	if err != nil {
		w.t.Fatal(err)
	}
	return res.Spec.Record.ID
}

// To Requirement with context posts the context once, with the marker
// and a link back to the Spec, and code-central:requirement replaces
// code-central:draft; then nothing more is due.
func TestToRequirementPostsTheContextOnceAndLabelsTheIssueRequirement(t *testing.T) {
	w := newWatching(t)
	spec := w.toRequirement(w.arrivedFirst(labelled(1, "code-central:draft")), "Seen on every login.")
	from := len(w.h.Calls())
	w.polled()
	body := fmt.Sprintf("body=<!-- code-central %s context -->\nSeen on every login.\n\n[%s in Code Central](%s/?view=inbox&list=requirements&requirement=%s)", spec, spec, w.srv.URL, spec)
	w.wantWrites(from,
		"POST repos/acme/web/issues/1/comments "+body,
		"POST repos/acme/web/issues/1/labels labels[]=code-central:requirement",
		"DELETE repos/acme/web/issues/1/labels/code-central:draft")
	if due := w.writesDue(); len(due) != 1 || len(due[0].Comments) != 0 {
		t.Fatalf("writes due after = %+v, want only the label", due)
	}
	w.openIssues("", 1, labelled(1, "code-central:requirement"))
	from = len(w.h.Calls())
	w.polled()
	w.wantWrites(from)
}

// Once To Requirement has snapshotted the issue, a later edit on GitHub
// changes neither the Requirement nor brings the issue in again.
func TestALaterGitHubEditDoesntChangeTheRequirement(t *testing.T) {
	w := newWatching(t)
	spec := w.toRequirement(w.arrivedFirst(labelled(1, "code-central:requirement")), "")
	edited := labelled(1, "code-central:requirement")
	edited.Title = "Issue 1, edited"
	w.openIssues("", 1, edited)
	w.polled()
	sp, err := w.srv.Core.GetSpec(context.Background(), "acme", core.GetSpecInput{Spec: core.Ref(spec)})
	if err != nil {
		t.Fatal(err)
	}
	if sp.Title != "Issue 1" || sp.Body == nil || *sp.Body != "# Issue 1\n\nBody 1\n" {
		t.Fatalf("spec = %+v, want the issue as To Requirement found it", sp)
	}
	if got := w.incoming(); len(got) != 0 {
		t.Fatalf("incoming = %v, want none", got)
	}
	if rows := w.linked(); len(rows) != 2 {
		t.Fatalf("linked = %+v, want the Incoming Issue and the Spec only", rows)
	}
}

// Unlinking the Requirement's Spec stops every write to the issue.
func TestAfterUnlinkingTheSpecNothingMoreIsWritten(t *testing.T) {
	w := newWatching(t)
	spec := w.toRequirement(w.arrivedFirst(labelled(1, "code-central:draft")), "Seen on every login.")
	if _, err := w.srv.Core.Unlink(human, "acme", core.UnlinkInput{Target: core.Ref(spec)}); err != nil {
		t.Fatal(err)
	}
	from := len(w.h.Calls())
	w.polled()
	w.wantWrites(from)
}
