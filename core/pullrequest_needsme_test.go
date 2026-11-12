package core_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Needs Me under Pull Request (code-central-391): To merge links the pull
// request, Pull request closed is new, the CI failure options never offer
// Land anyway, and there is no CI gate.

// withCITracking turns CI Tracking on beside Pull Request.
func (f fixture) withCITracking(t *testing.T) {
	t.Helper()
	err := f.store.Update(func(w store.Writer) error {
		p, _ := w.Project("acme")
		p.CI = store.CI{Tracking: true, Provider: store.CIGitHubActions, Repository: "acme/web", PushRemoteURL: p.CI.PushRemoteURL}
		return w.PutProject(p)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAnAcceptedTicketUnderPullRequestIsToMergeLinkingItsPullRequest(t *testing.T) {
	f := newFixture(t)
	f.pullRequestAccepted(t)
	f.clock.Advance(core.NotLandedAfter + time.Hour)

	wantReason(t, f.get(t, "2"), core.ReasonToMerge)
	row, ok := longRow(t, f, "acme-2")
	if !ok || row.Reason != core.ReasonToMerge || row.Branch != "acme-2" || row.PullRequest != nil {
		t.Fatalf("row %+v, want to_merge on acme-2 before any pull request", row)
	}

	f.reportPR(t, core.ReportPullRequestInput{Branch: "acme-2", State: store.PRConflicting})
	row, _ = longRow(t, f, "acme-2")
	if row.Reason != core.ReasonToMerge || row.PullRequest == nil || row.PullRequest.State != store.PRConflicting ||
		row.PullRequest.Link != "https://github.com/acme/web/pull/7" {
		t.Fatalf("row %+v, want to_merge linking the conflicting pull request", row)
	}
}

func TestTheWatchNeverRaisesAcceptedNotLandedUnderPullRequest(t *testing.T) {
	f := newFixture(t)
	f.pullRequestAccepted(t)
	tick := watch(t, f)
	tick()
	f.clock.Advance(core.NotLandedAfter + time.Second)
	if got := tick(); len(got) != 0 {
		t.Fatalf("got %v, want none", got)
	}
}

func TestAClosedPullRequestIsPullRequestClosedUntilItIsReopened(t *testing.T) {
	f := newFixture(t)
	f.pullRequestAccepted(t)
	f.reportPR(t, core.ReportPullRequestInput{Branch: "acme-2", State: store.PROpen})
	wantReason(t, f.get(t, "2"), core.ReasonToMerge)

	f.clock.Advance(time.Hour)
	f.reportPR(t, core.ReportPullRequestInput{Branch: "acme-2", State: store.PRClosed})
	wantReason(t, f.get(t, "2"), core.ReasonPullRequestClosed)
	row, _ := longRow(t, f, "acme-2")
	if row.PullRequest == nil || row.PullRequest.State != store.PRClosed {
		t.Fatalf("row %+v, want the closed pull request's link", row)
	}

	f.reportPR(t, core.ReportPullRequestInput{Branch: "acme-2", State: store.PROpen})
	wantReason(t, f.get(t, "2"), core.ReasonToMerge)
}

func TestPullRequestClosedOffersSendBackAndCancel(t *testing.T) {
	f := newFixture(t)
	f.pullRequestAccepted(t)
	f.reportPR(t, core.ReportPullRequestInput{Branch: "acme-2", State: store.PRClosed})
	wantReason(t, f.get(t, "2"), core.ReasonPullRequestClosed)

	if _, err := f.core.SendBack(human, "acme", core.SendBackInput{Ticket: "2"}); err == nil {
		t.Fatal("send_back with no note succeeded, want a note required")
	}
	if _, err := f.core.SendBack(human, "acme", core.SendBackInput{Ticket: "2", Note: "reviewers closed it; redo the form"}); err != nil {
		t.Fatalf("send_back: %v", err)
	}
	if v := f.get(t, "2"); v.State != store.Ready {
		t.Fatalf("state %s, want Ready", v.State)
	}
}

func TestPullRequestClosedOffersCancel(t *testing.T) {
	f := newFixture(t)
	f.pullRequestAccepted(t)
	f.reportPR(t, core.ReportPullRequestInput{Branch: "acme-2", State: store.PRClosed})
	if _, err := f.core.Cancel(human, "acme", core.CancelInput{Ticket: "acme-2"}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if v := f.get(t, "2"); v.State != store.Cancelled {
		t.Fatalf("state %s, want Cancelled", v.State)
	}
}

func TestAMergedPullRequestIsNotPullRequestClosed(t *testing.T) {
	f := newFixture(t)
	f.pullRequestAccepted(t)
	f.reportPR(t, core.ReportPullRequestInput{Branch: "acme-2", State: store.PRMerged, MergeCommit: "def456"})
	wantReason(t, f.get(t, "2"), core.ReasonToMerge)
}

func TestCIFailedUnderPullRequestNeverOffersLandAnyway(t *testing.T) {
	f := newFixture(t)
	f.pullRequestAccepted(t)
	f.withCITracking(t)
	f.reportRun(t, "acme-2", store.CICompleted, store.CIFailure)

	v := f.getTicket(t, "acme-2")
	wantReason(t, f.get(t, "2"), core.ReasonCIFailed)
	if want := []string{"rerun_ci", "send_back", "new_fix_ticket"}; !slices.Equal(v.CIFailureOptions, want) {
		t.Fatalf("options %v, want %v", v.CIFailureOptions, want)
	}
	_, err := f.core.LandAnyway(human, "acme", core.TicketInput{Ticket: "acme-2"})
	var e *core.Error
	if !errors.As(err, &e) || e.Code != core.ActionNotAllowed || !strings.Contains(e.Message, "Pull Request") {
		t.Fatalf("land_anyway: %v, want it refused naming Pull Request", err)
	}
}

func TestCICancelledUnderPullRequestOffersRerunAndSendBack(t *testing.T) {
	f := newFixture(t)
	f.pullRequestAccepted(t)
	f.withCITracking(t)
	f.reportRun(t, "acme-2", store.CICompleted, store.CICancelled)

	wantReason(t, f.get(t, "2"), core.ReasonCICancelled)
	if want := []string{"rerun_ci", "send_back"}; !slices.Equal(f.getTicket(t, "acme-2").CIFailureOptions, want) {
		t.Fatalf("options %v, want %v", f.getTicket(t, "acme-2").CIFailureOptions, want)
	}
}

func TestCIUnavailableNeverAppliesUnderPullRequest(t *testing.T) {
	f := newFixture(t)
	f.pullRequestAccepted(t)
	f.withCITracking(t)
	f.clock.Advance(core.CIUnavailableAfter + time.Minute)
	wantReason(t, f.get(t, "2"), core.ReasonToMerge)
}

// pullRequestWork turns Pull Request on and makes Ticket n Accepted and
// pushed as commit abc123.
func (f fixture) pullRequestWork(t *testing.T, n int) {
	t.Helper()
	if _, err := f.core.ReportPushRemote(ctx, "acme", core.ReportPushRemoteInput{URL: "git@github.com:acme/web.git", Base: "main"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingPullRequest}); err != nil {
		t.Fatal(err)
	}
	err := f.store.Update(func(w store.Writer) error {
		tk, _ := w.Ticket("acme", n)
		tk.State = store.Accepted
		tk.PushedCommits = []store.PushedCommit{{Commit: "abc123", Branch: tk.Branch, Pushed: f.clock.Now()}}
		return w.PutTicket("acme", tk)
	})
	if err != nil {
		t.Fatal(err)
	}
}
