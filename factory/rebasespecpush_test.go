package factory_test

import (
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/factory/factorytest"
)

// Spec code-central-1196: the Spec Branch's rebase moves each Ticket's work
// with it. These use real git: the Fake doesn't model a rebase.

// rebasedSpecPushed is a Spec whose two Tickets were replayed onto its Spec
// Branch before the base moved on, then landed by merge and push, with CI
// Tracking on: it returns the Tickets, the pushed commit and the running
// Factory, with the Tickets Accepted and waiting on CI.
func rebasedSpecPushed(t *testing.T, h *factorytest.Rig) (a, b, pushed string, r *factorytest.Running) {
	t.Helper()
	remote := h.AddRemote("origin")
	on, landPerSpec := true, true
	h.SetLandingMethod(api.SetLandingMethodInput{Method: domain.LandingMerge, Push: true, Remote: "origin", Rebase: &on, LandPerSpec: &landPerSpec})
	trackCI(t, h)
	h.Publish(remote, h.Tip("main"), "main")
	h.MakeRequirement()
	a, _ = accepted(h, "First", "one.txt", "1\n")
	b, _ = accepted(h, "Second", "two.txt", "2\n")
	release := holdTicketsStage(t, h)
	r = h.Start()
	r.WantOutput(b + ": replayed onto spec/" + h.SpecID())

	h.Publish(remote, h.CommitOn("main", "base.txt", "base\n"), "main")
	release()
	r.Poll()
	r.WantOutput(a + ": CI tracks main at ")
	tk := h.Ticket(a)
	if tk.State != domain.Accepted || tk.PushedCommit == nil {
		t.Fatalf("%s with pushed commit %+v, want the Spec pushed and its Tickets left Accepted", tk.State, tk.PushedCommit)
	}
	return a, b, tk.PushedCommit.Commit, r
}

func TestUnderMergeAndPushWithRebaseASpecsTicketsLandOnceCIPassesOnItsRebasedPush(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.RealGit())
	a, b, pushed, r := rebasedSpecPushed(t, h)

	reportRun(t, h, 1, a, pushed, domain.CICompleted, domain.CISuccess)
	reportRun(t, h, 2, b, pushed, domain.CICompleted, domain.CISuccess)
	r.Poll()
	h.WantState(a, domain.Done)
	h.WantState(b, domain.Done)
}

func TestUnderMergeAndPushWithRebaseASpecsTicketsLandAfterARestartBetweenThePushAndCI(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.RealGit())
	a, b, pushed, r := rebasedSpecPushed(t, h)

	r.Stop()
	reportRun(t, h, 1, a, pushed, domain.CICompleted, domain.CISuccess)
	reportRun(t, h, 2, b, pushed, domain.CICompleted, domain.CISuccess)
	h.Start()
	h.WantState(a, domain.Done)
	h.WantState(b, domain.Done)
}

func TestUnderPullRequestWithRebaseASpecsTicketsLandOnceItsRebasedPullRequestMergesAndCIPasses(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.RealGit())
	remote := h.AddRemote("origin")
	rebasePullRequestOn(h, "origin", true)
	trackCI(t, h)
	h.Publish(remote, h.Tip("main"), "main")
	h.MakeRequirement()
	a, _ := accepted(h, "First", "one.txt", "1\n")
	b, _ := accepted(h, "Second", "two.txt", "2\n")
	branch := "spec/" + h.SpecID()
	release := holdTicketsStage(t, h)
	r := h.Start()
	r.WantOutput(b + ": replayed onto " + branch)

	h.Publish(remote, h.CommitOn("main", "base.txt", "base\n"), "main")
	release()
	r.Poll()
	r.WantOutput(h.SpecID() + ": pushed " + branch + " to origin")
	pushed := h.Tip(branch)
	for _, id := range []string{a, b} {
		tk := h.Ticket(id)
		if !descendsFrom(h, pushed, h.Tip(id)) || tk.SpecMerge == nil || !descendsFrom(h, pushed, tk.SpecMerge.Commit) {
			t.Errorf("%s's branch %s and Spec merge %+v aren't in the rebased Spec Branch %s", id, h.Tip(id), tk.SpecMerge, pushed)
		}
	}

	squash := h.CommitOn("squash", "one.txt", "1\n")
	h.PushByHand(remote, "squash", "main")
	reportPullRequest(t, h, branch, squash)
	reportRun(t, h, 1, a, squash, domain.CICompleted, domain.CISuccess)
	reportRun(t, h, 2, b, squash, domain.CICompleted, domain.CISuccess)
	r.Poll()
	h.WantState(a, domain.Done)
	h.WantState(b, domain.Done)
}
