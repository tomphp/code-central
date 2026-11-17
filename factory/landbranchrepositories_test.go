package factory_test

import (
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// The Branch Landing Method across Repositories (Spec code-central-848,
// ADR 0049): a Ticket lands in each Changed Repository once its branch
// there is merged into that Repository's base by hand, and is Done once
// merged in every one. With Push on, accept pushes its branch in each.

func TestUnderBranchHandMergingInBothRepositoriesLandsTheTicket(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.SetLandingMethod(branchLanding)
	api := h.AddRepository("api")
	id := inTwo(h, "Add an endpoint")
	r := h.Start()
	h.WantState(id, store.InReview)
	h.Accept(id)
	r.Poll()
	h.WantState(id, store.Accepted)
	r.WantOutput(id + ": Accepted under the Branch Landing Method: merge " + id + " into main in Repository api and the Factory lands it")

	// Merged in api alone: Partly landed.
	h.GitIn(api.Path, "merge", "-q", "--no-ff", "-m", "Merge "+id, id)
	r.Poll()

	tk := h.WantState(id, store.Accepted)
	apiWork := h.GitIn(api.Path, "rev-parse", "refs/heads/"+id)
	if !tk.PartlyLanded || landedIn(tk, "api") != apiWork || landedIn(tk, h.Project) != "" {
		t.Fatalf("partly landed %v %+v, want it landed in api alone, at its work %s", tk.PartlyLanded, tk.Repositories, apiWork)
	}
	r.WantOutput(id + ": merged into main by hand in Repository api")

	// Merged in the Home Repository too: Done.
	h.Git("merge", "-q", "--no-ff", "-m", "Merge "+id, id)
	r.Poll()

	tk = h.WantState(id, store.Done)
	if landedIn(tk, h.Project) != h.Git("rev-parse", "refs/heads/"+id) {
		t.Fatalf("Changed Repositories %+v, want the Home Repository landed at its work", tk.Repositories)
	}
}

func TestUnderBranchAndPushAcceptPushesTheBranchInEachChangedRepository(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	home := h.AddRemote("origin")
	api := h.AddRepository("api")
	h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingBranch, Push: true, Remote: "origin"})
	id := inTwo(h, "Add an endpoint")
	r := h.Start()
	h.WantState(id, store.InReview)

	h.Accept(id)
	r.Poll()

	if got, want := home.Git("rev-parse", "refs/heads/"+id), h.Git("rev-parse", "refs/heads/"+id); got != want {
		t.Errorf("the Home Repository's remote %s is at %q, want %s", id, got, want)
	}
	if got, want := api.Remote("origin").Git("rev-parse", "refs/heads/"+id), h.GitIn(api.Path, "rev-parse", "refs/heads/"+id); got != want {
		t.Errorf("api's remote %s is at %q, want %s", id, got, want)
	}
	if out := api.Remote("origin").Git("rev-parse", "main"); out != h.GitIn(api.Path, "rev-parse", "origin/main") {
		t.Errorf("api's base was pushed under Branch: its remote's main is at %s", out)
	}
	r.WantOutput(id + ": pushed " + id + " to origin in Repository api")
	h.WantState(id, store.Accepted)
}
