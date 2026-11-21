package factory_test

import (
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"

	"github.com/tomoram/code-central/factory/factorytest"
)

// mergeAndPush sets the Merge Landing Method with Push to remote.
func mergeAndPush(h *factorytest.Harness, remote string) {
	h.SetLandingMethod(api.SetLandingMethodInput{Method: domain.LandingMerge, Push: true, Remote: remote})
}

func TestUnderMergeAndPushTheBaseIsPushedBeforeTheTicketLands(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	remote := h.AddRemote("origin")
	mergeAndPush(h, "origin")
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")

	r := h.Start()
	tk := h.WantState(id, domain.Done)
	r.WantOutput(id + ": pushed main to origin")
	merge := h.Tip("main")
	if got := remote.Tip("main"); got != merge {
		t.Errorf("the remote's main is at %q, want the merge %s", got, merge)
	}
	if m := tk.Moves[len(tk.Moves)-1]; m.Commit != merge {
		t.Errorf("landed with %q, want the merge %s", m.Commit, merge)
	}
}

func TestAFactoryWhosePushNamesAMissingRemoteStartsWithTheHomeLocalOnlyAndClaims(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	h.AddRemote("origin")
	mergeAndPush(h, "upstream")
	ready := h.ReadyTicket("Still claimed")
	h.Script(ready, factorytest.Wait())

	h.Start()
	h.WantState(ready, domain.InProgress)
	if home := repositoryView(t, h, h.Project); !home.LocalOnly {
		t.Fatalf("Home %+v, want it local-only: its clone has no remote upstream", home)
	}
}

func TestUnderBranchAndPushAnAcceptedTicketsBranchReachesABareRemote(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	remote := h.AddRemote("origin")
	h.SetLandingMethod(api.SetLandingMethodInput{Method: domain.LandingBranch, Push: true, Remote: "origin"})
	id, commit := accepted(h, "Add a feature", "feature.txt", "feature\n")

	r := h.Start()
	r.WantOutput(id + ": pushed " + id + " to origin")
	if got := remote.Tip(id); got != commit {
		t.Errorf("the remote's %s is at %q, want the finish commit %s", id, got, commit)
	}
	if remote.Exists("main") {
		t.Errorf("the base was pushed under Branch")
	}
}
