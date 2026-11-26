package factory_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"

	"github.com/tomoram/code-central/factory/factorytest"
)

// lastFailure is the newest Failure Report on a Failed Ticket.
func lastFailure(t *testing.T, h *factorytest.Rig, id string) string {
	t.Helper()
	tk := h.WantState(id, domain.Failed)
	r := tk.FailureReports[len(tk.FailureReports)-1]
	return r.WhatBroke + "\n" + r.Why + "\n" + r.WhatToFix + "\n" + r.WhereWorkStands
}

func TestAStrayDirectoryAtTheWorktreePathIsAFailureReportAndIsLeftAlone(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("Something's in the way")
	stray := filepath.Join(h.Worktree(id), "mine.txt")
	if err := os.MkdirAll(h.Worktree(id), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stray, []byte("not the Factory's"), 0o644); err != nil {
		t.Fatal(err)
	}

	h.Start()
	report := lastFailure(t, h, id)

	if !strings.Contains(report, h.Worktree(id)) {
		t.Errorf("failure report %q doesn't name the path", report)
	}
	if n := len(h.Calls()); n != 0 {
		t.Errorf("claude ran %d times, want none", n)
	}
	if got, _ := os.ReadFile(stray); string(got) != "not the Factory's" {
		t.Errorf("the stray directory was changed: mine.txt = %q", got)
	}
}

func TestAWorktreeOnAnotherBranchAtTheWorktreePathIsAFailureReportAndIsLeftAlone(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("Wrong branch")
	h.AddWorktreeOnNewBranch(h.Worktree(id), "experiment", "main")

	h.Start()
	report := lastFailure(t, h, id)

	if !strings.Contains(report, h.Worktree(id)) || !strings.Contains(report, "experiment") {
		t.Errorf("failure report %q doesn't name the path and the branch it's on", report)
	}
	if b := h.BranchOf(h.Worktree(id)); b != "experiment" {
		t.Errorf("the worktree was switched to %q", b)
	}
	if n := len(h.Calls()); n != 0 {
		t.Errorf("claude ran %d times, want none", n)
	}
}

func TestTheTicketsBranchCheckedOutElsewhereIsAFailureReport(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("Busy branch")
	elsewhere := h.Repo + "-elsewhere"
	h.AddWorktreeOnNewBranch(elsewhere, id, "main")

	h.Start()
	report := lastFailure(t, h, id)

	if !strings.Contains(report, "Branch "+id) || !strings.Contains(report, elsewhere) {
		t.Errorf("failure report %q doesn't name the branch and where it's checked out", report)
	}
	if _, err := os.Stat(h.Worktree(id)); !os.IsNotExist(err) {
		t.Errorf("a worktree was made at %s anyway", h.Worktree(id))
	}
	if b := h.BranchOf(elsewhere); b != id {
		t.Errorf("the other worktree was switched to %q", b)
	}
}

func TestAStageWorktreePathHoldingABranchWorktreeIsAFailureReport(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.StageTicket("spec", "Spec: on a branch")
	h.AddWorktreeOnNewBranch(h.Worktree(id), "mine", "main")

	h.Start()
	report := lastFailure(t, h, id)

	if !strings.Contains(report, h.Worktree(id)) || !strings.Contains(report, "mine") {
		t.Errorf("failure report %q doesn't name the path and the branch it's on", report)
	}
	if b := h.BranchOf(h.Worktree(id)); b != "mine" {
		t.Errorf("the worktree was switched to %q", b)
	}
}

// move moves a Ticket to state as the human.
func move(t *testing.T, h *factorytest.Rig, id string, state domain.State) {
	t.Helper()
	if _, err := h.Human().Move(context.Background(), h.Project, api.MoveInput{Ticket: api.Ref(id), ToState: state}); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestWorktreesAreKeptThroughInReviewGiveUpAndFailed(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	review := h.ReadyTicket("Finished")
	gaveUp := h.ReadyTicket("Too hard")
	h.Script(gaveUp, factorytest.CLI("give-up", "{{ticket}}", "--claim", "{{claim}}", "--note", "Too hard."), factorytest.Result("success"))
	failed := h.ReadyTicket("Broken")
	h.Script(failed, factorytest.CLI("report-failure", "{{ticket}}", "--claim", "{{claim}}", "--what-broke", "x", "--why", "x", "--what-to-fix", "x", "--where-work-stands", "x"), factorytest.Result("success"))

	r := h.Start()
	h.WantState(review, domain.InReview)
	h.WantState(failed, domain.Failed)
	if tk := h.WantState(gaveUp, domain.Ready); len(tk.Claims) == 0 {
		t.Fatalf("%s is Ready but was never claimed, want its turn to have given up", gaveUp)
	}
	r.Poll()

	for _, id := range []string{review, gaveUp, failed} {
		if !exists(h.Worktree(id)) {
			t.Errorf("%s's worktree was removed while it was %s", id, h.Ticket(id).State)
		}
	}
}

func TestACleanWorktreeIsRemovedOnDoneAndItsBranchKept(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("Shipped")
	h.Script(id,
		factorytest.WriteFile("x.txt", "x\n"),
		factorytest.Git("add", "x.txt"),
		factorytest.Git("commit", "-q", "-m", "x"),
		factorytest.Finish("Done."), factorytest.Result("success"))
	r := h.Start()
	h.WantState(id, domain.InReview)
	r.WantOutput(id + " turn ended")
	tip := h.Tip(id)

	move(t, h, id, domain.Done)
	r.Poll()

	if exists(h.Worktree(id)) {
		t.Errorf("the Done Ticket's clean worktree %s is still there", h.Worktree(id))
	}
	if h.HasWorktree(h.Worktree(id)) {
		t.Errorf("git still lists the worktree %s", h.Worktree(id))
	}
	if got := h.Tip(id); got != tip {
		t.Errorf("branch %s is at %s, want it kept at %s", id, got, tip)
	}
}

func TestADirtyWorktreeIsLeftOnCancelledWithAWarning(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("Abandoned")
	h.Script(id, factorytest.WriteFile("wip.txt", "wip\n"), factorytest.Finish("Done."), factorytest.Result("success"))
	r := h.Start()
	h.WantState(id, domain.InReview)
	r.WantOutput(id + " turn ended")

	move(t, h, id, domain.Cancelled)
	r.Poll()

	if got, _ := os.ReadFile(filepath.Join(h.Worktree(id), "wip.txt")); string(got) != "wip\n" {
		t.Errorf("the dirty worktree was changed: wip.txt = %q", got)
	}
	r.WantOutput("warning: " + id)
	if !strings.Contains(r.Stdout(), h.Worktree(id)) {
		t.Errorf("stdout %q: the warning doesn't name the worktree", r.Stdout())
	}
	if !h.Exists(id) {
		t.Errorf("branch %s was deleted", id)
	}
}

func TestTheSweepAtStartRemovesFinishedTicketsWorktreesAndPrunes(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	done := h.ReadyTicket("Done before")
	h.AddWorktreeOnNewBranch(h.Worktree(done), done, "main")
	move(t, h, done, domain.Done)
	stage := h.StageTicket("spec", "Spec: cancelled before")
	h.AddDetachedWorktreeAt(h.Worktree(stage), "main")
	move(t, h, stage, domain.Cancelled)
	gone := h.Repo + "-gone"
	h.AddDetachedWorktreeAt(gone, "main")
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}

	r := h.Start()
	r.WantOutput("base: main")
	for _, id := range []string{done, stage} {
		if exists(h.Worktree(id)) {
			t.Errorf("the sweep left %s's worktree %s", id, h.Worktree(id))
		}
	}

	if h.HasWorktree(gone) {
		t.Errorf("git worktree list still has the deleted %s", gone)
	}
	if !h.Exists(done) {
		t.Errorf("branch %s was deleted", done)
	}
}

func TestAGitFailureSettingUpTheWorktreeIsAFailureReport(t *testing.T) {
	t.Parallel()
	// Real git: only it has a ref lock for the Fake to stand in for.
	h := factorytest.New(t, factorytest.RealGit())
	id := h.ReadyTicket("git says no")
	// A stale ref lock makes `git worktree add -b` fail to make the branch.
	lock := filepath.Join(h.Git("rev-parse", "--absolute-git-dir"), "refs", "heads", id+".lock")
	if err := os.WriteFile(lock, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	h.Start()
	report := lastFailure(t, h, id)

	if !strings.Contains(report, "worktree add") || !strings.Contains(report, h.Worktree(id)) {
		t.Errorf("failure report %q doesn't name the git command that failed and the path", report)
	}
	if n := len(h.Calls()); n != 0 {
		t.Errorf("claude ran %d times, want none", n)
	}
}
