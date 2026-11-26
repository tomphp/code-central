package factory_test

import (
	"os"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"

	"github.com/tomoram/code-central/factory/factorytest"
)

var branchLanding = api.SetLandingMethodInput{Method: domain.LandingBranch}

// withWorktree makes an Accepted Ticket whose branch adds file, with the
// worktree a Factory Claim would have left at its path.
func withWorktree(h *factorytest.Rig, title, file, content string) (id, commit string) {
	id, commit = accepted(h, title, file, content)
	h.AddWorktreeAt(h.Worktree(id), id)
	return id, commit
}

func TestUnderBranchAnAcceptedTicketIsLeftOnItsBranchAndLandedOnceMergedByHand(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	h.SetLandingMethod(branchLanding)
	id, commit := withWorktree(h, "Add a feature", "feature.txt", "feature\n")
	base := h.Tip("main")

	r := h.Start()
	r.Poll()
	if s := h.Ticket(id).State; s != domain.Accepted {
		t.Fatalf("%s is %s, want it left Accepted", id, s)
	}
	if h.Tip("main") != base {
		t.Error("main moved under the Branch Landing Method")
	}
	if _, err := os.Stat(h.Worktree(id)); !os.IsNotExist(err) {
		t.Errorf("the worktree %s is still there: %v", h.Worktree(id), err)
	}
	if got := h.Tip(id); got != commit {
		t.Errorf("branch %s is at %q, want it kept at %s", id, got, commit)
	}
	r.WantOutput(id + ": Accepted under the Branch Landing Method: merge " + id + " into main")

	h.MergeByHand(id, "main")
	r.Poll()
	tk := h.WantState(id, domain.Done)
	if m := tk.Moves[len(tk.Moves)-1]; m.From != domain.Accepted || m.Commit != commit {
		t.Errorf("land move %+v, want from Accepted with the finish commit %s", m, commit)
	}
	if strings.Count(r.Stdout(), "Accepted under the Branch Landing Method") != 1 {
		t.Errorf("want the Branch line once, got %q", r.Stdout())
	}
}
