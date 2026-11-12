package factory_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// setMergeBase sets Merge with Push to origin and the base, which is what
// the Project's Options save together.
func setMergeBase(h *factorytest.Harness, base string) {
	h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingMerge, Push: true, Remote: "origin", Base: &base})
}

// pullRequestBase changes the base under Pull Request. The Factory
// reports the harness remote's path as the Push remote's URL on each poll,
// which Pull Request doesn't accept, so the Code Host's URL is reported
// again first.
func pullRequestBase(h *factorytest.Harness, base string) {
	if _, err := h.Server.Client.ReportPushRemote(context.Background(), h.Project, core.ReportPushRemoteInput{URL: "https://github.com/acme/app.git"}); err != nil {
		panic(err)
	}
	h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingPullRequest, Remote: "origin", Base: &base})
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
	r.WaitForOutput("the base, ghost, exists neither here nor on")

	setBase(h, "release")
	r.Poll()
	tk := h.WaitForState(id, store.Done)
	r.WaitForOutput("base: release, was ghost")
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
	r.WaitForOutput("the base, ghost, exists neither here nor on")
	h.Script(id, factorytest.Wait())

	setBase(h, "release")
	r.Poll()
	r.WaitForOutput(id + ": conflicts with release")
}

func TestChangingTheBaseAfterTheMergeButBeforeThePushFinishesOnTheOldBase(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	h.Git("branch", "release", "main")
	oldRelease := h.Git("rev-parse", "release")
	setMergeBase(h, "main")
	id, commit := accepted(h, "Add a feature", "feature.txt", "feature\n")
	remote.Break()
	r := h.Start()
	r.WaitForOutput(id + ": can't land yet")
	if !contains(h, "main", commit) {
		t.Fatalf("main doesn't hold %s: the test needs the merge made", commit)
	}

	setMergeBase(h, "release")
	remote.Restore()
	r.Poll()
	h.WaitForState(id, store.Done)
	r.WaitForOutput(id + ": pushed main to origin")
	r.WaitForOutput(id + ": landed on main at ")
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
	remote.Break()
	r := h.Start()
	r.WaitForOutput(h.SpecID() + ": merged spec/" + h.SpecID() + " into main at ")

	setMergeBase(h, "release")
	remote.Restore()
	r.Poll()
	h.WaitForState(a, store.Done)
	h.WaitForState(b, store.Done)
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
	r.WaitForOutput("the base, ghost, exists neither here nor on")

	setBase(h, "release")
	r.Poll()
	h.WaitForState(a, store.Done)
	h.WaitForState(b, store.Done)
	if got := h.Git("rev-parse", "main"); got != oldMain {
		t.Errorf("main moved to %s, want it left at %s", got, oldMain)
	}
	r.WaitForOutput(h.SpecID() + ": merged spec/" + h.SpecID() + " into release at ")
}

func TestUnderBranchAWaitingTicketIsWatchedOnTheNewBase(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.Git("branch", "release", "main")
	h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingBranch})
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	r := h.Start()
	r.WaitForOutput(id + ": Accepted under the Branch Landing Method: merge " + id + " into main")

	// Merged by hand into the branch the base will be.
	h.Git("checkout", "-q", "release")
	h.Git("merge", "-q", "--no-ff", "-m", "Merge "+id, id)
	h.Git("checkout", "-q", "main")
	r.Poll()
	if s := h.Ticket(id).State; s != store.Accepted {
		t.Fatalf("%s is %s, want it still waiting on main", id, s)
	}

	base := "release"
	h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingBranch, Base: &base})
	r.Poll()
	h.WaitForState(id, store.Done)
	r.WaitForOutput(id + ": merged into release by hand")
}

func TestUnderPullRequestAnOpenPullRequestKeepsItsTargetAndNewOnesUseTheNewBase(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	h.Git("branch", "release", "main")
	pullRequestOn(h, "origin")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	r := h.Start()
	waitPushed(t, h, id, "")
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
	h.WaitForState(id, store.Done)
	r.WaitForOutput(id + ": pull request 7 merged")
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
	r.WaitForOutput("has diverged from the remote's")

	pullRequestBase(h, "release")
	r.Poll()
	r.WaitForOutput("base: release, was main")
	h.WaitFor("Base diverged clears", func() bool { _, ok := baseDivergedItem(t, h); return !ok })
}
