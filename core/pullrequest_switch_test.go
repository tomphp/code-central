package core_test

import (
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Switching away from Pull Request (code-central-396): each Accepted Ticket
// with an open pull request gets a comment, and the Landing Method section
// notes the pull requests left open, with links.

func (f fixture) switchTo(t *testing.T, m store.LandingMethod) core.ProjectView {
	t.Helper()
	v, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: m, Remote: "origin"})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func (f fixture) openPullRequestsRead(t *testing.T) []core.OpenPullRequest {
	t.Helper()
	p, err := f.core.GetProject(human, "acme")
	if err != nil {
		t.Fatal(err)
	}
	return p.Landing.OpenPullRequests
}

func TestSwitchingAwayFromPullRequestCommentsOnEachAcceptedTicketWithAnOpenOne(t *testing.T) {
	f := newFixture(t)
	f.pullRequestAccepted(t)
	f.reportPR(t, core.ReportPullRequestInput{Branch: "acme-2", State: store.PROpen})

	v := f.switchTo(t, store.LandingMerge)
	want := []core.OpenPullRequest{{Ticket: "acme-2", Link: "https://github.com/acme/web/pull/7"}}
	if got := v.Landing.OpenPullRequests; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("notice = %+v, want %+v", got, want)
	}
	if got := f.openPullRequestsRead(t); len(got) != 1 || got[0] != want[0] {
		t.Fatalf("read back %+v, want %+v", got, want)
	}
	tk := f.get(t, "2")
	last := tk.Comments[len(tk.Comments)-1]
	if !strings.Contains(last.Body, "https://github.com/acme/web/pull/7") || !strings.Contains(last.Body, "Close it") {
		t.Fatalf("comment %q, want it to name the pull request left open", last.Body)
	}
	n := len(tk.Comments)
	f.switchTo(t, store.LandingMerge)
	if got := len(f.get(t, "2").Comments); got != n {
		t.Fatalf("a repeated switch commented again: %d comments, want %d", got, n)
	}
}

func TestSwitchingAwayLeavesNoNoticeForAClosedPullRequest(t *testing.T) {
	for _, state := range []store.PullRequestState{store.PRClosed} {
		f := newFixture(t)
		f.pullRequestAccepted(t)
		in := core.ReportPullRequestInput{Branch: "acme-2", State: state}
		f.reportPR(t, in)
		before := len(f.get(t, "2").Comments)
		v := f.switchTo(t, store.LandingBranch)
		if len(v.Landing.OpenPullRequests) != 0 {
			t.Errorf("%s: notice = %+v, want none", state, v.Landing.OpenPullRequests)
		}
		if n := len(f.get(t, "2").Comments); n != before {
			t.Errorf("%s: %d comments, want %d", state, n, before)
		}
	}
}

func TestTheNoticeIsNotShownUnderPullRequestOrOnceTheTicketLeavesAccepted(t *testing.T) {
	f := newFixture(t)
	f.pullRequestAccepted(t)
	f.reportPR(t, core.ReportPullRequestInput{Branch: "acme-2", State: store.PRConflicting})
	if got := f.openPullRequestsRead(t); len(got) != 0 {
		t.Fatalf("under Pull Request: notice %+v, want none", got)
	}
	f.switchTo(t, store.LandingBranch)
	if got := f.openPullRequestsRead(t); len(got) != 1 {
		t.Fatalf("after the switch: notice %+v, want the conflicting pull request", got)
	}
	f.switchTo(t, store.LandingPullRequest)
	if got := f.openPullRequestsRead(t); len(got) != 0 {
		t.Fatalf("back under Pull Request: notice %+v, want none", got)
	}
}
