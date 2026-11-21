package factory_test

import (
	"context"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"

	"github.com/tomoram/code-central/factory/factorytest"
)

// pullRequestOn sets the Pull Request Landing Method with the Push remote.
func pullRequestOn(h *factorytest.Harness, remote string) {
	// The harness's remote is a path; Pull Request needs a Code Host's URL.
	if _, err := h.Server.Client.ReportPushRemote(context.Background(), h.Project, api.ReportPushRemoteInput{URL: "https://github.com/acme/app.git", Base: "main"}); err != nil {
		panic(err)
	}
	h.SetLandingMethod(api.SetLandingMethodInput{Method: domain.LandingPullRequest, Remote: remote})
}

// wantPushed checks the Factory has reported ticket's branch as pushed, at
// commit when it isn't "": the Pull Request Watcher would see it in its list.
// The Factory has settled.
func wantPushed(t *testing.T, h *factorytest.Harness, ticket, commit string) {
	t.Helper()
	w, err := h.Server.Client.GetPullRequestWatch(context.Background(), h.Project)
	if err != nil {
		t.Fatal(err)
	}
	for _, work := range w.Work {
		if work.Branch == ticket && (commit == "" || work.Commit == commit) {
			return
		}
	}
	t.Fatalf("the pushed work isn't reported: want %s at %q, the Watcher's list is %+v", ticket, commit, w.Work)
}

// reportPullRequest is the Pull Request Watcher reporting ticket's pull
// request, as merged at commit when commit isn't "".
func reportPullRequest(t *testing.T, h *factorytest.Harness, branch, commit string) {
	t.Helper()
	in := api.ReportPullRequestInput{Branch: branch, Number: 7, Link: "https://example.test/pull/7", State: domain.PROpen}
	if commit != "" {
		in.State, in.MergeCommit = domain.PRMerged, commit
	}
	if _, err := h.Server.Client.ReportPullRequest(context.Background(), h.Project, in); err != nil {
		t.Fatal(err)
	}
}

func TestUnderPullRequestAcceptingPushesTheBranchAndReportsIt(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	remote := h.AddRemote("origin")
	pullRequestOn(h, "origin")
	id, commit := accepted(h, "Add a feature", "feature.txt", "feature\n")

	r := h.Start()
	r.WantOutput(id + ": pushed " + id + " to origin")
	if got := remote.Tip(id); got != commit {
		t.Errorf("the remote's %s is at %q, want %s", id, got, commit)
	}
	wantPushed(t, h, id, commit)
	if tk := h.Ticket(id); tk.State != domain.Accepted {
		t.Errorf("%s is %s, want it waiting Accepted for its pull request", id, tk.State)
	}
}

func TestUnderPullRequestEarlierAcceptedTicketsArePushedOnTheNextPoll(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	remote := h.AddRemote("origin")
	h.SetLandingMethod(api.SetLandingMethodInput{Method: domain.LandingBranch})
	id, commit := accepted(h, "Accepted before the switch", "early.txt", "early\n")
	r := h.Start()
	r.Poll()

	pullRequestOn(h, "origin")
	r.Poll()
	wantPushed(t, h, id, commit)
	if got := remote.Tip(id); got != commit {
		t.Errorf("the remote's %s is at %q, want %s", id, got, commit)
	}
}

func TestUnderPullRequestATicketLandsAtTheMergeCommitOfASquashMergeMadeOnTheRemote(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	remote := h.AddRemote("origin")
	pullRequestOn(h, "origin")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	r := h.Start()
	wantPushed(t, h, id, "")
	reportPullRequest(t, h, id, "")
	r.Poll()
	if tk := h.Ticket(id); tk.State != domain.Accepted || tk.PullRequest == nil {
		t.Fatalf("%s is %s with pull request %+v, want Accepted with an open one", id, tk.State, tk.PullRequest)
	}

	// A squash merge: the host's commit has the work but not its history.
	squash := h.CommitOn("squash", "feature.txt", "feature\n")
	h.PushByHand(remote, "squash", "main")
	reportPullRequest(t, h, id, squash)
	r.Poll()

	tk := h.WantState(id, domain.Done)
	if m := tk.Moves[len(tk.Moves)-1]; m.Commit != squash {
		t.Errorf("landed at %q, want the merge commit %s", m.Commit, squash)
	}
	if got := h.Tip("main"); got != squash {
		t.Errorf("the local base is at %s, want it fast-forwarded to %s", got, squash)
	}
}

func TestUnderPullRequestATicketWaitsWhileTheMergeCommitIsNotFetched(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	h.AddRemote("origin")
	pullRequestOn(h, "origin")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	r := h.Start()
	wantPushed(t, h, id, "")

	// The merge commit exists locally but the remote's base doesn't hold it.
	unfetched := h.CommitOn("unfetched", "feature.txt", "feature\n")
	reportPullRequest(t, h, id, unfetched)
	r.Poll()
	r.WantOutput(id + ": its pull request is merged at")
	if tk := h.Ticket(id); tk.State != domain.Accepted {
		t.Errorf("%s is %s, want it waiting Accepted", id, tk.State)
	}
}

func TestUnderPullRequestAFailedFetchOnlyWarns(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	remote := h.AddRemote("origin")
	pullRequestOn(h, "origin")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	r := h.Start()
	wantPushed(t, h, id, "")
	squash := h.CommitOn("squash", "feature.txt", "feature\n")
	h.PushByHand(remote, "squash", "main")
	reportPullRequest(t, h, id, squash)

	remote.Break()
	r.Poll()
	r.WantOutput("warning: can't fetch origin")
	if _, ok := baseDivergedItem(t, h); ok {
		t.Error("a failed fetch showed Base diverged")
	}
	if tk := h.Ticket(id); tk.State != domain.Accepted {
		t.Errorf("%s is %s, want it left Accepted", id, tk.State)
	}

	remote.Restore()
	r.Poll()
	h.WantState(id, domain.Done)
}

func TestUnderPullRequestADivergedBaseStopsLandingAndClaiming(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.RealGit())
	remote := h.AddRemote("origin")
	pullRequestOn(h, "origin")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	h.PushByHand(remote, "main", "main")
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	h.PushByHand(remote, "theirs", "main")
	h.CommitOn("main", "mine.txt", "mine\n")
	r := h.Start()
	wantPushed(t, h, id, "")
	reportPullRequest(t, h, id, theirs)
	ready := h.ReadyTicket("Waits for the base")
	r.Poll()
	r.WantOutput("has diverged from the remote's")
	if tk := h.Ticket(id); tk.State != domain.Accepted {
		t.Errorf("%s is %s, want it held Accepted", id, tk.State)
	}
	if tk := h.Ticket(ready); tk.State != domain.Ready {
		t.Errorf("%s is %s, want it not claimed", ready, tk.State)
	}
	local := h.Tip("main")
	row, ok := baseDivergedItem(t, h)
	if !ok {
		t.Fatal("Base diverged doesn't show in Needs Me")
	}
	if got := row.BaseDiverged; got.Local != local || got.Remote != theirs {
		t.Errorf("Base diverged shows the local base at %s and the remote's at %s, want %s and %s", got.Local, got.Remote, local, theirs)
	}

	h.Git("reset", "-q", "--hard", "origin/main")
	r.Poll()
	h.WantState(id, domain.Done)
	if _, ok := baseDivergedItem(t, h); ok {
		t.Error("Base diverged still shows in Needs Me after the base was reset to the remote's")
	}
	h.WantCalls(1)
}

// baseDivergedItem is the Project's Base diverged item in Needs Me.
func baseDivergedItem(t *testing.T, h *factorytest.Harness) (api.NeedsMeRow, bool) {
	t.Helper()
	if rows := h.NeedsMeItems(api.ReasonBaseDiverged); len(rows) > 0 {
		return rows[0], true
	}
	return api.NeedsMeRow{}, false
}

func TestUnderPullRequestANewBranchIsCutFromTheFastForwardedBase(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	remote := h.AddRemote("origin")
	pullRequestOn(h, "origin")
	h.PushByHand(remote, "main", "main")
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	h.PushByHand(remote, "theirs", "main")
	ready := h.ReadyTicket("Starts from the host's base")

	h.Start()
	calls := h.WantCalls(1)
	if got := h.Tip("main"); got != theirs {
		t.Errorf("the local base is at %s, want it fast-forwarded to %s", got, theirs)
	}
	if calls[0].Ticket != ready {
		t.Fatalf("calls %+v, want %s claimed", calls, ready)
	}
	// The branch was cut from the host's base, so it holds its commit.
	if !h.IsAncestor(theirs, ready) {
		t.Errorf("%s is not in %s", theirs, ready)
	}
}

func TestUnderPullRequestATicketWithNothingToMergeLandsWithoutAPullRequest(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	h.AddRemote("origin")
	pullRequestOn(h, "origin")
	id := h.ReadyTicket("No change")
	head := h.Tip("main")
	h.Finished(id, "main", head)
	h.Accept(id)

	h.Start()
	tk := h.WantState(id, domain.Done)
	if tk.PullRequest != nil {
		t.Errorf("%s has pull request %+v, want none", id, tk.PullRequest)
	}
}

func TestUnderPullRequestATicketWithoutABranchLands(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	h.AddRemote("origin")
	pullRequestOn(h, "origin")
	id := h.ReadyTicket("Research")
	h.Finished(id, "", "")
	h.Accept(id)

	h.Start()
	h.WantState(id, domain.Done)
}

func TestUnderPullRequestAMergedPullRequestLandsAnInReviewTicketAndWarnsOfOthers(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	remote := h.AddRemote("origin")
	pullRequestOn(h, "origin")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	r := h.Start()
	wantPushed(t, h, id, "")
	reportPullRequest(t, h, id, "")
	if _, err := h.Human().SendBack(context.Background(), h.Project, api.SendBackInput{Ticket: api.Ref(id), Note: "fix"}); err != nil {
		t.Fatal(err)
	}
	squash := h.CommitOn("squash", "feature.txt", "feature\n")
	h.PushByHand(remote, "squash", "main")
	reportPullRequest(t, h, id, squash)
	r.Poll()
	r.WantOutput("warning: " + id + " is")
	if !commented(h.Ticket(id), "pull request 7 was merged") {
		t.Errorf("want a comment on the merge, got %+v", h.Ticket(id).Comments)
	}
}

func TestUnderPullRequestLandingNeedsNoCI(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	remote := h.AddRemote("origin")
	pullRequestOn(h, "origin")
	if _, err := h.Human().SetCITracking(context.Background(), h.Project, api.SetCITrackingInput{Tracking: true, Repository: "acme/app"}); err != nil {
		t.Fatal(err)
	}
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	r := h.Start()
	wantPushed(t, h, id, "")
	squash := h.CommitOn("squash", "feature.txt", "feature\n")
	h.PushByHand(remote, "squash", "main")
	reportPullRequest(t, h, id, squash)
	r.Poll()
	h.WantState(id, domain.Done)
}

func TestSwitchingFromPullRequestToMergeLandsWaitingTicketsAlreadyInTheBaseAndMergesTheOthers(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	h.AddRemote("origin")
	pullRequestOn(h, "origin")
	inBase, _ := accepted(h, "Merged by hand", "byhand.txt", "by hand\n")
	other, _ := accepted(h, "Still waiting", "waiting.txt", "waiting\n")
	r := h.Start()
	wantPushed(t, h, inBase, "")
	wantPushed(t, h, other, "")
	reportPullRequest(t, h, inBase, "")
	h.MergeByHand(inBase, "main")
	handMerge := h.Tip("main")

	h.SetLandingMethod(api.SetLandingMethodInput{Method: domain.LandingMerge})
	r.Poll()
	h.WantState(inBase, domain.Done)
	h.WantState(other, domain.Done)
	if h.Tip("main") == handMerge {
		t.Errorf("main is still at %s, want %s merged into it", handMerge, other)
	}
}

func TestSwitchingFromPullRequestToBranchWaitsForTheBranchInTheBaseByAncestry(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	h.AddRemote("origin")
	pullRequestOn(h, "origin")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	r := h.Start()
	wantPushed(t, h, id, "")
	reportPullRequest(t, h, id, "")

	h.SetLandingMethod(api.SetLandingMethodInput{Method: domain.LandingBranch})
	r.Poll()
	if tk := h.Ticket(id); tk.State != domain.Accepted {
		t.Fatalf("%s is %s, want it waiting Accepted for its branch", id, tk.State)
	}
	h.MergeByHand(id, "main")
	r.Poll()
	h.WantState(id, domain.Done)
}
