package factory_test

import (
	"os"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"

	"github.com/tomoram/code-central/factory/factorytest"
)

// requirementAccepted is accepted on a Requirement Spec.
func requirementAccepted(h *factorytest.Harness, title, file, content string) (id, commit string) {
	h.MakeRequirement()
	return accepted(h, title, file, content)
}

func TestAnAcceptedSpecLandedTicketIsMergedIntoTheSpecBranchAndNotTheBase(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id, commit := requirementAccepted(h, "Add a feature", "feature.txt", "feature\n")
	oldMain := h.Git("rev-parse", "main")
	holdTicketsStage(t, h)

	r := h.Start()
	r.WantOutput(id + ": merged into spec/" + h.SpecID() + " at ")

	branch := "spec/" + h.SpecID()
	tip := h.Git("rev-parse", branch)
	if got := h.Git("log", "-1", "--format=%P", tip); got != oldMain+" "+commit {
		t.Errorf("%s has parents %q, want %q then %q", branch, got, oldMain, commit)
	}
	if h.Git("rev-parse", "main") != oldMain {
		t.Error("main moved: nothing reaches the base until the Spec lands")
	}
	tk := h.Ticket(id)
	if tk.State != domain.Accepted || tk.SpecMerge == nil || tk.SpecMerge.Commit != tip || !tk.SpecMerge.Counts {
		t.Errorf("%s is %s with Spec merge %+v, want Accepted merged at %s", id, tk.State, tk.SpecMerge, tip)
	}
}

func TestASpecBranchConflictSendsTheTicketBack(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.MakeRequirement()
	first, _ := accepted(h, "First", "README", "first\n")
	second, _ := accepted(h, "Second", "README", "second\n")

	h.Script(second, factorytest.Wait())
	h.Start()
	if !movedTo(h.Ticket(second), domain.Accepted, domain.Ready) {
		t.Fatalf("%s was not sent back: %+v", second, h.Ticket(second).Moves)
	}

	if tk := h.Ticket(first); tk.SpecMerge == nil {
		t.Errorf("%s, the older accept, has no Spec merge", first)
	}
	if !commented(h.Ticket(second), "spec/"+h.SpecID(), "README") {
		t.Errorf("no send_back note naming the Spec Branch and README: %+v", h.Ticket(second).Comments)
	}
}

func TestATicketWhoseWorkIsInTheBaseLandsAsPlainMerge(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id, commit := requirementAccepted(h, "Add a feature", "feature.txt", "feature\n")
	h.Git("merge", "-q", "--ff-only", commit)

	h.Start()
	h.WantState(id, domain.Done)
	if got := h.Git("branch", "--list", "spec/*"); got != "" {
		t.Errorf("a Spec Branch was made: %q", got)
	}
}

func TestTheFirstClaimMakesTheSpecBranchAndCutsTheTicketBranchFromIt(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.MakeRequirement()
	id := h.ReadyTicket("Add a feature")
	h.Script(id, factorytest.Wait())
	base := h.Git("rev-parse", "main")

	h.Start()

	if got := h.Git("rev-parse", "spec/"+h.SpecID()); got != base {
		t.Errorf("the Spec Branch is at %s, want the base's tip %s", got, base)
	}
	if got := h.GitIn(h.Worktree(id), "rev-parse", "HEAD"); got != base {
		t.Errorf("the Ticket's branch is at %s, want the Spec Branch's tip %s", got, base)
	}
}

func TestASecondTicketIsCutFromTheSpecBranchHoldingTheFirstOnes(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	_, _ = requirementAccepted(h, "First", "one.txt", "1\n")
	holdTicketsStage(t, h)
	r := h.Start()
	r.WantOutput("merged into spec/" + h.SpecID())
	tip := h.Git("rev-parse", "spec/"+h.SpecID())

	id := h.ReadyTicket("Second")
	h.Script(id, factorytest.Wait())
	r.Poll()

	if got := h.GitIn(h.Worktree(id), "rev-parse", "HEAD"); got != tip {
		t.Errorf("the Ticket's branch is at %s, want the Spec Branch's tip %s", got, tip)
	}
}

func TestAnAcceptedTicketsWorktreeIsRemovedOnceItsSpecMergeIsRecorded(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.MakeRequirement()
	id := h.ReadyTicket("Add a feature")
	h.Git("worktree", "add", "-q", "-b", id, h.Worktree(id), "main")
	commit := h.GitIn(h.Worktree(id), "commit", "-q", "--allow-empty", "-m", "work")
	_ = commit
	head := h.GitIn(h.Worktree(id), "rev-parse", "HEAD")
	h.Finished(id, id, head)
	h.Accept(id)
	holdTicketsStage(t, h)

	r := h.Start()
	r.WantOutput(id + ": Accepted: removed its worktree")
	if _, err := os.Stat(h.Worktree(id)); err == nil {
		t.Error("the worktree is still there")
	}
}

func TestUnderTheBranchMethodNoSpecBranchIsMade(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.SetLandingMethod(api.SetLandingMethodInput{Method: domain.LandingBranch})
	id, _ := requirementAccepted(h, "Add a feature", "feature.txt", "feature\n")
	h.Start()
	if h.Ticket(id).State != domain.Accepted || h.Git("branch", "--list", "spec/*") != "" {
		t.Errorf("want %s left Accepted with no Spec Branch", id)
	}
}
