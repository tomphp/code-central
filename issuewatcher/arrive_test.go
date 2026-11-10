package issuewatcher_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/internal/fakegh"
	"github.com/tomoram/code-central/store"
)

// issue is an item in GitHub's list of a repository's issues; a pull
// request is one too, marked as such.
type issue struct {
	Number      int     `json:"number"`
	Title       string  `json:"title"`
	Body        *string `json:"body"`
	HTMLURL     string  `json:"html_url"`
	State       string  `json:"state"`
	Labels      []label `json:"labels"`
	PullRequest *struct {
		URL string `json:"url"`
	} `json:"pull_request,omitempty"`
}

// label is a label on an issue.
type label struct {
	Name string `json:"name"`
}

// openIssue is acme/web's open issue n, with a body.
func openIssue(n int) issue {
	body := fmt.Sprintf("Body %d", n)
	return issue{Number: n, Title: fmt.Sprintf("Issue %d", n), Body: &body, HTMLURL: fmt.Sprintf("https://github.com/acme/web/issues/%d", n), State: "open"}
}

// pullRequest is acme/web's open pull request n, as the issues list
// gives it.
func pullRequest(n int) issue {
	i := openIssue(n)
	i.Title = fmt.Sprintf("Pull request %d", n)
	i.HTMLURL = fmt.Sprintf("https://github.com/acme/web/pull/%d", n)
	i.PullRequest = &struct {
		URL string `json:"url"`
	}{URL: fmt.Sprintf("https://api.github.com/repos/acme/web/pulls/%d", n)}
	return i
}

// issuesPath is the gh api path of page of acme/web's open issues,
// oldest first, those with label if it isn't empty, 100 to a page.
func issuesPath(label string, page int) string {
	q := url.Values{"state": {"open"}, "sort": {"created"}, "direction": {"asc"}, "per_page": {"100"}, "page": {strconv.Itoa(page)}}
	if label != "" {
		q.Set("labels", label)
	}
	return "repos/acme/web/issues?" + q.Encode()
}

// openIssues scripts GitHub's answer to listing page of acme/web's open
// issues with label.
func (w *watching) openIssues(label string, page int, issues ...issue) {
	w.t.Helper()
	b, err := json.Marshal(append([]issue{}, issues...))
	if err != nil {
		w.t.Fatal(err)
	}
	w.h.Answer(fakegh.Answer{Args: []string{"api", issuesPath(label, page)}, Stdout: string(b)})
}

// watch is acme's get_issue_watch.
func (w *watching) watch() core.IssueWatch {
	w.t.Helper()
	watch, err := w.srv.Core.GetIssueWatch(context.Background(), "acme")
	if err != nil {
		w.t.Fatal(err)
	}
	return watch
}

// incoming is acme's Incoming Issues as get_issue_watch lists them, each
// as its Link's ID, URL, title and description.
func (w *watching) incoming() []string {
	w.t.Helper()
	var out []string
	for _, i := range w.watch().Issues {
		if i.Link.Integration != "github" {
			w.t.Fatalf("Incoming Issue %s linked by %q, want github", i.Ticket, i.Link.Integration)
		}
		out = append(out, fmt.Sprintf("%s %s %s: %s", i.Link.ID, i.Link.URL, i.Title, i.Description))
	}
	return out
}

// linked is every Spec and Ticket in acme linked by github, detached too.
func (w *watching) linked() []core.LinkedRow {
	w.t.Helper()
	rows, err := w.srv.Core.FindLinked(context.Background(), "acme", core.FindLinkedInput{Integration: "github"})
	if err != nil {
		w.t.Fatal(err)
	}
	return rows
}

// polled waits out the interval, then waits for the poll it starts to
// end.
func (w *watching) polled() {
	w.t.Helper()
	w.nextPoll()
	w.srv.Clock.BlockUntil(1)
}

// stop closes the Issue Watcher's stdin and waits for it to exit, as it
// should, with no error.
func (w *watching) stop() {
	w.t.Helper()
	w.stdin.Close()
	select {
	case err := <-w.done:
		if err != nil {
			w.t.Fatalf("Watch = %v, want nil on its stdin closing", err)
		}
	case <-time.After(10 * time.Second):
		w.t.Fatal("the Issue Watcher kept running after its stdin closed")
	}
}

// issueLine is openIssue(n) as incoming lists it.
func issueLine(n int) string {
	return fmt.Sprintf("acme/web#%d https://github.com/acme/web/issues/%d Issue %d: Body %d", n, n, n, n)
}

// wantIncoming checks acme's Incoming Issues are openIssue's ns, and
// that nothing else is linked: each issue arrived once.
func wantIncoming(t *testing.T, w *watching, ns ...int) {
	t.Helper()
	var want []string
	for _, n := range ns {
		want = append(want, issueLine(n))
	}
	if got := w.incoming(); !slices.Equal(got, want) {
		t.Fatalf("Incoming Issues %q, want %q", got, want)
	}
	if got := len(w.linked()); got != len(ns) {
		t.Fatalf("%d linked items, want %d: one per issue", got, len(ns))
	}
}

// Every issue already open when GitHub Issues is turned on arrives, as a
// Draft Ticket on the Issues Spec linked to it; pull requests don't.
func TestOpenIssuesArriveAsDraftIncomingIssuesButPullRequestsDont(t *testing.T) {
	w := newWatching(t)
	none := issue{Number: 4, Title: "Issue 4", HTMLURL: "https://github.com/acme/web/issues/4"}
	w.openIssues("", 1, openIssue(1), pullRequest(2), openIssue(3), none)
	w.start(w.h.Bin)
	w.status(core.WatcherWatching)

	want := []string{issueLine(1), issueLine(3), "acme/web#4 https://github.com/acme/web/issues/4 Issue 4: "}
	if got := w.incoming(); !slices.Equal(got, want) {
		t.Fatalf("Incoming Issues %q, want %q", got, want)
	}
	watch, err := w.srv.Core.GetIssueWatch(context.Background(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range watch.Issues {
		got, err := w.srv.Core.GetTicket(context.Background(), "acme", core.GetTicketInput{Ticket: core.Ref(i.Ticket)})
		if err != nil {
			t.Fatal(err)
		}
		if got.Link == nil || got.State != store.Draft || got.Spec.ID != watch.Spec || got.Link.ExternalState != store.ExternalOpen {
			t.Fatalf("%s is %s on %s, %s, want Draft on the Issues Spec %s, open", i.Ticket, got.State, got.Spec.ID, got.Link.ExternalState, watch.Spec)
		}
	}

	// An issue opened later arrives on the next poll.
	w.openIssues("", 1, openIssue(1), pullRequest(2), openIssue(3), none, openIssue(5))
	w.polled()
	if got := w.incoming(); len(got) != 4 || got[3] != issueLine(5) {
		t.Fatalf("Incoming Issues %q, want issue 5 added", got)
	}
}

// GitHub lists 100 issues a page; every page is read.
func TestEveryPageOfOpenIssuesArrives(t *testing.T) {
	w := newWatching(t)
	var first []issue
	var want []int
	for n := 1; n <= 100; n++ {
		first = append(first, openIssue(n))
		want = append(want, n)
	}
	w.openIssues("", 1, first...)
	w.openIssues("", 2, openIssue(101))
	w.start(w.h.Bin)
	w.status(core.WatcherWatching)
	wantIncoming(t, w, append(want, 101)...)
}

// With a filter label, only the issues GitHub lists with it arrive.
func TestWithAFilterLabelOnlyLabelledIssuesArrive(t *testing.T) {
	w := newWatching(t)
	if _, err := w.srv.Core.SetGitHubIssues(human, "acme", core.SetGitHubIssuesInput{On: true, Repository: "acme/web", Label: "good first issue"}); err != nil {
		t.Fatal(err)
	}
	w.openIssues("", 1, openIssue(1), openIssue(2))
	w.openIssues("good first issue", 1, openIssue(2))
	w.start(w.h.Bin)
	w.status(core.WatcherWatching)
	wantIncoming(t, w, 2)
}

// Each issue arrives once: a restarted Issue Watcher keeps no state, and
// finds the issues it already brought in by their Links.
func TestARestartedIssueWatcherCreatesNoDuplicates(t *testing.T) {
	w := newWatching(t)
	w.openIssues("", 1, openIssue(1), openIssue(2))
	w.start(w.h.Bin)
	w.status(core.WatcherWatching)
	w.polled()
	wantIncoming(t, w, 1, 2)

	w.stop()
	w.openIssues("", 1, openIssue(1), openIssue(2), openIssue(3))
	w.start(w.h.Bin)
	// The stopped loop's timer is still waiting too.
	w.srv.Clock.BlockUntil(2)
	wantIncoming(t, w, 1, 2, 3)
}

// Turning GitHub Issues off and on again picks up where it left off.
func TestTurningGitHubIssuesOffAndOnCreatesNoDuplicates(t *testing.T) {
	w := newWatching(t)
	w.openIssues("", 1, openIssue(1))
	w.start(w.h.Bin)
	w.status(core.WatcherWatching)
	for _, on := range []bool{false, true} {
		if _, err := w.srv.Core.SetGitHubIssues(human, "acme", core.SetGitHubIssuesInput{On: on, Repository: "acme/web"}); err != nil {
			t.Fatal(err)
		}
		w.polled()
	}
	w.status(core.WatcherWatching)
	wantIncoming(t, w, 1)
}

// An unlinked Incoming Issue keeps its detached Link, which stops its
// issue coming in again.
func TestADetachedLinkStopsAnIssueComingInAgain(t *testing.T) {
	w := newWatching(t)
	w.openIssues("", 1, openIssue(1))
	w.start(w.h.Bin)
	w.status(core.WatcherWatching)
	rows := w.linked()
	if len(rows) != 1 {
		t.Fatalf("linked %v, want issue 1's Ticket", rows)
	}
	if _, err := w.srv.Core.Unlink(human, "acme", core.UnlinkInput{Target: core.Ref(rows[0].ID)}); err != nil {
		t.Fatal(err)
	}
	w.polled()
	if got := w.incoming(); len(got) != 0 {
		t.Fatalf("Incoming Issues %q, want none once unlinked", got)
	}
	if got := w.linked(); len(got) != 1 || !got[0].Link.Detached {
		t.Fatalf("linked %v, want only the detached Ticket", got)
	}
}

// GitHub rate-limiting the issue list is like rate-limiting the check:
// the Issue Watcher is still Watching, until GitHub's reset time, and
// brings the issues in once it may call again.
func TestARateLimitedIssueListWaitsForTheResetTime(t *testing.T) {
	w := newWatching(t)
	reset := w.srv.Clock.Now().Add(15 * time.Minute)
	w.rateLimitedOn(issuesPath("", 1), "X-Ratelimit-Remaining: 0", fmt.Sprintf("X-Ratelimit-Reset: %d", reset.Unix()))
	w.start(w.h.Bin)
	w.rateLimitedUntil(reset)

	w.openIssues("", 1, openIssue(1))
	w.srv.Clock.BlockUntil(1)
	w.srv.Clock.Advance(15 * time.Minute)
	w.rateLimitedUntil(time.Time{})
	wantIncoming(t, w, 1)
}
