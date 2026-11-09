package factory_test

import (
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// mergeAndPush sets the Merge Landing Method with Push to remote.
func mergeAndPush(h *factorytest.Harness, remote string) {
	h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingMerge, Push: true, Remote: remote})
}

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

func TestTheFactoryRefusesToStartWhenPushNamesAMissingRemote(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	mergeAndPush(h, "upstream")
	refuses(t, h, h.Repo, `Push names the remote "upstream", which this repository doesn't have`)
	refuses(t, h, h.Repo, "Project Options")
}

func TestAPushRemoteThatGoesMissingHoldsLandingButNotClaiming(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	mergeAndPush(h, "origin")
	r := h.Start()

	mergeAndPush(h, "nowhere")
	ready := h.ReadyTicket("Still claimed")
	h.Script(ready, factorytest.Wait())
	r.Poll()
	r.WaitForOutput(`warning: nothing lands until this is fixed: Push names the remote "nowhere", which this repository doesn't have`)
	h.WaitForState(ready, store.InProgress)
}

func TestUnderBranchAndPushAnAcceptedTicketsBranchReachesABareRemote(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingBranch, Push: true, Remote: "origin"})
	id, commit := accepted(h, "Add a feature", "feature.txt", "feature\n")

	r := h.Start()
	r.WaitForOutput(id + ": pushed " + id + " to origin")
	if got := remote.Git("rev-parse", "refs/heads/"+id); got != commit {
		t.Errorf("the remote's %s is at %q, want the finish commit %s", id, got, commit)
	}
	if out := remote.Git("branch", "--list", "main"); out != "" {
		t.Errorf("the base was pushed under Branch: %q", out)
	}
}
