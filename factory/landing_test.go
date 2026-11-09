package factory_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

var branchLanding = core.SetLandingMethodInput{Method: store.LandingBranch}

// withWorktree makes an Accepted Ticket whose branch adds file, with the
// worktree a Factory Claim would have left at its path.
func withWorktree(h *factorytest.Harness, title, file, content string) (id, commit string) {
	id = h.ReadyTicket(title)
	commit = h.CommitOn(id, file, content)
	h.Git("worktree", "add", "-q", h.Worktree(id), id)
	h.Finished(id, id, commit)
	h.Accept(id)
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
	if s := h.Ticket(id).State; s != store.Accepted {
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
	r.WaitForOutput(id + ": Accepted under the Branch Landing Method: merge " + id + " into main")

	h.Git("merge", "-q", "--no-ff", "-m", "My own merge", id)
	r.Poll()
	tk := h.WaitForState(id, store.Done)
	if m := tk.Moves[len(tk.Moves)-1]; m.From != store.Accepted || m.Commit != commit {
		t.Errorf("land move %+v, want from Accepted with the finish commit %s", m, commit)
	}
	if strings.Count(r.Stdout(), "Accepted under the Branch Landing Method") != 1 {
		t.Errorf("want the Branch line once, got %q", r.Stdout())
	}
}

func TestUnderBranchATicketFinishedWithoutACommitLandsAtItsBranchTip(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.SetLandingMethod(branchLanding)
	id := h.ReadyTicket("No commit given")
	tip := h.CommitOn(id, "feature.txt", "feature\n")
	h.Finished(id, id, "")
	h.Accept(id)
	h.Git("merge", "-q", "--no-ff", "-m", "My own merge", id)

	h.Start()
	tk := h.WaitForState(id, store.Done)
	if m := tk.Moves[len(tk.Moves)-1]; m.Commit != tip {
		t.Errorf("landed with %q, want the branch tip %s", m.Commit, tip)
	}
}

func TestUnderBranchAConflictingBranchIsNeverSentBack(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.SetLandingMethod(branchLanding)
	id, _ := accepted(h, "Change the readme", "README", "theirs\n")
	h.CommitOn("main", "README", "ours\n")
	base := h.Git("rev-parse", "main")

	r := h.Start()
	r.Poll()
	tk := h.Ticket(id)
	if tk.State != store.Accepted || movedTo(tk, store.Accepted, store.Ready) {
		t.Errorf("%s is %s with moves %+v; want it left Accepted, never sent back", id, tk.State, tk.Moves)
	}
	if h.Git("rev-parse", "main") != base {
		t.Error("main moved under the Branch Landing Method")
	}
	if commented(tk, "Factory") {
		t.Errorf("want no comment from the Factory, got %+v", tk.Comments)
	}
}

func TestUnderBranchADirtyWorktreeOfAnAcceptedTicketIsWarnedAboutAndLeft(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.SetLandingMethod(branchLanding)
	id, _ := withWorktree(h, "Add a feature", "feature.txt", "feature\n")
	mine := filepath.Join(h.Worktree(id), "notes.txt")
	if err := os.WriteFile(mine, []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := h.Start()
	r.Poll()
	if got := strings.Count(r.Stdout(), "warning: "+id+" is Accepted but its worktree"); got != 1 {
		t.Errorf("want one warning over two polls, got %d in %q", got, r.Stdout())
	}
	if _, err := os.Stat(mine); err != nil {
		t.Errorf("the dirty worktree was touched: %v", err)
	}
}

func TestUnderMergeAnAcceptedTicketsWorktreeIsKeptUntilItLands(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id, _ := withWorktree(h, "Change the readme", "README", "theirs\n")
	h.CommitOn("main", "README", "ours\n")
	// Hold the next Claim's turn, so the sent-back Ticket isn't worked.
	h.Script(id, factorytest.Wait())

	r := h.Start()
	r.Poll()
	if strings.Contains(r.Stdout(), "removed its worktree") {
		t.Errorf("a worktree was removed under Merge: %q", r.Stdout())
	}
}

func TestAChangeOfLandingMethodAppliesFromTheNextPoll(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.SetLandingMethod(branchLanding)
	first, _ := accepted(h, "Left as a branch", "first.txt", "first\n")
	base := h.Git("rev-parse", "main")

	r := h.Start()
	r.Poll()
	if s := h.Ticket(first).State; s != store.Accepted || h.Git("rev-parse", "main") != base {
		t.Fatalf("%s is %s and main moved to %s; want it left under Branch", first, s, h.Git("rev-parse", "main"))
	}

	// Branch to Merge: the Accepted Ticket not merged by hand is merged.
	h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingMerge})
	r.Poll()
	h.WaitForState(first, store.Done)
	if got := h.Git("log", "-1", "--format=%s", "main"); got != "Merge "+first+": Left as a branch" {
		t.Errorf("main's tip is %q, want the Factory's merge of %s", got, first)
	}

	// Merge to Branch: the next accept is left alone.
	h.SetLandingMethod(branchLanding)
	second, _ := accepted(h, "Left as a branch too", "second.txt", "second\n")
	base = h.Git("rev-parse", "main")
	r.Poll()
	r.Poll()
	if s := h.Ticket(second).State; s != store.Accepted || h.Git("rev-parse", "main") != base {
		t.Errorf("%s is %s and main at %s; want it left under Branch", second, s, h.Git("rev-parse", "main"))
	}
}
