package factory_test

import (
	"context"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// pullRequestOn sets the Pull Request Landing Method with the Push remote.
func pullRequestOn(h *factorytest.Harness, remote string) {
	// The harness's remote is a path; Pull Request needs a Code Host's URL.
	if _, err := h.Server.Client.ReportPushRemote(context.Background(), h.Project, core.ReportPushRemoteInput{URL: "https://github.com/acme/app.git", Base: "main"}); err != nil {
		panic(err)
	}
	h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingPullRequest, Remote: remote})
}

// waitPushed waits for the Factory to report ticket's branch as pushed, at
// commit when it isn't "": the Pull Request Watcher would see it in its list.
func waitPushed(t *testing.T, h *factorytest.Harness, ticket, commit string) {
	t.Helper()
	h.WaitFor("the pushed work is reported", func() bool {
		w, err := h.Server.Client.GetPullRequestWatch(context.Background(), h.Project)
		if err != nil {
			t.Fatal(err)
		}
		for _, work := range w.Work {
			if work.Branch == ticket && (commit == "" || work.Commit == commit) {
				return true
			}
		}
		return false
	})
}

// reportPullRequest is the Pull Request Watcher reporting ticket's pull
// request, as merged at commit when commit isn't "".
func reportPullRequest(t *testing.T, h *factorytest.Harness, branch, commit string) {
	t.Helper()
	in := core.ReportPullRequestInput{Branch: branch, Number: 7, Link: "https://example.test/pull/7", State: store.PROpen}
	if commit != "" {
		in.State, in.MergeCommit = store.PRMerged, commit
	}
	if _, err := h.Server.Client.ReportPullRequest(context.Background(), h.Project, in); err != nil {
		t.Fatal(err)
	}
}

func TestUnderPullRequestAcceptingPushesTheBranchAndReportsIt(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	pullRequestOn(h, "origin")
	id, commit := accepted(h, "Add a feature", "feature.txt", "feature\n")

	r := h.Start()
	r.WaitForOutput(id + ": pushed " + id + " to origin")
	if got := remote.Git("rev-parse", id); got != commit {
		t.Errorf("the remote's %s is at %q, want %s", id, got, commit)
	}
	waitPushed(t, h, id, commit)
	if tk := h.Ticket(id); tk.State != store.Accepted {
		t.Errorf("%s is %s, want it waiting Accepted for its pull request", id, tk.State)
	}
}

func TestUnderPullRequestEarlierAcceptedTicketsArePushedOnTheNextPoll(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingBranch})
	id, commit := accepted(h, "Accepted before the switch", "early.txt", "early\n")
	r := h.Start()
	r.Poll()

	pullRequestOn(h, "origin")
	r.Poll()
	waitPushed(t, h, id, commit)
	if got := remote.Git("rev-parse", id); got != commit {
		t.Errorf("the remote's %s is at %q, want %s", id, got, commit)
	}
}

func TestUnderPullRequestATicketLandsAtTheMergeCommitOfASquashMergeMadeOnTheRemote(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	pullRequestOn(h, "origin")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	r := h.Start()
	waitPushed(t, h, id, "")
	reportPullRequest(t, h, id, "")
	r.Poll()
	if tk := h.Ticket(id); tk.State != store.Accepted || tk.PullRequest == nil {
		t.Fatalf("%s is %s with pull request %+v, want Accepted with an open one", id, tk.State, tk.PullRequest)
	}

	// A squash merge: the host's commit has the work but not its history.
	squash := h.CommitOn("squash", "feature.txt", "feature\n")
	h.Git("push", "-q", remote.Path, "squash:refs/heads/main")
	reportPullRequest(t, h, id, squash)
	r.Poll()

	tk := h.WaitForState(id, store.Done)
	if m := tk.Moves[len(tk.Moves)-1]; m.Commit != squash {
		t.Errorf("landed at %q, want the merge commit %s", m.Commit, squash)
	}
	if got := h.Git("rev-parse", "main"); got != squash {
		t.Errorf("the local base is at %s, want it fast-forwarded to %s", got, squash)
	}
}

func TestUnderPullRequestATicketWaitsWhileTheMergeCommitIsNotFetched(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	pullRequestOn(h, "origin")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	r := h.Start()
	waitPushed(t, h, id, "")

	// The merge commit exists locally but the remote's base doesn't hold it.
	unfetched := h.CommitOn("unfetched", "feature.txt", "feature\n")
	reportPullRequest(t, h, id, unfetched)
	r.Poll()
	r.WaitForOutput(id + ": its pull request is merged at")
	if tk := h.Ticket(id); tk.State != store.Accepted {
		t.Errorf("%s is %s, want it waiting Accepted", id, tk.State)
	}
}

func TestUnderPullRequestAFailedFetchOnlyWarns(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	pullRequestOn(h, "origin")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	r := h.Start()
	waitPushed(t, h, id, "")
	squash := h.CommitOn("squash", "feature.txt", "feature\n")
	h.Git("push", "-q", remote.Path, "squash:refs/heads/main")
	reportPullRequest(t, h, id, squash)

	remote.Break()
	r.Poll()
	r.WaitForOutput("warning: can't fetch origin")
	if _, ok := baseDivergedItem(t, h); ok {
		t.Error("a failed fetch showed Base diverged")
	}
	if tk := h.Ticket(id); tk.State != store.Accepted {
		t.Errorf("%s is %s, want it left Accepted", id, tk.State)
	}

	remote.Restore()
	r.Poll()
	h.WaitForState(id, store.Done)
}

func TestUnderPullRequestADivergedBaseStopsLandingAndClaiming(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	pullRequestOn(h, "origin")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	h.Git("push", "-q", remote.Path, "main:refs/heads/main")
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	h.Git("push", "-q", remote.Path, "theirs:refs/heads/main")
	h.CommitOn("main", "mine.txt", "mine\n")
	r := h.Start()
	waitPushed(t, h, id, "")
	reportPullRequest(t, h, id, theirs)
	ready := h.ReadyTicket("Waits for the base")
	r.Poll()
	r.WaitForOutput("has diverged from the remote's")
	if tk := h.Ticket(id); tk.State != store.Accepted {
		t.Errorf("%s is %s, want it held Accepted", id, tk.State)
	}
	if tk := h.Ticket(ready); tk.State != store.Ready {
		t.Errorf("%s is %s, want it not claimed", ready, tk.State)
	}
	local := h.Git("rev-parse", "main")
	h.WaitFor("Base diverged shows in Needs Me", func() bool {
		row, ok := baseDivergedItem(t, h)
		return ok && row.BaseDiverged.Local == local && row.BaseDiverged.Remote == theirs
	})

	h.Git("reset", "-q", "--hard", "origin/main")
	r.Poll()
	h.WaitForState(id, store.Done)
	h.WaitFor("Base diverged clears", func() bool { _, ok := baseDivergedItem(t, h); return !ok })
	h.WaitFor("the turn", func() bool { return len(h.Calls()) == 1 })
}

// baseDivergedItem is the Project's Base diverged item in Needs Me.
func baseDivergedItem(t *testing.T, h *factorytest.Harness) (core.NeedsMeRow, bool) {
	t.Helper()
	v, err := h.Human().NeedsMe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range v.Groups {
		if g.Reason == core.ReasonBaseDiverged {
			for _, row := range g.Tickets {
				if row.Project == h.Project {
					return row, true
				}
			}
		}
	}
	return core.NeedsMeRow{}, false
}

func TestUnderPullRequestANewBranchIsCutFromTheFastForwardedBase(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	pullRequestOn(h, "origin")
	h.Git("push", "-q", remote.Path, "main:refs/heads/main")
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	h.Git("push", "-q", remote.Path, "theirs:refs/heads/main")
	ready := h.ReadyTicket("Starts from the host's base")

	h.Start()
	h.WaitFor("the turn", func() bool { return len(h.Calls()) == 1 })
	if got := h.Git("rev-parse", "main"); got != theirs {
		t.Errorf("the local base is at %s, want it fast-forwarded to %s", got, theirs)
	}
	if h.Calls()[0].Ticket != ready {
		t.Fatalf("calls %+v, want %s claimed", h.Calls(), ready)
	}
	// The branch was cut from the host's base, so it holds its commit.
	h.Git("merge-base", "--is-ancestor", theirs, ready)
}

func TestUnderPullRequestATicketWithNothingToMergeLandsWithoutAPullRequest(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	pullRequestOn(h, "origin")
	id := h.ReadyTicket("No change")
	head := h.Git("rev-parse", "main")
	h.Finished(id, "main", head)
	h.Accept(id)

	h.Start()
	tk := h.WaitForState(id, store.Done)
	if tk.PullRequest != nil {
		t.Errorf("%s has pull request %+v, want none", id, tk.PullRequest)
	}
}

func TestUnderPullRequestATicketWithoutABranchLands(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	pullRequestOn(h, "origin")
	id := h.ReadyTicket("Research")
	h.Finished(id, "", "")
	h.Accept(id)

	h.Start()
	h.WaitForState(id, store.Done)
}

func TestUnderPullRequestAMergedPullRequestLandsAnInReviewTicketAndWarnsOfOthers(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	pullRequestOn(h, "origin")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	r := h.Start()
	waitPushed(t, h, id, "")
	reportPullRequest(t, h, id, "")
	if _, err := h.Human().SendBack(context.Background(), h.Project, core.SendBackInput{Ticket: core.Ref(id), Note: "fix"}); err != nil {
		t.Fatal(err)
	}
	squash := h.CommitOn("squash", "feature.txt", "feature\n")
	h.Git("push", "-q", remote.Path, "squash:refs/heads/main")
	reportPullRequest(t, h, id, squash)
	r.Poll()
	r.WaitForOutput("warning: " + id + " is")
	if !commented(h.Ticket(id), "pull request 7 was merged") {
		t.Errorf("want a comment on the merge, got %+v", h.Ticket(id).Comments)
	}
}

func TestUnderPullRequestLandingNeedsNoCI(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	pullRequestOn(h, "origin")
	if _, err := h.Human().SetCITracking(context.Background(), h.Project, core.SetCITrackingInput{Tracking: true, Repository: "acme/app"}); err != nil {
		t.Fatal(err)
	}
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	r := h.Start()
	waitPushed(t, h, id, "")
	squash := h.CommitOn("squash", "feature.txt", "feature\n")
	h.Git("push", "-q", remote.Path, "squash:refs/heads/main")
	reportPullRequest(t, h, id, squash)
	r.Poll()
	h.WaitForState(id, store.Done)
}

func TestSwitchingFromPullRequestToMergeLandsWaitingTicketsAlreadyInTheBaseAndMergesTheOthers(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	pullRequestOn(h, "origin")
	inBase, _ := accepted(h, "Merged by hand", "byhand.txt", "by hand\n")
	other, _ := accepted(h, "Still waiting", "waiting.txt", "waiting\n")
	r := h.Start()
	waitPushed(t, h, inBase, "")
	waitPushed(t, h, other, "")
	reportPullRequest(t, h, inBase, "")
	h.Git("merge", "-q", "--no-ff", "-m", "by hand", inBase)
	handMerge := h.Git("rev-parse", "main")

	h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingMerge})
	r.Poll()
	h.WaitForState(inBase, store.Done)
	h.WaitForState(other, store.Done)
	if h.Git("rev-parse", "main") == handMerge {
		t.Errorf("main is still at %s, want %s merged into it", handMerge, other)
	}
}

func TestSwitchingFromPullRequestToBranchWaitsForTheBranchInTheBaseByAncestry(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	pullRequestOn(h, "origin")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	r := h.Start()
	waitPushed(t, h, id, "")
	reportPullRequest(t, h, id, "")

	h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingBranch})
	r.Poll()
	if tk := h.Ticket(id); tk.State != store.Accepted {
		t.Fatalf("%s is %s, want it waiting Accepted for its branch", id, tk.State)
	}
	h.Git("merge", "-q", "--no-ff", "-m", "merged by hand", id)
	r.Poll()
	h.WaitForState(id, store.Done)
}
