package issuewatcher_test

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/internal/fakegh"
	"github.com/tomoram/code-central/store"
)

// readIssue is acme/web's issue n as GitHub gives it when it is read on
// its own: in state, open or closed, with labels.
func (w *watching) readIssue(n int, state string, labels ...string) {
	w.t.Helper()
	i := openIssue(n)
	ls := []map[string]string{}
	for _, l := range labels {
		ls = append(ls, map[string]string{"name": l})
	}
	b, err := json.Marshal(map[string]any{
		"number": n, "state": state, "title": i.Title, "body": i.Body, "html_url": i.HTMLURL,
		"repository_url": "https://api.github.com/repos/acme/web", "labels": ls,
	})
	if err != nil {
		w.t.Fatal(err)
	}
	w.h.Answer(fakegh.Answer{Args: []string{"api", fakegh.IssuePath("acme/web", n)}, Stdout: string(b)})
}

// arrived starts the Issue Watcher with openIssue's ns open, and waits
// for them to arrive.
func (w *watching) arrived(label string, ns ...int) {
	w.t.Helper()
	var issues []issue
	for _, n := range ns {
		issues = append(issues, openIssue(n))
	}
	w.openIssues(label, 1, issues...)
	w.start(w.h.Bin)
	w.status(core.WatcherWatching)
	wantIncoming(w.t, w, ns...)
}

// ticketOf is the Incoming Issue linked to acme/web's issue n.
func (w *watching) ticketOf(n int) core.TicketView {
	w.t.Helper()
	id := fmt.Sprintf("acme/web#%d", n)
	for _, row := range w.linked() {
		if row.Link.ID == id {
			got, err := w.srv.Core.GetTicket(context.Background(), "acme", core.GetTicketInput{Ticket: core.Ref(row.ID), Parts: []string{core.PartEdits}})
			if err != nil {
				w.t.Fatal(err)
			}
			return got
		}
	}
	w.t.Fatalf("no Ticket linked to %s", id)
	return core.TicketView{}
}

// wantExternal checks the Incoming Issue linked to acme/web's issue n is
// still Draft, linked and not detached, with external state want.
func wantExternal(t *testing.T, w *watching, n int, want store.ExternalState) {
	t.Helper()
	got := w.ticketOf(n)
	if got.State != store.Draft || got.Link == nil || got.Link.Detached || got.Link.ExternalState != want {
		t.Fatalf("issue %d's Ticket is %s with Link %+v, want Draft, linked, %s", n, got.State, got.Link, want)
	}
}

// reasons is each of acme's Tickets waiting in Needs Me, by its reason.
func (w *watching) reasons() map[string]core.NeedsMeReason {
	w.t.Helper()
	nm, err := w.srv.Core.NeedsMe(context.Background())
	if err != nil {
		w.t.Fatal(err)
	}
	out := map[string]core.NeedsMeReason{}
	for _, g := range nm.Groups {
		for _, row := range g.Tickets {
			out[row.ID] = g.Reason
		}
	}
	return out
}

// An edit to an Incoming Issue's title or body on GitHub updates it, as
// the integration; an unchanged issue isn't updated.
func TestEditsOnGitHubUpdateAnIncomingIssue(t *testing.T) {
	w := newWatching(t)
	w.arrived("", 1, 2, 3)

	retitled, rewritten := openIssue(1), openIssue(2)
	retitled.Title = "Issue 1, retitled"
	body := "Body 2, rewritten"
	rewritten.Body = &body
	w.openIssues("", 1, retitled, rewritten, openIssue(3))
	w.polled()

	want := []string{
		"acme/web#1 https://github.com/acme/web/issues/1 Issue 1, retitled: Body 1",
		"acme/web#2 https://github.com/acme/web/issues/2 Issue 2: Body 2, rewritten",
		issueLine(3),
	}
	if got := w.incoming(); !slices.Equal(got, want) {
		t.Fatalf("Incoming Issues %q, want %q", got, want)
	}
	for n, field := range map[int]string{1: "title", 2: "description"} {
		edits := w.ticketOf(n).Edits
		if len(edits) != 1 || edits[0].Actor != string(core.Integration) || !slices.Equal(slices.Collect(maps.Keys(edits[0].Previous)), []string{field}) {
			t.Fatalf("issue %d's edits %+v, want one by the integration, of its %s", n, edits, field)
		}
	}
	if edits := w.ticketOf(3).Edits; len(edits) != 0 {
		t.Fatalf("unchanged issue 3's edits %+v, want none", edits)
	}

	// Polling again with nothing changed updates nothing.
	w.polled()
	for n, want := range map[int]int{1: 1, 2: 1, 3: 0} {
		if got := len(w.ticketOf(n).Edits); got != want {
			t.Fatalf("issue %d has %d edits after a poll with nothing changed, want %d", n, got, want)
		}
	}
}

// An issue closed on GitHub before it is triaged shows in Needs Me as
// closed externally; reopened, it is open again.
func TestAClosedIssueIsClosedExternallyUntilItIsReopened(t *testing.T) {
	w := newWatching(t)
	w.arrived("", 1, 2)
	id := w.ticketOf(1).ID

	w.openIssues("", 1, openIssue(2))
	w.readIssue(1, "closed")
	w.polled()
	wantExternal(t, w, 1, store.ExternalClosed)
	wantExternal(t, w, 2, store.ExternalOpen)
	if got := w.reasons(); got[id] != core.ReasonClosedExternally || len(got) != 1 {
		t.Fatalf("Needs Me %v, want only %s, closed externally", got, id)
	}
	// It stays an Incoming Issue, to triage.
	wantIncoming(t, w, 1, 2)

	w.openIssues("", 1, openIssue(1), openIssue(2))
	w.polled()
	wantExternal(t, w, 1, store.ExternalOpen)
	if got := w.reasons(); len(got) != 0 {
		t.Fatalf("Needs Me %v, want nothing once issue 1 is reopened", got)
	}
}

// A deleted issue's Incoming Issue is marked deleted, not deleted.
func TestADeletedIssueIsMarkedDeleted(t *testing.T) {
	w := newWatching(t)
	w.arrived("", 1)
	w.openIssues("", 1)
	w.h.Answer(fakegh.DeletedIssue("acme/web", 1))
	w.polled()
	wantExternal(t, w, 1, store.ExternalDeleted)
	wantIncoming(t, w, 1)
}

// An issue transferred to another repository is out of scope, whether
// gh can follow it there or not, and keeps its text.
func TestATransferredIssueIsOutOfScope(t *testing.T) {
	w := newWatching(t)
	w.arrived("", 1, 2)
	w.openIssues("", 1)
	w.h.Answer(fakegh.TransferredIssue("acme/web", 1, "acme/api", 7))
	w.h.Answer(fakegh.Answer{Args: []string{"api", fakegh.IssuePath("acme/web", 2)}, Exit: 1,
		Stdout: "HTTP/2.0 404 Not Found\r\n\r\n{\"message\":\"Not Found\"}", Stderr: "gh: Not Found (HTTP 404)\n"})
	w.polled()
	wantExternal(t, w, 1, store.ExternalOutOfScope)
	wantExternal(t, w, 2, store.ExternalOutOfScope)
	wantIncoming(t, w, 1, 2)
}

// An issue that loses the filter label is out of scope; given it again,
// it is open again.
func TestAnIssueThatLosesTheFilterLabelIsOutOfScopeUntilItHasItAgain(t *testing.T) {
	w := newWatching(t)
	if _, err := w.srv.Core.SetGitHubIssues(human, "acme", core.SetGitHubIssuesInput{On: true, Repository: "acme/web", Label: "Bug"}); err != nil {
		t.Fatal(err)
	}
	w.arrived("Bug", 1, 2)

	w.openIssues("Bug", 1, openIssue(2))
	w.readIssue(1, "open", "enhancement")
	// GitHub's label names match whatever their case.
	w.readIssue(2, "open", "bug")
	w.polled()
	wantExternal(t, w, 1, store.ExternalOutOfScope)
	wantExternal(t, w, 2, store.ExternalOpen)

	w.openIssues("Bug", 1, openIssue(1), openIssue(2))
	w.polled()
	wantExternal(t, w, 1, store.ExternalOpen)
}

// GitHub rate-limiting an issue's read leaves its Incoming Issue as it
// is, until GitHub's reset time.
func TestARateLimitedIssueReadWaitsForTheResetTime(t *testing.T) {
	w := newWatching(t)
	w.arrived("", 1)
	reset := w.srv.Clock.Now().Add(15 * time.Minute)
	w.openIssues("", 1)
	w.h.Answer(fakegh.Answer{Args: []string{"api", fakegh.IssuePath("acme/web", 1)}, Exit: 1,
		Stdout: fmt.Sprintf("HTTP/2.0 403 Forbidden\r\nX-Ratelimit-Remaining: 0\r\nX-Ratelimit-Reset: %d\r\n\r\n{}", reset.Unix()),
		Stderr: "gh: API rate limit exceeded for user ID 1. (HTTP 403)\n"})
	w.nextPoll()
	w.rateLimitedUntil(reset)
	wantExternal(t, w, 1, store.ExternalOpen)

	w.readIssue(1, "closed")
	w.srv.Clock.BlockUntil(1)
	w.srv.Clock.Advance(15 * time.Minute)
	w.rateLimitedUntil(time.Time{})
	wantExternal(t, w, 1, store.ExternalClosed)
}
