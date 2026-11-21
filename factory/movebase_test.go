package factory_test

import (
	"github.com/tomoram/code-central/domain"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomoram/code-central/factory/factorytest"
)

// Under Merge + Push, when the local base and the remote's each hold
// commits the other lacks, the Factory moves the local base onto the
// remote's tip and pushes (code-central-429): its own unpushed merges are
// rebuilt with the Ticket's branch untouched as the second parent, and
// the human's unpushed commits are replayed on top. A conflict, or a
// merge that isn't the Factory's, leaves the base as it was and reports
// Base diverged.

// wantAncestor fails the test unless commit is reachable from rev.
func wantAncestor(t *testing.T, h *factorytest.Harness, commit, rev string) {
	t.Helper()
	if !h.IsAncestor(commit, rev) {
		t.Errorf("%s doesn't hold %s", rev, commit)
	}
}

// subjectsAbove is the subjects on main's first-parent line above base,
// oldest first, one to a line.
func subjectsAbove(h *factorytest.Harness, base string) string {
	commits, _ := inRange(h, h.Tip("main"), base)
	var subjects []string
	for _, c := range commits {
		subjects = append(subjects, h.Subject(c))
	}
	return strings.Join(subjects, "\n")
}

// unpushedMerge starts the Factory with the remote refusing pushes, so
// its merge of id stays in the local base unpushed, and returns the
// running Factory with the remote accepting pushes again.
func unpushedMerge(t *testing.T, h *factorytest.Harness, remote *factorytest.Remote, id string) *factorytest.Running {
	t.Helper()
	remote.RefusePushes()
	r := h.Start()
	r.WantOutput(id + ": can't land yet")
	remote.AcceptPushes()
	return r
}

func TestUnderMergeAndPushAnUnpushedFactoryMergeIsRebuiltOnTheRemotesTipAndTheTicketLands(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	remote := h.AddRemote("origin")
	mergeAndPush(h, "origin")
	h.Publish(remote, h.Tip("main"), "main")
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	id, head := accepted(h, "Add a feature", "feature.txt", "feature\n")
	r := unpushedMerge(t, h, remote, id)
	oldMerge := h.Tip("main")
	h.Publish(remote, h.Tip("theirs"), "main")

	r.Poll()
	r.WantOutput("main: moved 1 unpushed commit onto origin/main")
	h.WantState(id, domain.Done)
	merge := h.Tip("main")
	if got := strings.Join(h.Parents(merge), " "); got != theirs+" "+head {
		t.Errorf("main's tip has parents %q, want the remote's %s then the Ticket's head %s", got, theirs, head)
	}
	if got := h.Subject(merge); got != "Merge "+id+": Add a feature" {
		t.Errorf("the rebuilt merge's message is %q, want the original's", got)
	}
	if got := remote.Tip("main"); got != merge {
		t.Errorf("the remote's main is at %q, want the rebuilt merge %s", got, merge)
	}
	wantAncestor(t, h, head, "main")
	if h.IsAncestor(oldMerge, "main") {
		t.Errorf("main still holds the old merge %s", oldMerge)
	}
}

func TestUnderMergeAndPushAnUnpushedHumanCommitIsReplayedOnTheRemotesTipAndPushed(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	remote := h.AddRemote("origin")
	mergeAndPush(h, "origin")
	h.Publish(remote, h.Tip("main"), "main")
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	h.Publish(remote, h.Tip("theirs"), "main")
	id, head := accepted(h, "Add a feature", "feature.txt", "feature\n")
	mine := h.CommitOnBy("main", "mine.txt", "mine\n", "Pat Human <pat@example.com>")

	r := h.Start()
	r.WantOutput("main: moved 1 unpushed commit onto origin/main")
	h.WantState(id, domain.Done)
	if got := subjectsAbove(h, theirs); got != "Change mine.txt\nMerge "+id+": Add a feature" {
		t.Errorf("main holds on top of the remote's:\n%s\nwant the human's commit replayed, then the merge", got)
	}
	replayed := h.Parents(h.Tip("main"))[0]
	if got := h.Author(replayed); got != "Pat Human <pat@example.com>" {
		t.Errorf("the replayed commit's author is %q, want the human's kept", got)
	}
	if got := strings.Join(h.Parents(replayed), " "); got != theirs {
		t.Errorf("the replayed commit's parent is %s, want the remote's %s", got, theirs)
	}
	if got := remote.Tip("main"); got != h.Tip("main") {
		t.Errorf("the remote's main is at %q, want it pushed level with the local base", got)
	}
	if h.IsAncestor(mine, "main") {
		t.Errorf("main still holds the original commit %s", mine)
	}
	wantAncestor(t, h, head, "main")
}

func TestUnderMergeAndPushAFactoryMergeAndAHumanCommitAreMovedTogetherInOrder(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	remote := h.AddRemote("origin")
	mergeAndPush(h, "origin")
	h.Publish(remote, h.Tip("main"), "main")
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	id, head := accepted(h, "Add a feature", "feature.txt", "feature\n")
	r := unpushedMerge(t, h, remote, id)
	h.CommitOn("main", "mine.txt", "mine\n")
	h.Publish(remote, h.Tip("theirs"), "main")

	r.Poll()
	r.WantOutput("main: moved 2 unpushed commits onto origin/main")
	h.WantState(id, domain.Done)
	if got := subjectsAbove(h, theirs); got != "Merge "+id+": Add a feature\nChange mine.txt" {
		t.Errorf("main holds on top of the remote's:\n%s\nwant the merge rebuilt, then the human's commit", got)
	}
	if got := strings.Join(h.Parents(h.Parents(h.Tip("main"))[0]), " "); got != theirs+" "+head {
		t.Errorf("the rebuilt merge has parents %q, want %s then the Ticket's head %s", got, theirs, head)
	}
	if got := remote.Tip("main"); got != h.Tip("main") {
		t.Errorf("the remote's main is at %q, want it pushed level with the local base", got)
	}
	wantAncestor(t, h, head, "main")
}

func TestUnderMergeAndPushAnUnpushedMergeThatIsntTheFactorysIsBaseDiverged(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.RealGit())
	remote := h.AddRemote("origin")
	mergeAndPush(h, "origin")
	h.Publish(remote, h.Tip("main"), "main")
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	h.Publish(remote, h.Tip("theirs"), "main")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	h.CommitOn("topic", "topic.txt", "topic\n")
	h.MergeByHand("topic", "main")
	before := h.Tip("main")

	r := h.Start()
	r.WantOutput("warning: can't move main onto origin/main: " + before[:12] + " is a merge the Factory didn't make")
	r.WantOutput("has diverged from the remote's")
	if got := h.Tip("main"); got != before {
		t.Errorf("main moved to %s, want it left at %s", got, before)
	}
	if got := remote.Tip("main"); got != theirs {
		t.Errorf("the remote's main is at %q, want it left at %s", got, theirs)
	}
	if tk := h.Ticket(id); tk.State != domain.Accepted {
		t.Errorf("%s is %s, want it held Accepted", id, tk.State)
	}
}

func TestUnderMergeAndPushUncommittedChangesInTheWayOfTheMoveMakeThePollWait(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	remote := h.AddRemote("origin")
	mergeAndPush(h, "origin")
	h.Publish(remote, h.Tip("main"), "main")
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	h.Publish(remote, h.Tip("theirs"), "main")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	h.CommitOn("main", "mine.txt", "mine\n")
	before := h.Tip("main")
	inTheWay := filepath.Join(h.Repo, "theirs.txt")
	if err := os.WriteFile(inTheWay, []byte("mine, uncommitted\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := h.Start()
	r.WantOutput("warning: can't move main onto origin/main yet: uncommitted changes in")
	if tk := h.Ticket(id); tk.State != domain.Accepted {
		t.Errorf("%s is %s, want it held Accepted", id, tk.State)
	}
	if got := h.Tip("main"); got != before {
		t.Errorf("main moved to %s, want it left at %s", got, before)
	}
	if _, ok := baseDivergedItem(t, h); ok {
		t.Error("uncommitted changes showed Base diverged")
	}

	if err := os.Remove(inTheWay); err != nil {
		t.Fatal(err)
	}
	r.Poll()
	h.WantState(id, domain.Done)
	wantAncestor(t, h, theirs, "main")
	if got := remote.Tip("main"); got != h.Tip("main") {
		t.Errorf("the remote's main is at %q, want it pushed level with the local base", got)
	}
}

func TestUnderMergeAndPushAnUnpushedSpecBranchMergeIsRebuiltAndItsTicketsLand(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	remote := h.AddRemote("origin")
	mergeAndPush(h, "origin")
	h.Publish(remote, h.Tip("main"), "main")
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	a, b := twoAccepted(t, h)
	remote.RefusePushes()
	r := h.Start()
	r.WantOutput(h.SpecID() + ": merged spec/" + h.SpecID() + " into main at ")
	r.WantOutput(a + ": can't land yet")
	remote.AcceptPushes()
	specTip := h.Tip("spec/" + h.SpecID())
	h.Publish(remote, h.Tip("theirs"), "main")

	r.Poll()
	r.WantOutput("main: moved 1 unpushed commit onto origin/main")
	h.WantState(a, domain.Done)
	h.WantState(b, domain.Done)
	if got := strings.Join(h.Parents(h.Tip("main")), " "); got != theirs+" "+specTip {
		t.Errorf("main's tip has parents %q, want the remote's %s then the Spec Branch's %s", got, theirs, specTip)
	}
	if got := remote.Tip("main"); got != h.Tip("main") {
		t.Errorf("the remote's main is at %q, want it pushed level with the local base", got)
	}
}
