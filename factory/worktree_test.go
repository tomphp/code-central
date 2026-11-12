package factory_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

func TestAnExistingWorktreeOnTheTicketsBranchIsReused(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Carry on")
	h.Git("worktree", "add", "-q", "-b", id, h.Worktree(id), "main")
	left := filepath.Join(h.Worktree(id), "left-by-an-earlier-claim.txt")
	if err := os.WriteFile(left, []byte("wip"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.Script(id, factorytest.NeedFile("left-by-an-earlier-claim.txt"), factorytest.Finish("Done."), factorytest.Result("success"))

	h.Start()
	h.WaitForState(id, store.InReview)
	if c := h.Calls()[0]; c.Dir != h.Worktree(id) {
		t.Errorf("claude ran in %s, want the reused worktree", c.Dir)
	}
}

func TestAnExistingBranchNamedAfterTheTicketGetsTheWorktree(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Started before")
	h.Git("branch", id)
	h.Git("commit", "-q", "--allow-empty", "-m", "main moves on")
	branchTip := h.Git("rev-parse", id)

	h.Start()
	h.WaitForState(id, store.InReview)
	if got := h.GitIn(h.Worktree(id), "rev-parse", "HEAD"); got != branchTip {
		t.Errorf("worktree at %s, want the existing branch's tip %s", got, branchTip)
	}
}

func TestANewBranchStartsFromTheProjectsBase(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("On release")
	h.Git("branch", "release")
	h.Git("commit", "-q", "--allow-empty", "-m", "main moves on")
	h.Git("checkout", "-q", "--detach")
	releaseTip := h.Git("rev-parse", "release")
	release := "release"
	h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingMerge, Base: &release})

	r := h.Start()
	r.WaitForOutput("base: release")
	h.WaitForState(id, store.InReview)
	if got := h.GitIn(h.Worktree(id), "rev-parse", "HEAD"); got != releaseTip {
		t.Errorf("branch %s starts at %s, want release's tip %s", id, got, releaseTip)
	}
}
