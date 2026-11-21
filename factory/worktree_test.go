package factory_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"

	"github.com/tomoram/code-central/factory/factorytest"
)

func TestAnExistingWorktreeOnTheTicketsBranchIsReused(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("Carry on")
	h.AddWorktreeOnNewBranch(h.Worktree(id), id, "main")
	left := filepath.Join(h.Worktree(id), "left-by-an-earlier-claim.txt")
	if err := os.WriteFile(left, []byte("wip"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.Script(id, factorytest.NeedFile("left-by-an-earlier-claim.txt"), factorytest.Finish("Done."), factorytest.Result("success"))

	h.Start()
	h.WantState(id, domain.InReview)
	if c := h.Calls()[0]; c.Dir != h.Worktree(id) {
		t.Errorf("claude ran in %s, want the reused worktree", c.Dir)
	}
}

func TestAnExistingBranchNamedAfterTheTicketGetsTheWorktree(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("Started before")
	h.NewBranch(id, "main")
	h.CommitOn("main", "moves-on.txt", "main moves on\n")
	branchTip := h.Tip(id)

	h.Start()
	h.WantState(id, domain.InReview)
	if got := h.HeadOf(h.Worktree(id)); got != branchTip {
		t.Errorf("worktree at %s, want the existing branch's tip %s", got, branchTip)
	}
}

func TestANewBranchStartsFromTheProjectsBase(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("On release")
	h.NewBranch("release", "main")
	h.CommitOn("main", "moves-on.txt", "main moves on\n")
	h.Detach()
	releaseTip := h.Tip("release")
	release := "release"
	h.SetLandingMethod(api.SetLandingMethodInput{Method: domain.LandingMerge, Base: &release})

	r := h.Start()
	r.WantOutput("base: release")
	h.WantState(id, domain.InReview)
	if got := h.HeadOf(h.Worktree(id)); got != releaseTip {
		t.Errorf("branch %s starts at %s, want release's tip %s", id, got, releaseTip)
	}
}
