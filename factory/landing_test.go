package factory_test

import (
	"github.com/tomoram/code-central/domain"
	"os"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
)

var branchLanding = core.SetLandingMethodInput{Method: domain.LandingBranch}

// withWorktree makes an Accepted Ticket whose branch adds file, with the
// worktree a Factory Claim would have left at its path.
func withWorktree(h *factorytest.Harness, title, file, content string) (id, commit string) {
	id, commit = accepted(h, title, file, content)
	h.Git("worktree", "add", "-q", h.Worktree(id), id)
	return id, commit
}

func TestUnderBranchAnAcceptedTicketIsLeftOnItsBranchAndLandedOnceMergedByHand(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.SetLandingMethod(branchLanding)
	id, commit := withWorktree(h, "Add a feature", "feature.txt", "feature\n")
	base := h.Git("rev-parse", "main")

	r := h.Start()
	r.Poll()
	if s := h.Ticket(id).State; s != domain.Accepted {
		t.Fatalf("%s is %s, want it left Accepted", id, s)
	}
	if h.Git("rev-parse", "main") != base {
		t.Error("main moved under the Branch Landing Method")
	}
	if _, err := os.Stat(h.Worktree(id)); !os.IsNotExist(err) {
		t.Errorf("the worktree %s is still there: %v", h.Worktree(id), err)
	}
	if got := h.Git("rev-parse", "refs/heads/"+id); got != commit {
		t.Errorf("branch %s is at %q, want it kept at %s", id, got, commit)
	}
	r.WantOutput(id + ": Accepted under the Branch Landing Method: merge " + id + " into main")

	h.Git("merge", "-q", "--no-ff", "-m", "My own merge", id)
	r.Poll()
	tk := h.WantState(id, domain.Done)
	if m := tk.Moves[len(tk.Moves)-1]; m.From != domain.Accepted || m.Commit != commit {
		t.Errorf("land move %+v, want from Accepted with the finish commit %s", m, commit)
	}
	if strings.Count(r.Stdout(), "Accepted under the Branch Landing Method") != 1 {
		t.Errorf("want the Branch line once, got %q", r.Stdout())
	}
}
