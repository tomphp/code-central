package core_test

import (
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// The Pull Request Watcher's reads and reports (code-central-388).

// pullRequestAccepted turns Pull Request on and makes acme-2 Accepted and
// pushed as its branch.
func (f fixture) pullRequestAccepted(t *testing.T) {
	t.Helper()
	f.accepted(t)
	if _, err := f.core.ReportPushRemote(ctx, "acme", core.ReportPushRemoteInput{URL: "git@github.com:acme/web.git", Base: "main"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingPullRequest}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.ReportPushedCommit(ctx, "acme", core.ReportPushedCommitInput{Ticket: "2", Commit: "abc123", Branch: "acme-2"}); err != nil {
		t.Fatal(err)
	}
}

func (f fixture) watch(t *testing.T) core.PullRequestWatch {
	t.Helper()
	w, err := f.core.GetPullRequestWatch(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func (f fixture) reportPR(t *testing.T, in core.ReportPullRequestInput) {
	t.Helper()
	if in.Link == "" {
		in.Link = "https://github.com/acme/web/pull/7"
	}
	if in.Number == 0 {
		in.Number = 7
	}
	if _, err := f.core.ReportPullRequest(ctx, "acme", in); err != nil {
		t.Fatalf("report_pull_request: %v", err)
	}
}

func TestThePullRequestWatchListsAnAcceptedTicketsBranchWithWhatToOpenItWith(t *testing.T) {
	f := newFixture(t)
	f.pullRequestAccepted(t)
	w := f.watch(t)
	if !w.On || w.CodeHost != "github" || w.Repository != "acme/web" || w.Base != "main" || w.PATEnv != "ADO_PAT" {
		t.Fatalf("watch = %+v", w)
	}
	if len(w.Work) != 1 {
		t.Fatalf("work = %+v, want the one branch", w.Work)
	}
	work := w.Work[0]
	if work.Branch != "acme-2" || work.Commit != "abc123" || work.Pushed.IsZero() || work.PullRequest != nil || work.Title == "" {
		t.Errorf("work = %+v", work)
	}
	if !strings.Contains(work.Body, "Done on acme-2.") || !strings.Contains(work.Body, "acme-2") {
		t.Errorf("body = %q, want the finish summary and the Ticket ID", work.Body)
	}
}

func TestThePullRequestWatchIsEmptyUnderAnotherLandingMethod(t *testing.T) {
	f := newFixture(t)
	f.accepted(t)
	if w := f.watch(t); w.On || len(w.Work) != 0 {
		t.Fatalf("watch = %+v, want off and empty", w)
	}
}

func TestReportingAPullRequestRecordsItOnTheTicketAndItsRowsAndReadsBack(t *testing.T) {
	f := newFixture(t)
	f.pullRequestAccepted(t)
	f.reportPR(t, core.ReportPullRequestInput{Branch: "acme-2", State: store.PROpen})
	v := f.get(t, "2")
	if v.PullRequest == nil || v.PullRequest.Number != 7 || v.PullRequest.State != store.PROpen || v.PullRequest.Link == "" {
		t.Fatalf("pull request = %+v", v.PullRequest)
	}
	rows, err := f.core.FindTickets(ctx, "acme", core.FindTicketsInput{})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.ID == "acme-2" && (r.PullRequest == nil || r.PullRequest.State != store.PROpen) {
			t.Errorf("row = %+v, want the pull request", r)
		}
	}
	if w := f.watch(t); w.Work[0].PullRequest == nil || w.Work[0].PullRequest.Number != 7 {
		t.Errorf("work = %+v, want its current pull request", w.Work)
	}
}

func TestAMergedReportStoresTheMergeCommitAndOnlyAMergedReportMay(t *testing.T) {
	f := newFixture(t)
	f.pullRequestAccepted(t)
	f.reportPR(t, core.ReportPullRequestInput{Branch: "acme-2", State: store.PRMerged, MergeCommit: "def456"})
	if pr := f.get(t, "2").PullRequest; pr == nil || pr.State != store.PRMerged || pr.MergeCommit != "def456" {
		t.Fatalf("pull request = %+v", pr)
	}
	for _, in := range []core.ReportPullRequestInput{
		{Branch: "acme-2", Number: 7, Link: "l", State: store.PRMerged},
		{Branch: "acme-2", Number: 7, Link: "l", State: store.PROpen, MergeCommit: "def456"},
		{Branch: "acme-2", Number: 7, Link: "l", State: "weird"},
		{Branch: "", Number: 7, Link: "l", State: store.PROpen},
	} {
		if _, err := f.core.ReportPullRequest(ctx, "acme", in); err == nil {
			t.Errorf("%+v accepted, want Invalid", in)
		}
	}
}

func TestAClosedPullRequestMayBeReportedOpenAgain(t *testing.T) {
	f := newFixture(t)
	f.pullRequestAccepted(t)
	f.reportPR(t, core.ReportPullRequestInput{Branch: "acme-2", State: store.PRClosed})
	f.reportPR(t, core.ReportPullRequestInput{Branch: "acme-2", State: store.PROpen})
	f.reportPR(t, core.ReportPullRequestInput{Branch: "acme-2", State: store.PRConflicting})
	if pr := f.get(t, "2").PullRequest; pr == nil || pr.State != store.PRConflicting {
		t.Fatalf("pull request = %+v, want conflicting", pr)
	}
}

func TestReportingAPullRequestForABranchNoTicketPushedIsNotFound(t *testing.T) {
	f := newFixture(t)
	f.pullRequestAccepted(t)
	_, err := f.core.ReportPullRequest(ctx, "acme", core.ReportPullRequestInput{Branch: "nope", Number: 1, Link: "l", State: store.PROpen})
	if e, ok := err.(*core.Error); !ok || e.Code != core.NotFound {
		t.Fatalf("err = %#v, want NotFound", err)
	}
}

func TestPushedWorkStaysOnTheWatchWhileInReviewAfterItsPullRequestOpenedAndLeavesOnceDone(t *testing.T) {
	f := newFixture(t)
	f.pullRequestAccepted(t)
	f.move(t, "2", store.InReview)
	if w := f.watch(t); len(w.Work) != 0 {
		t.Fatalf("work = %+v, want none: no pull request was opened", w.Work)
	}
	f.move(t, "2", store.Accepted)
	f.reportPR(t, core.ReportPullRequestInput{Branch: "acme-2", State: store.PROpen})
	f.move(t, "2", store.InReview)
	if w := f.watch(t); len(w.Work) != 1 {
		t.Fatalf("work = %+v, want the branch kept while In Review", w.Work)
	}
	f.move(t, "2", store.Accepted)
	if _, err := f.core.Land(ctx, "acme", core.LandInput{Ticket: "2", Commit: "def456"}); err != nil {
		t.Fatal(err)
	}
	if w := f.watch(t); len(w.Work) != 0 {
		t.Fatalf("work = %+v, want none once Done", w.Work)
	}
}

func TestAPushReportCarriesItsBranchAndIsRecordedUnderPullRequestWithoutCITracking(t *testing.T) {
	f := newFixture(t)
	f.pullRequestAccepted(t)
	v := f.get(t, "2")
	if v.PushedCommit != nil && v.CIResult != "" {
		t.Errorf("ci result = %q, want none without CI Tracking", v.CIResult)
	}
	if w := f.watch(t); len(w.Work) != 1 || w.Work[0].Branch != "acme-2" {
		t.Fatalf("work = %+v, want the pushed branch", w.Work)
	}
}
