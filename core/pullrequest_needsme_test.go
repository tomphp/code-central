package core_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
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
		url := p.CI.PushRemoteURL
		p.CI = trackingOn("acme/web")
		p.CI.PushRemoteURL = url
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

	wantReason(t, f.get(t, "2"), api.ReasonToMerge)
	row, ok := longRow(t, f, "acme-2")
	if !ok || row.Reason != api.ReasonToMerge || row.Branch != "acme-2" || row.PullRequest != nil {
		t.Fatalf("row %+v, want to_merge on acme-2 before any pull request", row)
	}

	f.reportPR(t, api.ReportPullRequestInput{Branch: "acme-2", State: domain.PRConflicting})
	row, _ = longRow(t, f, "acme-2")
	if row.Reason != api.ReasonToMerge || row.PullRequest == nil || row.PullRequest.State != domain.PRConflicting ||
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
	f.reportPR(t, api.ReportPullRequestInput{Branch: "acme-2", State: domain.PROpen})
	wantReason(t, f.get(t, "2"), api.ReasonToMerge)

	f.clock.Advance(time.Hour)
	f.reportPR(t, api.ReportPullRequestInput{Branch: "acme-2", State: domain.PRClosed})
	wantReason(t, f.get(t, "2"), api.ReasonPullRequestClosed)
	row, _ := longRow(t, f, "acme-2")
	if row.PullRequest == nil || row.PullRequest.State != domain.PRClosed {
		t.Fatalf("row %+v, want the closed pull request's link", row)
	}

	f.reportPR(t, api.ReportPullRequestInput{Branch: "acme-2", State: domain.PROpen})
	wantReason(t, f.get(t, "2"), api.ReasonToMerge)
}

func TestPullRequestClosedOffersSendBackAndCancel(t *testing.T) {
	f := newFixture(t)
	f.pullRequestAccepted(t)
	f.reportPR(t, api.ReportPullRequestInput{Branch: "acme-2", State: domain.PRClosed})
	wantReason(t, f.get(t, "2"), api.ReasonPullRequestClosed)

	if _, err := f.core.SendBack(human, "acme", api.SendBackInput{Ticket: "2"}); err == nil {
		t.Fatal("send_back with no note succeeded, want a note required")
	}
	if _, err := f.core.SendBack(human, "acme", api.SendBackInput{Ticket: "2", Note: "reviewers closed it; redo the form"}); err != nil {
		t.Fatalf("send_back: %v", err)
	}
	if v := f.get(t, "2"); v.State != domain.Ready {
		t.Fatalf("state %s, want Ready", v.State)
	}
}

func TestPullRequestClosedOffersCancel(t *testing.T) {
	f := newFixture(t)
	f.pullRequestAccepted(t)
	f.reportPR(t, api.ReportPullRequestInput{Branch: "acme-2", State: domain.PRClosed})
	if _, err := f.core.Cancel(human, "acme", api.CancelInput{Ticket: "acme-2"}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if v := f.get(t, "2"); v.State != domain.Cancelled {
		t.Fatalf("state %s, want Cancelled", v.State)
	}
}

func TestAMergedPullRequestIsNotPullRequestClosed(t *testing.T) {
	f := newFixture(t)
	f.pullRequestAccepted(t)
	f.reportPR(t, api.ReportPullRequestInput{Branch: "acme-2", State: domain.PRMerged, MergeCommit: "def456"})
	wantReason(t, f.get(t, "2"), api.ReasonToMerge)
}

func TestCIFailedUnderPullRequestNeverOffersLandAnyway(t *testing.T) {
	f := newFixture(t)
	f.pullRequestAccepted(t)
	f.withCITracking(t)
	f.reportRun(t, "acme-2", domain.CICompleted, domain.CIFailure)

	v := f.getTicket(t, "acme-2")
	wantReason(t, f.get(t, "2"), api.ReasonCIFailed)
	if want := []api.CIFailureOption{"rerun_ci", "send_back", "new_fix_ticket"}; !slices.Equal(v.CIFailureOptions, want) {
		t.Fatalf("options %v, want %v", v.CIFailureOptions, want)
	}
	_, err := f.core.LandAnyway(human, "acme", api.CIResponseInput{Ticket: "acme-2"})
	var e *api.Error
	if !errors.As(err, &e) || e.Code != api.ActionNotAllowed || !strings.Contains(e.Message, "Pull Request") {
		t.Fatalf("land_anyway: %v, want it refused naming Pull Request", err)
	}
}

func TestCICancelledUnderPullRequestOffersRerunAndSendBack(t *testing.T) {
	f := newFixture(t)
	f.pullRequestAccepted(t)
	f.withCITracking(t)
	f.reportRun(t, "acme-2", domain.CICompleted, domain.CICancelled)

	wantReason(t, f.get(t, "2"), api.ReasonCICancelled)
	if want := []api.CIFailureOption{"rerun_ci", "send_back"}; !slices.Equal(f.getTicket(t, "acme-2").CIFailureOptions, want) {
		t.Fatalf("options %v, want %v", f.getTicket(t, "acme-2").CIFailureOptions, want)
	}
}

func TestCIUnavailableNeverAppliesUnderPullRequest(t *testing.T) {
	f := newFixture(t)
	f.pullRequestAccepted(t)
	f.withCITracking(t)
	f.clock.Advance(core.CIUnavailableAfter + time.Minute)
	wantReason(t, f.get(t, "2"), api.ReasonToMerge)
}

// pullRequestWork turns Pull Request on and makes Ticket n Accepted and
// pushed as commit abc123.
func (f fixture) pullRequestWork(t *testing.T, n int) {
	t.Helper()
	if _, err := f.core.ReportPushRemote(ctx, "acme", api.ReportPushRemoteInput{URL: "git@github.com:acme/web.git", Base: "main"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.SetLandingMethod(human, "acme", api.SetLandingMethodInput{Method: domain.LandingPullRequest}); err != nil {
		t.Fatal(err)
	}
	err := f.store.Update(func(w store.Writer) error {
		tk, _ := w.Ticket("acme", n)
		tk.State = domain.Accepted
		tk.PushedCommits = []domain.PushedCommit{{Commit: "abc123", Branch: tk.Branch, Pushed: f.clock.Now()}}
		return w.PutTicket("acme", tk)
	})
	if err != nil {
		t.Fatal(err)
	}
}
