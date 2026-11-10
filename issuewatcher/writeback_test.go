package issuewatcher_test

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/internal/fakegh"
)

// writes are the fake gh's calls since the from'th that write to GitHub,
// each as its method, path and fields, such as
// "POST repos/acme/web/issues/1/labels labels[]=code-central:draft".
func (w *watching) writes(from int) []string {
	w.t.Helper()
	var out []string
	for _, c := range w.h.Calls()[from:] {
		i := slices.Index(c.Args, "--method")
		if len(c.Args) < 2 || c.Args[0] != "api" || i < 0 {
			continue
		}
		line := c.Args[i+1] + " " + c.Args[1]
		for j, a := range c.Args {
			if a == "-f" {
				line += " " + c.Args[j+1]
			}
		}
		out = append(out, line)
	}
	return out
}

// wantWrites checks the fake gh was asked to write exactly want since the
// from'th call.
func (w *watching) wantWrites(from int, want ...string) {
	w.t.Helper()
	if got := w.writes(from); !slices.Equal(got, want) {
		w.t.Fatalf("writes to GitHub:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// labelled is openIssue(n), open, carrying labels.
func labelled(n int, labels ...string) issue {
	i := openIssue(n)
	for _, l := range labels {
		i.Labels = append(i.Labels, label{Name: l})
	}
	return i
}

// missingLabel scripts GitHub answering that acme/web has no label name.
func (w *watching) missingLabel(name string) {
	w.h.Answer(fakegh.Answer{Args: []string{"api", "repos/acme/web/labels/" + name}, Exit: 1,
		Stdout: "HTTP/2.0 404 Not Found\r\n\r\n" + `{"message":"Not Found"}`, Stderr: "gh: Not Found (HTTP 404)\n"})
}

// commentsOn scripts the comments on acme/web's issue n.
func (w *watching) commentsOn(n int, bodies ...string) {
	w.t.Helper()
	var cs []map[string]string
	for _, b := range bodies {
		cs = append(cs, map[string]string{"body": b})
	}
	b, err := json.Marshal(cs)
	if err != nil {
		w.t.Fatal(err)
	}
	w.h.Answer(fakegh.Answer{Args: []string{"api", fmt.Sprintf("repos/acme/web/issues/%d/comments?page=1&per_page=100", n)}, Stdout: string(b)})
}

// arrivedFirst starts the Issue Watcher with issues open, waits for them to
// arrive, and returns the first one's Ticket.
func (w *watching) arrivedFirst(issues ...issue) string {
	w.t.Helper()
	w.openIssues("", 1, issues...)
	w.start(w.h.Bin)
	w.status(core.WatcherWatching)
	watch := w.watch()
	if len(watch.Issues) == 0 {
		w.t.Fatalf("get_issue_watch = %+v, want the issues arrived", watch)
	}
	return watch.Issues[0].Ticket
}

// wontFix is the human's Won't fix on ticket with reason.
func (w *watching) wontFix(ticket, reason string) {
	w.t.Helper()
	if _, err := w.srv.Core.WontFix(human, "acme", core.WontFixInput{Ticket: core.Ref(ticket), Reason: reason}); err != nil {
		w.t.Fatal(err)
	}
}

// writesDue is what get_issue_watch says is still due.
func (w *watching) writesDue() []core.IssueWrites {
	w.t.Helper()
	return w.watch().Writes
}

// An Incoming Issue gets code-central:draft on the poll after it
// arrives, the label created in the repository first as it is missing.
func TestAnIncomingIssueIsLabelledDraftCreatingTheLabel(t *testing.T) {
	w := newWatching(t)
	w.missingLabel("code-central:draft")
	w.arrivedFirst(labelled(1))
	from := len(w.h.Calls())
	w.polled()
	w.wantWrites(from,
		"POST repos/acme/web/labels name=code-central:draft color=ededed description=Where Code Central's work on this issue stands",
		"POST repos/acme/web/issues/1/labels labels[]=code-central:draft")
}

// The Issue Watcher changes only what differs: an issue that has its
// label is left alone, and other status labels are taken off.
func TestOnlyTheLabelThatDiffersIsChanged(t *testing.T) {
	w := newWatching(t)
	w.arrivedFirst(labelled(1, "bug", "code-central:draft"), labelled(2, "code-central:wont-fix", "bug"))
	from := len(w.h.Calls())
	w.polled()
	w.wantWrites(from,
		"POST repos/acme/web/issues/2/labels labels[]=code-central:draft",
		"DELETE repos/acme/web/issues/2/labels/code-central:wont-fix")
}

// Won't fix with a reason posts one comment, with the marker and a link
// back, sets code-central:wont-fix and closes the issue as not planned;
// then nothing more is due, so the next poll writes nothing.
func TestWontFixWithAReasonCommentsLabelsAndClosesAsNotPlanned(t *testing.T) {
	w := newWatching(t)
	ticket := w.arrivedFirst(labelled(1, "code-central:draft"))
	w.wontFix(ticket, "Works as intended.")
	from := len(w.h.Calls())
	w.polled()
	body := fmt.Sprintf("body=<!-- code-central %s wont-fix -->\nWorks as intended.\n\n[%s in Code Central](%s/?view=inbox&ticket=%s)", ticket, ticket, w.srv.URL, ticket)
	w.wantWrites(from,
		"POST repos/acme/web/issues/1/comments "+body,
		"POST repos/acme/web/issues/1/labels labels[]=code-central:wont-fix",
		"DELETE repos/acme/web/issues/1/labels/code-central:draft",
		"PATCH repos/acme/web/issues/1 state=closed state_reason=not_planned")
	if due := w.writesDue(); len(due) != 0 {
		t.Fatalf("writes due after = %+v, want none", due)
	}
	from = len(w.h.Calls())
	w.polled()
	w.wantWrites(from)
}

// Won't fix with no reason only labels and closes the issue.
func TestWontFixWithoutAReasonOnlyCloses(t *testing.T) {
	w := newWatching(t)
	ticket := w.arrivedFirst(labelled(1, "code-central:draft"))
	w.wontFix(ticket, "")
	from := len(w.h.Calls())
	w.polled()
	w.wantWrites(from,
		"POST repos/acme/web/issues/1/labels labels[]=code-central:wont-fix",
		"DELETE repos/acme/web/issues/1/labels/code-central:draft",
		"PATCH repos/acme/web/issues/1 state=closed state_reason=not_planned")
}

// A crash between posting a comment and reporting it leaves the comment
// due, but its marker on the issue stops a second post: the Issue
// Watcher only reports it written.
func TestACommentAlreadyOnTheIssueIsReportedNotPostedAgain(t *testing.T) {
	w := newWatching(t)
	ticket := w.arrivedFirst(labelled(1, "code-central:wont-fix"))
	w.wontFix(ticket, "Works as intended.")
	w.commentsOn(1, "Me too.", core.IssueMarker(ticket, "wont-fix")+"\nWorks as intended.")
	from := len(w.h.Calls())
	w.polled()
	w.wantWrites(from, "PATCH repos/acme/web/issues/1 state=closed state_reason=not_planned")
	if due := w.writesDue(); len(due) != 0 {
		t.Fatalf("writes due after = %+v, want the comment reported written", due)
	}
}

// A restarted Issue Watcher keeps no state, and posts nothing twice.
func TestARestartedIssueWatcherPostsNothingTwice(t *testing.T) {
	w := newWatching(t)
	ticket := w.arrivedFirst(labelled(1, "code-central:draft"))
	w.wontFix(ticket, "Works as intended.")
	from := len(w.h.Calls())
	w.polled()
	w.stop()
	w.start(w.h.Bin)
	// The stopped loop's timer is still waiting too.
	w.srv.Clock.BlockUntil(2)
	w.polled()
	if got := w.writes(from); len(got) != 4 || !strings.HasPrefix(got[0], "POST repos/acme/web/issues/1/comments") {
		t.Fatalf("writes to GitHub:\n%s\nwant one comment, label, unlabel and close, once", strings.Join(got, "\n"))
	}
}

// Writes that come due while GitHub Issues is off are made once it is
// turned back on.
func TestWritesDueWhileGitHubIssuesWasOffAreMadeWhenItIsBackOn(t *testing.T) {
	w := newWatching(t)
	ticket := w.arrivedFirst(labelled(1, "code-central:draft"))
	if _, err := w.srv.Core.SetGitHubIssues(human, "acme", core.SetGitHubIssuesInput{On: false, Repository: "acme/web"}); err != nil {
		t.Fatal(err)
	}
	w.wontFix(ticket, "")
	from := len(w.h.Calls())
	w.polled()
	w.wantWrites(from)

	if _, err := w.srv.Core.SetGitHubIssues(human, "acme", core.SetGitHubIssuesInput{On: true, Repository: "acme/web"}); err != nil {
		t.Fatal(err)
	}
	w.polled()
	w.wantWrites(from,
		"POST repos/acme/web/issues/1/labels labels[]=code-central:wont-fix",
		"DELETE repos/acme/web/issues/1/labels/code-central:draft",
		"PATCH repos/acme/web/issues/1 state=closed state_reason=not_planned")
}

// An issue the poll didn't list, such as one already closed, is read
// before its label and closing are compared with it.
func TestAnIssueNotListedIsReadForItsLabelsAndState(t *testing.T) {
	w := newWatching(t)
	ticket := w.arrivedFirst(labelled(1, "code-central:draft"))
	w.wontFix(ticket, "")
	w.openIssues("", 1)
	w.h.Answer(fakegh.Answer{Args: []string{"api", "repos/acme/web/issues/1"},
		Stdout: `{"number":1,"state":"closed","labels":[{"name":"code-central:wont-fix"}]}`})
	from := len(w.h.Calls())
	w.polled()
	w.wantWrites(from)
	if due := w.writesDue(); len(due) != 0 {
		t.Fatalf("writes due after = %+v, want none once seen closed", due)
	}
}
