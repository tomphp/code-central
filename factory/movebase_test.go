package factory_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// Under Merge + Push, when the local base and the remote's each hold
// commits the other lacks, the Factory moves the local base onto the
// remote's tip and pushes (code-central-429): its own unpushed merges are
// rebuilt with the Ticket's branch untouched as the second parent, and
// the human's unpushed commits are replayed on top. A conflict, or a
// merge that isn't the Factory's, leaves the base as it was and reports
// Base diverged.

// unpushedMerge starts the Factory with the remote refusing pushes, so
// its merge of id stays in the local base unpushed, and returns the
// running Factory with the remote accepting pushes again.
func unpushedMerge(t *testing.T, h *factorytest.Harness, remote *factorytest.Remote, id string) *factorytest.Running {
	t.Helper()
	remote.RefusePushes()
	r := h.Start()
	r.WaitForOutput(id + ": can't land yet")
	remote.AcceptPushes()
	return r
}

func TestUnderMergeAndPushAnUnpushedFactoryMergeIsRebuiltOnTheRemotesTipAndTheTicketLands(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	mergeAndPush(h, "origin")
	h.Git("push", "-q", remote.Path, "main:refs/heads/main")
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	id, head := accepted(h, "Add a feature", "feature.txt", "feature\n")
	r := unpushedMerge(t, h, remote, id)
	oldMerge := h.Git("rev-parse", "main")
	h.Git("push", "-q", remote.Path, "theirs:refs/heads/main")

	r.Poll()
	r.WaitForOutput("main: moved 1 unpushed commit onto origin/main")
	h.WaitForState(id, store.Done)
	merge := h.Git("rev-parse", "main")
	if got := h.Git("log", "-1", "--format=%P", merge); got != theirs+" "+head {
		t.Errorf("main's tip has parents %q, want the remote's %s then the Ticket's head %s", got, theirs, head)
	}
	if got := h.Git("log", "-1", "--format=%s", merge); got != "Merge "+id+": Add a feature" {
		t.Errorf("the rebuilt merge's message is %q, want the original's", got)
	}
	if got := remote.Git("rev-parse", "main"); got != merge {
		t.Errorf("the remote's main is at %q, want the rebuilt merge %s", got, merge)
	}
	h.Git("merge-base", "--is-ancestor", head, "main")
	if out := h.Git("branch", "--contains", oldMerge); strings.Contains(out, "main") {
		t.Errorf("main still holds the old merge %s", oldMerge)
	}
}

func TestUnderMergeAndPushAnUnpushedHumanCommitIsReplayedOnTheRemotesTipAndPushed(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	mergeAndPush(h, "origin")
	h.Git("push", "-q", remote.Path, "main:refs/heads/main")
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	h.Git("push", "-q", remote.Path, "theirs:refs/heads/main")
	id, head := accepted(h, "Add a feature", "feature.txt", "feature\n")
	mine := h.CommitOn("main", "mine.txt", "mine\n")
	h.Git("commit", "-q", "--amend", "--no-edit", "--author=Pat Human <pat@example.com>")
	mine = h.Git("rev-parse", "main")

	r := h.Start()
	r.WaitForOutput("main: moved 1 unpushed commit onto origin/main")
	h.WaitForState(id, store.Done)
	if got := h.Git("log", "--first-parent", "--reverse", "--format=%s", theirs+"..main"); got != "Change mine.txt\nMerge "+id+": Add a feature" {
		t.Errorf("main holds on top of the remote's:\n%s\nwant the human's commit replayed, then the merge", got)
	}
	replayed := h.Git("rev-parse", "main~1")
	if got := h.Git("log", "-1", "--format=%an <%ae>", replayed); got != "Pat Human <pat@example.com>" {
		t.Errorf("the replayed commit's author is %q, want the human's kept", got)
	}
	if got := h.Git("log", "-1", "--format=%P", replayed); got != theirs {
		t.Errorf("the replayed commit's parent is %s, want the remote's %s", got, theirs)
	}
	if got := remote.Git("rev-parse", "main"); got != h.Git("rev-parse", "main") {
		t.Errorf("the remote's main is at %q, want it pushed level with the local base", got)
	}
	if out := h.Git("branch", "--contains", mine); strings.Contains(out, "main") {
		t.Errorf("main still holds the original commit %s", mine)
	}
	h.Git("merge-base", "--is-ancestor", head, "main")
}

func TestUnderMergeAndPushAFactoryMergeAndAHumanCommitAreMovedTogetherInOrder(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	mergeAndPush(h, "origin")
	h.Git("push", "-q", remote.Path, "main:refs/heads/main")
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	id, head := accepted(h, "Add a feature", "feature.txt", "feature\n")
	r := unpushedMerge(t, h, remote, id)
	h.CommitOn("main", "mine.txt", "mine\n")
	h.Git("push", "-q", remote.Path, "theirs:refs/heads/main")

	r.Poll()
	r.WaitForOutput("main: moved 2 unpushed commits onto origin/main")
	h.WaitForState(id, store.Done)
	if got := h.Git("log", "--first-parent", "--reverse", "--format=%s", theirs+"..main"); got != "Merge "+id+": Add a feature\nChange mine.txt" {
		t.Errorf("main holds on top of the remote's:\n%s\nwant the merge rebuilt, then the human's commit", got)
	}
	if got := h.Git("log", "-1", "--format=%P", "main~1"); got != theirs+" "+head {
		t.Errorf("the rebuilt merge has parents %q, want %s then the Ticket's head %s", got, theirs, head)
	}
	if got := remote.Git("rev-parse", "main"); got != h.Git("rev-parse", "main") {
		t.Errorf("the remote's main is at %q, want it pushed level with the local base", got)
	}
	h.Git("merge-base", "--is-ancestor", head, "main")
}

func TestUnderMergeAndPushAnUnpushedMergeThatIsntTheFactorysIsBaseDiverged(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	mergeAndPush(h, "origin")
	h.Git("push", "-q", remote.Path, "main:refs/heads/main")
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	h.Git("push", "-q", remote.Path, "theirs:refs/heads/main")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	h.CommitOn("topic", "topic.txt", "topic\n")
	h.Git("merge", "-q", "--no-ff", "-m", "Merge topic by hand", "topic")
	before := h.Git("rev-parse", "main")

	r := h.Start()
	r.WaitForOutput("warning: can't move main onto origin/main: " + before[:12] + " is a merge the Factory didn't make")
	r.WaitForOutput("has diverged from the remote's")
	if got := h.Git("rev-parse", "main"); got != before {
		t.Errorf("main moved to %s, want it left at %s", got, before)
	}
	if got := remote.Git("rev-parse", "main"); got != theirs {
		t.Errorf("the remote's main is at %q, want it left at %s", got, theirs)
	}
	if tk := h.Ticket(id); tk.State != store.Accepted {
		t.Errorf("%s is %s, want it held Accepted", id, tk.State)
	}
}

func TestUnderMergeAndPushUncommittedChangesInTheWayOfTheMoveMakeThePollWait(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	mergeAndPush(h, "origin")
	h.Git("push", "-q", remote.Path, "main:refs/heads/main")
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	h.Git("push", "-q", remote.Path, "theirs:refs/heads/main")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	h.CommitOn("main", "mine.txt", "mine\n")
	before := h.Git("rev-parse", "main")
	inTheWay := filepath.Join(h.Repo, "theirs.txt")
	if err := os.WriteFile(inTheWay, []byte("mine, uncommitted\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := h.Start()
	r.WaitForOutput("warning: can't move main onto origin/main yet: uncommitted changes in")
	if tk := h.Ticket(id); tk.State != store.Accepted {
		t.Errorf("%s is %s, want it held Accepted", id, tk.State)
	}
	if got := h.Git("rev-parse", "main"); got != before {
		t.Errorf("main moved to %s, want it left at %s", got, before)
	}
	if _, ok := baseDivergedItem(t, h); ok {
		t.Error("uncommitted changes showed Base diverged")
	}

	if err := os.Remove(inTheWay); err != nil {
		t.Fatal(err)
	}
	r.Poll()
	h.WaitForState(id, store.Done)
	h.Git("merge-base", "--is-ancestor", theirs, "main")
	if got := remote.Git("rev-parse", "main"); got != h.Git("rev-parse", "main") {
		t.Errorf("the remote's main is at %q, want it pushed level with the local base", got)
	}
}

func TestUnderMergeAndPushAnUnpushedSpecBranchMergeIsRebuiltAndItsTicketsLand(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	mergeAndPush(h, "origin")
	h.Git("push", "-q", remote.Path, "main:refs/heads/main")
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	a, b := twoAccepted(t, h)
	remote.RefusePushes()
	r := h.Start()
	r.WaitForOutput(h.SpecID() + ": merged spec/" + h.SpecID() + " into main at ")
	r.WaitForOutput(a + ": can't land yet")
	remote.AcceptPushes()
	specTip := h.Git("rev-parse", "spec/"+h.SpecID())
	h.Git("push", "-q", remote.Path, "theirs:refs/heads/main")

	r.Poll()
	r.WaitForOutput("main: moved 1 unpushed commit onto origin/main")
	h.WaitForState(a, store.Done)
	h.WaitForState(b, store.Done)
	if got := h.Git("log", "-1", "--format=%P", "main"); got != theirs+" "+specTip {
		t.Errorf("main's tip has parents %q, want the remote's %s then the Spec Branch's %s", got, theirs, specTip)
	}
	if got := remote.Git("rev-parse", "main"); got != h.Git("rev-parse", "main") {
		t.Errorf("the remote's main is at %q, want it pushed level with the local base", got)
	}
}
