package factory_test

import (
	"context"
	"github.com/tomoram/code-central/domain"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
)

// setMergeBase sets Merge with Push to origin and the base, which is what
// the Project's Options save together.
func setMergeBase(h *factorytest.Harness, base string) {
	h.SetLandingMethod(core.SetLandingMethodInput{Method: domain.LandingMerge, Push: true, Remote: "origin", Base: &base})
}

// pullRequestBase changes the base under Pull Request. The Factory
// reports the harness remote's path as the Push remote's URL on each poll,
// which Pull Request doesn't accept, so the Code Host's URL is reported
// again first.
func pullRequestBase(h *factorytest.Harness, base string) {
	if _, err := h.Server.Client.ReportPushRemote(context.Background(), h.Project, core.ReportPushRemoteInput{URL: "https://github.com/acme/app.git"}); err != nil {
		panic(err)
	}
	h.SetLandingMethod(core.SetLandingMethodInput{Method: domain.LandingPullRequest, Remote: "origin", Base: &base})
}

func contains(h *factorytest.Harness, branch, commit string) bool {
	return strings.Contains(h.Git("branch", "--contains", commit, "--list", branch), branch)
}

func TestChangingTheBaseMergesAnAcceptedTicketNotYetMergedIntoTheNewBase(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.Git("branch", "release", "main")
	oldMain := h.Git("rev-parse", "main")
	// A base that exists nowhere holds landing, so the Ticket is still
	// Accepted when the base changes.
	setBase(h, "ghost")
	id, commit := accepted(h, "Add a feature", "feature.txt", "feature\n")
	r := h.Start()
	r.WantOutput("the base, ghost, doesn't exist here, and this repository is local-only")

	setBase(h, "release")
	r.Poll()
	tk := h.WantState(id, domain.Done)
	r.WantOutput("base: release, was ghost")
	if !contains(h, "release", commit) {
		t.Errorf("release doesn't hold %s", commit)
	}
	if got := h.Git("rev-parse", "main"); got != oldMain {
		t.Errorf("main moved to %s, want it left at %s", got, oldMain)
	}
	if m := tk.Moves[len(tk.Moves)-1]; m.Commit != h.Git("rev-parse", "release") {
		t.Errorf("landed at %q, want release's merge", m.Commit)
	}
}

func TestChangingTheBaseSendsBackABranchThatConflictsWithTheNewBase(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.CommitOn("release", "feature.txt", "release's\n")
	setBase(h, "ghost")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	r := h.Start()
	r.WantOutput("the base, ghost, doesn't exist here, and this repository is local-only")
	h.Script(id, factorytest.Wait())

	setBase(h, "release")
	r.Poll()
	r.WantOutput(id + ": conflicts with release")
}

func TestChangingTheBaseAfterTheMergeButBeforeThePushFinishesOnTheOldBase(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	h.Git("branch", "release", "main")
	oldRelease := h.Git("rev-parse", "release")
	setMergeBase(h, "main")
	id, commit := accepted(h, "Add a feature", "feature.txt", "feature\n")
	remote.RefusePushes()
	r := h.Start()
	r.WantOutput(id + ": can't land yet")
	if !contains(h, "main", commit) {
		t.Fatalf("main doesn't hold %s: the test needs the merge made", commit)
	}

	setMergeBase(h, "release")
	remote.AcceptPushes()
	r.Poll()
	h.WantState(id, domain.Done)
	r.WantOutput(id + ": pushed main to origin")
	r.WantOutput(id + ": landed on main at ")
	if got, want := remote.Git("rev-parse", "main"), h.Git("rev-parse", "main"); got != want {
		t.Errorf("the remote's main is at %s, want %s", got, want)
	}
	if got := h.Git("rev-parse", "release"); got != oldRelease {
		t.Errorf("release moved to %s: the merge was made again", got)
	}
}

func TestChangingTheBaseAfterASpecIsMergedButBeforeThePushFinishesOnTheOldBase(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	h.Git("branch", "release", "main")
	oldRelease := h.Git("rev-parse", "release")
	setMergeBase(h, "main")
	a, b := twoAccepted(t, h)
	remote.RefusePushes()
	r := h.Start()
	r.WantOutput(h.SpecID() + ": merged spec/" + h.SpecID() + " into main at ")

	setMergeBase(h, "release")
	remote.AcceptPushes()
	r.Poll()
	h.WantState(a, domain.Done)
	h.WantState(b, domain.Done)
	if got, want := remote.Git("rev-parse", "main"), h.Git("rev-parse", "main"); got != want {
		t.Errorf("the remote's main is at %s, want %s", got, want)
	}
	if got := h.Git("rev-parse", "release"); got != oldRelease {
		t.Errorf("release moved to %s: the Spec was merged again", got)
	}
}

func TestChangingTheBaseMergesAWaitingSpecBranchIntoTheNewBase(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.Git("branch", "release", "main")
	oldMain := h.Git("rev-parse", "main")
	setBase(h, "ghost")
	a, b := twoAccepted(t, h)
	r := h.Start()
	r.WantOutput("the base, ghost, doesn't exist here, and this repository is local-only")

	setBase(h, "release")
	r.Poll()
	h.WantState(a, domain.Done)
	h.WantState(b, domain.Done)
	if got := h.Git("rev-parse", "main"); got != oldMain {
		t.Errorf("main moved to %s, want it left at %s", got, oldMain)
	}
	r.WantOutput(h.SpecID() + ": merged spec/" + h.SpecID() + " into release at ")
}

func TestUnderBranchAWaitingTicketIsWatchedOnTheNewBase(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.Git("branch", "release", "main")
	h.SetLandingMethod(core.SetLandingMethodInput{Method: domain.LandingBranch})
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	r := h.Start()
	r.WantOutput(id + ": Accepted under the Branch Landing Method: merge " + id + " into main")

	// Merged by hand into the branch the base will be.
	h.Git("checkout", "-q", "release")
	h.Git("merge", "-q", "--no-ff", "-m", "Merge "+id, id)
	h.Git("checkout", "-q", "main")
	r.Poll()
	if s := h.Ticket(id).State; s != domain.Accepted {
		t.Fatalf("%s is %s, want it still waiting on main", id, s)
	}

	base := "release"
	h.SetLandingMethod(core.SetLandingMethodInput{Method: domain.LandingBranch, Base: &base})
	r.Poll()
	h.WantState(id, domain.Done)
	r.WantOutput(id + ": merged into release by hand")
}

func TestUnderPullRequestAnOpenPullRequestKeepsItsTargetAndNewOnesUseTheNewBase(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	h.Git("branch", "release", "main")
	pullRequestOn(h, "origin")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	r := h.Start()
	wantPushed(t, h, id, "")
	reportPullRequest(t, h, id, "")

	pullRequestBase(h, "release")
	r.Poll()
	w, err := h.Server.Client.GetPullRequestWatch(context.Background(), h.Project)
	if err != nil {
		t.Fatal(err)
	}
	if w.Base != "release" {
		t.Errorf("the Watcher's base is %q, want new pull requests to target release", w.Base)
	}

	// The open pull request was into main, and is merged there.
	merge := h.CommitOn("theirs", "feature.txt", "feature\n")
	h.Git("push", "-q", remote.Path, "theirs:refs/heads/main")
	reportPullRequest(t, h, id, merge)
	r.Poll()
	h.WantState(id, domain.Done)
	r.WantOutput(id + ": pull request 7 merged")
}

func TestChangingTheBaseResetsBaseDiverged(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	h.Git("branch", "release", "main")
	h.Git("push", "-q", remote.Path, "main:refs/heads/release")
	h.Git("push", "-q", remote.Path, h.CommitOn("theirs", "theirs.txt", "t\n")+":refs/heads/main")
	h.CommitOn("main", "ours.txt", "o\n")
	pullRequestOn(h, "origin")
	r := h.Start()
	r.WantOutput("has diverged from the remote's")

	pullRequestBase(h, "release")
	r.Poll()
	r.WantOutput("base: release, was main")
	if _, ok := baseDivergedItem(t, h); ok {
		t.Error("Base diverged is still raised, want it cleared by the new base")
	}
}

func TestLandingByMergeWithoutPushResetsBaseDiverged(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	h.Git("push", "-q", remote.Path, h.CommitOn("theirs", "theirs.txt", "t\n")+":refs/heads/main")
	h.CommitOn("main", "ours.txt", "o\n")
	pullRequestOn(h, "origin")
	ready := h.ReadyTicket("Waits for the base")
	h.Script(ready, factorytest.Wait())
	r := h.Start()
	r.WantOutput("has diverged from the remote's")
	if tk := h.Ticket(ready); tk.State != domain.Ready {
		t.Fatalf("%s is %s, want it not claimed while the base has diverged", ready, tk.State)
	}

	// Merge without Push never keeps the base level with the remote's.
	h.SetLandingMethod(core.SetLandingMethodInput{Method: domain.LandingMerge})
	r.Poll()

	h.WantState(ready, domain.InProgress)
	if _, ok := baseDivergedItem(t, h); ok {
		t.Error("Base diverged is still raised, want it cleared under Merge without Push")
	}
}
