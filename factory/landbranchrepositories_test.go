package factory_test

import (
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"

	"github.com/tomoram/code-central/factory/factorytest"
)

// The Branch Landing Method across Repositories (Spec code-central-848,
// ADR 0049): a Ticket lands in each Changed Repository once its branch
// there is merged into that Repository's base by hand, and is Done once
// merged in every one. With Push on, accept pushes its branch in each.

func TestUnderBranchHandMergingInBothRepositoriesLandsTheTicket(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	h.SetLandingMethod(branchLanding)
	apiRepo := h.AddRepository("api")
	id := inTwo(h, "Add an endpoint")
	r := h.Start()
	h.WantState(id, domain.InReview)
	h.Accept(id)
	r.Poll()
	h.WantState(id, domain.Accepted)
	r.WantOutput(id + ": Accepted under the Branch Landing Method: merge " + id + " into main in Repository api and the Factory lands it")

	// Merged in api alone: Partly landed.
	apiRepo.MergeByHand(id, "main")
	r.Poll()

	tk := h.WantState(id, domain.Accepted)
	apiWork := apiRepo.Tip(id)
	if !tk.PartlyLanded || landedIn(tk, "api") != apiWork || landedIn(tk, h.Project) != "" {
		t.Fatalf("partly landed %v %+v, want it landed in api alone, at its work %s", tk.PartlyLanded, tk.Repositories, apiWork)
	}
	r.WantOutput(id + ": merged into main by hand in Repository api")

	// Merged in the Home Repository too: Done.
	h.MergeByHand(id, "main")
	r.Poll()

	tk = h.WantState(id, domain.Done)
	if landedIn(tk, h.Project) != h.Tip(id) {
		t.Fatalf("Changed Repositories %+v, want the Home Repository landed at its work", tk.Repositories)
	}
}

func TestUnderBranchAndPushAcceptPushesTheBranchInEachChangedRepository(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	home := h.AddRemote("origin")
	apiRepo := h.AddRepository("api")
	h.SetLandingMethod(api.SetLandingMethodInput{Method: domain.LandingBranch, Push: true, Remote: "origin"})
	id := inTwo(h, "Add an endpoint")
	r := h.Start()
	h.WantState(id, domain.InReview)

	h.Accept(id)
	r.Poll()

	if got, want := home.Tip(id), h.Tip(id); got != want {
		t.Errorf("the Home Repository's remote %s is at %q, want %s", id, got, want)
	}
	if got, want := apiRepo.Remote("origin").Tip(id), apiRepo.Tip(id); got != want {
		t.Errorf("api's remote %s is at %q, want %s", id, got, want)
	}
	if out := apiRepo.Remote("origin").Tip("main"); out != apiRepo.RemoteTip("origin", "main") {
		t.Errorf("api's base was pushed under Branch: its remote's main is at %s", out)
	}
	r.WantOutput(id + ": pushed " + id + " to origin in Repository api")
	h.WantState(id, domain.Accepted)
}
