package factory_test

import (
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// mergeAndPush sets the Merge Landing Method with Push to remote.
func mergeAndPush(h *factorytest.Harness, remote string) {
	h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingMerge, Push: true, Remote: remote})
}

// commentsSaying counts tk's comments that contain text.
func commentsSaying(tk core.TicketView, text string) int {
	n := 0
	for _, c := range tk.Comments {
		if strings.Contains(c.Body, text) {
			n++
		}
	}
	return n
}

// cantLandYet and cantPushYet open the Factory's comments on a Ticket it
// can't land, or whose branch it can't push, yet.
const (
	cantLandYet = "The Factory can't land this yet"
	cantPushYet = "The Factory can't push this Ticket's branch yet"
)

func TestUnderMergeAndPushTheBaseIsPushedBeforeTheTicketLands(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	mergeAndPush(h, "origin")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")

	r := h.Start()
	tk := h.WaitForState(id, store.Done)
	r.WaitForOutput(id + ": pushed main to origin")
	merge := h.Git("rev-parse", "main")
	if got := remote.Git("rev-parse", "main"); got != merge {
		t.Errorf("the remote's main is at %q, want the merge %s", got, merge)
	}
	if m := tk.Moves[len(tk.Moves)-1]; m.Commit != merge {
		t.Errorf("landed with %q, want the merge %s", m.Commit, merge)
	}
}

func TestUnderMergeAndPushAFailedPushIsCommentedOnceAndRetried(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	remote.Break()
	mergeAndPush(h, "origin")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")

	r := h.Start()
	r.Poll()
	tk := h.Ticket(id)
	if tk.State != store.Accepted {
		t.Fatalf("%s is %s, want it held Accepted until the push succeeds", id, tk.State)
	}
	merge := h.Git("rev-parse", "main")
	if got := h.Git("log", "-1", "--format=%s", merge); got != "Merge "+id+": Add a feature" {
		t.Errorf("main's tip is %q, want the Factory's merge", got)
	}
	if n := commentsSaying(tk, cantLandYet); n != 1 || !commented(tk, "couldn't push main to origin", "It stays Accepted") {
		t.Errorf("want one comment quoting the failed push, got %+v", tk.Comments)
	}
	r.WaitForOutput(id + ": can't land yet: couldn't push main to origin")

	remote.Restore()
	r.Poll()
	h.WaitForState(id, store.Done)
	if got := remote.Git("rev-parse", "main"); got != merge {
		t.Errorf("the remote's main is at %q, want the merge %s", got, merge)
	}
	if n := commentsSaying(h.Ticket(id), cantLandYet); n != 1 {
		t.Errorf("want still one comment, got %d", n)
	}
}

func TestUnderMergeAndPushARemoteThatIsAheadIsNeverOverwritten(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	theirs := h.CommitOn("theirs", "theirs.txt", "theirs\n")
	// Pushed by path, so the repo's origin/main doesn't know of it.
	h.Git("push", "-q", remote.Path, "theirs:refs/heads/main")
	mergeAndPush(h, "origin")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")

	r := h.Start()
	r.Poll()
	tk := h.Ticket(id)
	if tk.State != store.Accepted {
		t.Errorf("%s is %s, want it held Accepted", id, tk.State)
	}
	if got := remote.Git("rev-parse", "main"); got != theirs {
		t.Errorf("the remote's main is at %q, want it left at %s", got, theirs)
	}
	if !commented(tk, "couldn't push main to origin") {
		t.Errorf("want a comment on the rejected push, got %+v", tk.Comments)
	}
}

func TestUnderMergeAndPushAnInReviewTicketMergedByHandLandsOnceTheBaseIsPushed(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	remote.Break()
	mergeAndPush(h, "origin")
	id := h.ReadyTicket("Merged by hand")
	commit := h.CommitOn(id, "feature.txt", "feature\n")
	h.Finished(id, id, commit)
	h.Git("merge", "-q", "--no-ff", "-m", "My own merge", id)

	r := h.Start()
	r.Poll()
	tk := h.Ticket(id)
	if tk.State != store.InReview {
		t.Fatalf("%s is %s, want it held In Review until the base is pushed", id, tk.State)
	}
	if !commented(tk, "couldn't push main to origin", "It stays In Review") {
		t.Errorf("want a comment on the failed push, got %+v", tk.Comments)
	}

	remote.Restore()
	r.Poll()
	tk = h.WaitForState(id, store.Done)
	if m := tk.Moves[len(tk.Moves)-1]; m.From != store.InReview || m.Commit != commit {
		t.Errorf("land move %+v, want from In Review with the finish commit %s", m, commit)
	}
	if got, want := remote.Git("rev-parse", "main"), h.Git("rev-parse", "main"); got != want {
		t.Errorf("the remote's main is at %q, want %s", got, want)
	}
}

func TestTheFactoryRefusesToStartWhenPushNamesAMissingRemote(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	mergeAndPush(h, "upstream")
	refuses(t, h, h.Repo, `Push names the remote "upstream", which this repository doesn't have`)
	refuses(t, h, h.Repo, "Project Options")
}

func TestAPushRemoteThatGoesMissingHoldsLandingWithAWarningUntilFixed(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	mergeAndPush(h, "origin")
	r := h.Start()
	r.Poll()

	mergeAndPush(h, "nowhere")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	reviewed := h.ReadyTicket("Merged by hand")
	h.Finished(reviewed, reviewed, h.CommitOn(reviewed, "reviewed.txt", "reviewed\n"))
	h.Git("merge", "-q", "--no-ff", "-m", "My own merge", reviewed)
	base := h.Git("rev-parse", "main")
	ready := h.ReadyTicket("Still claimed")
	h.Script(ready, factorytest.Wait())

	r.Poll()
	r.Poll()
	if got := strings.Count(r.Stdout(), `warning: nothing lands until this is fixed: Push names the remote "nowhere"`); got != 1 {
		t.Errorf("want one warning over two polls, got %d in %q", got, r.Stdout())
	}
	if s := h.Ticket(id).State; s != store.Accepted {
		t.Errorf("%s is %s, want it held Accepted", id, s)
	}
	if s := h.Ticket(reviewed).State; s != store.InReview {
		t.Errorf("%s is %s, want the hand merge held In Review", reviewed, s)
	}
	if h.Git("rev-parse", "main") != base {
		t.Error("main moved while the Push remote was missing")
	}
	h.WaitForState(ready, store.InProgress)

	mergeAndPush(h, "origin")
	r.Poll()
	h.WaitForState(id, store.Done)
	h.WaitForState(reviewed, store.Done)
}

// branchAndPush sets the Branch Landing Method with Push to remote.
func branchAndPush(h *factorytest.Harness, remote string) {
	h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingBranch, Push: true, Remote: remote})
}

func TestUnderBranchAndPushTheBranchReachesTheRemoteOnAcceptAndNotWhileInReview(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	branchAndPush(h, "origin")
	id := h.ReadyTicket("Add a feature")
	commit := h.CommitOn(id, "feature.txt", "feature\n")
	h.Finished(id, id, commit)
	base := h.Git("rev-parse", "main")

	r := h.Start()
	r.Poll()
	if out := remote.Git("branch", "--list", id); out != "" {
		t.Fatalf("the remote has %s while it is In Review: %q", id, out)
	}

	h.Accept(id)
	r.Poll()
	r.WaitForOutput(id + ": pushed " + id + " to origin")
	if got := remote.Git("rev-parse", "refs/heads/"+id); got != commit {
		t.Errorf("the remote's %s is at %q, want the finish commit %s", id, got, commit)
	}
	if s := h.Ticket(id).State; s != store.Accepted {
		t.Errorf("%s is %s, want it left Accepted", id, s)
	}
	if h.Git("rev-parse", "main") != base {
		t.Error("main moved under the Branch Landing Method")
	}
	if out := remote.Git("branch", "--list", "main"); out != "" {
		t.Errorf("the base was pushed under Branch: %q", out)
	}

	r.Poll()
	if got := strings.Count(r.Stdout(), id+": pushed "); got != 1 {
		t.Errorf("want the branch pushed once, got %d in %q", got, r.Stdout())
	}

	h.Git("merge", "-q", "--no-ff", "-m", "My own merge", id)
	r.Poll()
	h.WaitForState(id, store.Done)
}

func TestUnderBranchAndPushAFailedPushIsCommentedOnceAndRetried(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	remote.Break()
	branchAndPush(h, "origin")
	id, commit := accepted(h, "Add a feature", "feature.txt", "feature\n")

	r := h.Start()
	r.Poll()
	r.Poll()
	tk := h.Ticket(id)
	if tk.State != store.Accepted {
		t.Fatalf("%s is %s, want it left Accepted", id, tk.State)
	}
	if n := commentsSaying(tk, cantPushYet); n != 1 || !commented(tk, "couldn't push "+id+" to origin", "It stays Accepted") {
		t.Errorf("want one comment quoting the failed push, got %+v", tk.Comments)
	}
	r.WaitForOutput(id + ": can't push yet: couldn't push " + id + " to origin")

	remote.Restore()
	r.Poll()
	r.WaitForOutput(id + ": pushed " + id + " to origin")
	if got := remote.Git("rev-parse", "refs/heads/"+id); got != commit {
		t.Errorf("the remote's %s is at %q, want the finish commit %s", id, got, commit)
	}
	tk = h.Ticket(id)
	if tk.State != store.Accepted || commentsSaying(tk, cantPushYet) != 1 {
		t.Errorf("%s is %s with comments %+v; want it Accepted with still one comment", id, tk.State, tk.Comments)
	}
}
