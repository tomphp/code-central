package factory_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// lastFailure is the newest Failure Report on a Failed Ticket.
func lastFailure(t *testing.T, h *factorytest.Harness, id string) string {
	t.Helper()
	tk := h.WaitForState(id, store.Failed)
	r := tk.FailureReports[len(tk.FailureReports)-1]
	return r.WhatBroke + "\n" + r.Why + "\n" + r.WhatToFix + "\n" + r.WhereWorkStands
}

func TestAStrayDirectoryAtTheWorktreePathIsAFailureReportAndIsLeftAlone(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
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
	h := factorytest.New(t)
	id := h.ReadyTicket("Wrong branch")
	h.Git("worktree", "add", "-q", "-b", "experiment", h.Worktree(id), "main")

	h.Start()
	report := lastFailure(t, h, id)

	if !strings.Contains(report, h.Worktree(id)) || !strings.Contains(report, "experiment") {
		t.Errorf("failure report %q doesn't name the path and the branch it's on", report)
	}
	if b := h.GitIn(h.Worktree(id), "rev-parse", "--abbrev-ref", "HEAD"); b != "experiment" {
		t.Errorf("the worktree was switched to %q", b)
	}
	if n := len(h.Calls()); n != 0 {
		t.Errorf("claude ran %d times, want none", n)
	}
}

func TestTheTicketsBranchCheckedOutElsewhereIsAFailureReport(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Busy branch")
	elsewhere := h.Repo + "-elsewhere"
	h.Git("worktree", "add", "-q", "-b", id, elsewhere, "main")

	h.Start()
	report := lastFailure(t, h, id)

	if !strings.Contains(report, "Branch "+id) || !strings.Contains(report, elsewhere) {
		t.Errorf("failure report %q doesn't name the branch and where it's checked out", report)
	}
	if _, err := os.Stat(h.Worktree(id)); !os.IsNotExist(err) {
		t.Errorf("a worktree was made at %s anyway", h.Worktree(id))
	}
	if b := h.GitIn(elsewhere, "rev-parse", "--abbrev-ref", "HEAD"); b != id {
		t.Errorf("the other worktree was switched to %q", b)
	}
}

func TestAStageWorktreePathHoldingABranchWorktreeIsAFailureReport(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.StageTicket("spec", "Spec: on a branch")
	h.Git("worktree", "add", "-q", "-b", "mine", h.Worktree(id), "main")

	h.Start()
	report := lastFailure(t, h, id)

	if !strings.Contains(report, h.Worktree(id)) || !strings.Contains(report, "mine") {
		t.Errorf("failure report %q doesn't name the path and the branch it's on", report)
	}
	if b := h.GitIn(h.Worktree(id), "rev-parse", "--abbrev-ref", "HEAD"); b != "mine" {
		t.Errorf("the worktree was switched to %q", b)
	}
}

// move moves a Ticket to state as the human.
func move(t *testing.T, h *factorytest.Harness, id string, state store.State) {
	t.Helper()
	if _, err := h.Human().Move(context.Background(), h.Project, core.MoveInput{Ticket: core.Ref(id), ToState: state}); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestWorktreesAreKeptThroughInReviewGiveUpAndFailed(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	review := h.ReadyTicket("Finished")
	gaveUp := h.ReadyTicket("Too hard")
	h.Script(gaveUp, factorytest.CLI("give-up", "{{ticket}}", "--claim", "{{claim}}", "--note", "Too hard."), factorytest.Result("success"))
	failed := h.ReadyTicket("Broken")
	h.Script(failed, factorytest.CLI("report-failure", "{{ticket}}", "--claim", "{{claim}}", "--what-broke", "x", "--why", "x", "--what-to-fix", "x", "--where-work-stands", "x"), factorytest.Result("success"))

	r := h.Start()
	h.WaitForState(review, store.InReview)
	h.WaitForState(failed, store.Failed)
	h.WaitFor("the give-up", func() bool { tk := h.Ticket(gaveUp); return len(tk.Claims) > 0 && tk.State == store.Ready })
	r.Poll()

	for _, id := range []string{review, gaveUp, failed} {
		if !exists(h.Worktree(id)) {
			t.Errorf("%s's worktree was removed while it was %s", id, h.Ticket(id).State)
		}
	}
}

func TestACleanWorktreeIsRemovedOnDoneAndItsBranchKept(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Shipped")
	h.Script(id,
		factorytest.Shell("echo x > x.txt && git add x.txt && git commit -q -m x"),
		factorytest.Finish("Done."), factorytest.Result("success"))
	r := h.Start()
	h.WaitForState(id, store.InReview)
	r.WaitForOutput(id + ": turn ended")
	tip := h.Git("rev-parse", id)

	move(t, h, id, store.Done)
	r.Poll()

	if exists(h.Worktree(id)) {
		t.Errorf("the Done Ticket's clean worktree %s is still there", h.Worktree(id))
	}
	if strings.Contains(h.Git("worktree", "list"), h.Worktree(id)) {
		t.Errorf("git still lists the worktree %s", h.Worktree(id))
	}
	if got := h.Git("rev-parse", id); got != tip {
		t.Errorf("branch %s is at %s, want it kept at %s", id, got, tip)
	}
}

func TestADirtyWorktreeIsLeftOnCancelledWithAWarning(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Abandoned")
	h.Script(id, factorytest.Shell("echo wip > wip.txt"), factorytest.Finish("Done."), factorytest.Result("success"))
	r := h.Start()
	h.WaitForState(id, store.InReview)
	r.WaitForOutput(id + ": turn ended")

	move(t, h, id, store.Cancelled)
	r.Poll()

	if got, _ := os.ReadFile(filepath.Join(h.Worktree(id), "wip.txt")); string(got) != "wip\n" {
		t.Errorf("the dirty worktree was changed: wip.txt = %q", got)
	}
	r.WaitForOutput("warning: " + id)
	if !strings.Contains(r.Stdout(), h.Worktree(id)) {
		t.Errorf("stdout %q: the warning doesn't name the worktree", r.Stdout())
	}
	if h.Git("branch", "--list", id) == "" {
		t.Errorf("branch %s was deleted", id)
	}
}

func TestTheSweepAtStartRemovesFinishedTicketsWorktreesAndPrunes(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	done := h.ReadyTicket("Done before")
	h.Git("worktree", "add", "-q", "-b", done, h.Worktree(done), "main")
	move(t, h, done, store.Done)
	stage := h.StageTicket("spec", "Spec: cancelled before")
	h.Git("worktree", "add", "-q", "--detach", h.Worktree(stage), "main")
	move(t, h, stage, store.Cancelled)
	gone := h.Repo + "-gone"
	h.Git("worktree", "add", "-q", "--detach", gone, "main")
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}

	r := h.Start()
	r.WaitForOutput("base: main")
	h.WaitFor("the sweep", func() bool { return !exists(h.Worktree(done)) && !exists(h.Worktree(stage)) })

	if list := h.Git("worktree", "list"); strings.Contains(list, gone) {
		t.Errorf("git worktree list still has the deleted %s:\n%s", gone, list)
	}
	if h.Git("branch", "--list", done) == "" {
		t.Errorf("branch %s was deleted", done)
	}
}
