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
	tk := h.WantState(id, store.Done)
	r.WantOutput(id + ": pushed main to origin")
	merge := h.Git("rev-parse", "main")
	if got := remote.Git("rev-parse", "main"); got != merge {
		t.Errorf("the remote's main is at %q, want the merge %s", got, merge)
	}
	if m := tk.Moves[len(tk.Moves)-1]; m.Commit != merge {
		t.Errorf("landed with %q, want the merge %s", m.Commit, merge)
	}
}

func TestAFactoryWhosePushNamesAMissingRemoteStartsWithTheHomeLocalOnlyAndClaims(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	mergeAndPush(h, "upstream")
	ready := h.ReadyTicket("Still claimed")
	h.Script(ready, factorytest.Wait())

	h.Start()
	h.WantState(ready, store.InProgress)
	if home := repositoryView(t, h, h.Project); !home.LocalOnly {
		t.Fatalf("Home %+v, want it local-only: its clone has no remote upstream", home)
	}
}

func TestUnderBranchAndPushAnAcceptedTicketsBranchReachesABareRemote(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingBranch, Push: true, Remote: "origin"})
	id, commit := accepted(h, "Add a feature", "feature.txt", "feature\n")

	r := h.Start()
	r.WantOutput(id + ": pushed " + id + " to origin")
	if got := remote.Git("rev-parse", "refs/heads/"+id); got != commit {
		t.Errorf("the remote's %s is at %q, want the finish commit %s", id, got, commit)
	}
	if out := remote.Git("branch", "--list", "main"); out != "" {
		t.Errorf("the base was pushed under Branch: %q", out)
	}
}
