package factory_test

import (
	"context"
	"github.com/tomoram/code-central/domain"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
)

// Landing in a local-only Repository (Spec code-central-879, ADR 0039):
// its Landing Policy is the Project's with Push off, so under Merge +
// Push and Branch + Push the Factory pushes, fetches and levels nothing
// there, and a Ticket lands there once merged into its base, with no CI
// gate.

func TestUnderMergeAndPushTheHomeIsPushedAndALocalOnlyRepositoryLandsOnMerge(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	home := h.AddRemote("origin")
	api := h.AddLocalRepository("api")
	mergeAndPush(h, "origin")
	id := inTwo(h, "Add an endpoint")
	r := h.Start()
	h.WantState(id, domain.InReview)

	h.Accept(id)
	r.Poll()

	tk := h.WantState(id, domain.Done)
	homeMerge, apiMerge := h.Git("rev-parse", "main"), h.GitIn(api.Path, "rev-parse", "main")
	if got := home.Git("rev-parse", "main"); got != homeMerge {
		t.Errorf("the Home Repository's remote main is at %s, want its merge %s pushed", got, homeMerge)
	}
	if landedIn(tk, "api") != apiMerge || landedIn(tk, h.Project) != homeMerge {
		t.Fatalf("Changed Repositories %+v, want api landed at its merge %s and the Home Repository at its pushed merge %s", tk.Repositories, apiMerge, homeMerge)
	}
	if out := r.Stdout(); strings.Contains(out, "origin in Repository api") {
		t.Errorf("stdout %q, want nothing pushed or fetched in api", out)
	}
	r.WantOutput(id + ": pushed main to origin\n")
	r.WantOutput(id + ": landed on main at " + apiMerge[:12] + " in Repository api\n")
}

func TestUnderMergeAndPushALocalOnlyRepositorysOwnRemoteIsNeverFetchedPushedOrLevelled(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	api := h.AddLocalRepository("api")
	other := addUnrelatedOrigin(h, api)
	theirs := other.Remote("origin").Git("rev-parse", "main")
	mergeAndPush(h, "origin")
	id := inAPI(h, "Add an endpoint")
	r := h.Start()
	h.WantState(id, domain.InReview)

	h.Accept(id)
	r.Poll()

	tk := h.WantState(id, domain.Done)
	if merge := h.GitIn(api.Path, "rev-parse", "main"); landedIn(tk, "api") != merge {
		t.Fatalf("Changed Repositories %+v, want api landed at its merge %s", tk.Repositories, merge)
	}
	if got := h.GitIn(api.Path, "for-each-ref", "refs/remotes"); got != "" {
		t.Errorf("api's remote-tracking refs are %q, want its origin never fetched", got)
	}
	if got := other.Remote("origin").Git("rev-parse", "main"); got != theirs {
		t.Errorf("api's origin's main moved to %s, want nothing pushed to it", got)
	}
	if rows := h.NeedsMeItems(core.ReasonBaseDiverged); len(rows) != 0 {
		t.Errorf("Base diverged items %+v, want none", rows)
	}
}

func TestALocalOnlyRepositorysMissingBaseIsNotLookedForOnItsOwnRemote(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	api := h.AddLocalRepository("api")
	other := addUnrelatedOrigin(h, api)
	// release is on api's origin, which isn't the Repository's, and not
	// in api itself.
	release := "release"
	other.Remote("origin").Git("branch", release, "main")
	if _, err := h.Human().EditRepository(context.Background(), h.Project, core.EditRepositoryInput{Name: "api", Base: &release}); err != nil {
		t.Fatal(err)
	}

	r := h.Start()
	r.WantOutput("warning: the base, release, doesn't exist in Repository api, which is local-only")
	r.Poll()

	rows := h.NeedsMeItems(core.ReasonBaseMissing)
	if len(rows) != 1 || rows[0].BaseMissing.Repository != "api" || rows[0].BaseMissing.Base != release {
		t.Fatalf("Base missing items %+v, want one naming api's release", rows)
	}
	if got := h.GitIn(api.Path, "for-each-ref", "refs/remotes", "refs/heads/"+release); got != "" {
		t.Errorf("api holds %q, want its origin never fetched and no release made from it", got)
	}
}

func TestUnderTheCIGateATicketChangingOnlyALocalOnlyRepositoryLandsOnMergeWithNoPushedCommit(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	api := h.AddLocalRepository("api")
	mergeAndPush(h, "origin")
	trackCI(t, h)
	// Its CI chosen by hand still gates nothing: nothing is pushed there.
	watchCIIn(t, h, "api", "acme/api")
	id := inAPI(h, "Add an endpoint")
	r := h.Start()
	h.WantState(id, domain.InReview)

	h.Accept(id)
	r.Poll()

	tk := h.WantState(id, domain.Done)
	if merge := h.GitIn(api.Path, "rev-parse", "main"); landedIn(tk, "api") != merge {
		t.Fatalf("Changed Repositories %+v, want api landed at its merge %s", tk.Repositories, merge)
	}
	for _, v := range tk.Repositories {
		if v.PushedCommit != nil || v.CIResult != "" {
			t.Errorf("%s's pushed commit %+v and CI result %q, want none, as under Merge without Push", v.Name, v.PushedCommit, v.CIResult)
		}
	}
	if tk.CIResult != "" {
		t.Errorf("CI result %q, want none", tk.CIResult)
	}
	if out := r.Stdout(); strings.Contains(out, "CI tracks") || strings.Contains(out, "waiting on CI") {
		t.Errorf("stdout %q, want no CI tracked or waited on", out)
	}
}

func TestUnderBranchAndPushAcceptPushesTheBranchOnlyWhereThereIsACloneURL(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	home := h.AddRemote("origin")
	api := h.AddLocalRepository("api")
	h.SetLandingMethod(core.SetLandingMethodInput{Method: domain.LandingBranch, Push: true, Remote: "origin"})
	id := inTwo(h, "Add an endpoint")
	r := h.Start()
	h.WantState(id, domain.InReview)

	h.Accept(id)
	r.Poll()

	tk := h.WantState(id, domain.Accepted)
	if got, want := home.Git("rev-parse", "refs/heads/"+id), h.Git("rev-parse", "refs/heads/"+id); got != want {
		t.Errorf("the Home Repository's remote %s is at %q, want %s", id, got, want)
	}
	if commented(tk, "can't push") {
		t.Fatalf("comments %+v, want no push tried in api", tk.Comments)
	}
	r.WantOutput(id + ": pushed " + id + " to origin\n")
	r.WantOutput(id + ": Accepted under the Branch Landing Method: merge " + id + " into main in Repository api and the Factory lands it")

	h.GitIn(api.Path, "merge", "-q", "--no-ff", "-m", "Merge "+id, id)
	h.Git("merge", "-q", "--no-ff", "-m", "Merge "+id, id)
	r.Poll()
	h.WantState(id, domain.Done)
}

func TestATicketWaitingOnCIInARepositoryWhoseCloneURLIsClearedLandsThereWithNoGate(t *testing.T) {
	t.Parallel()
	h, api, r, id := gatedInTwo(t, false)
	tk := h.WantState(id, domain.Accepted)
	apiPushed := pushedIn(tk, "api")
	if apiPushed == "" || landedIn(tk, "api") != "" {
		t.Fatalf("Changed Repositories %+v, want api pushed and waiting on its CI", tk.Repositories)
	}
	// The human stops pushing api, keeping its clone, and then fetches a
	// later tip of its old remote's main into it by hand.
	path, none := api.Path, ""
	if _, err := h.Human().EditRepository(context.Background(), h.Project, core.EditRepositoryInput{Name: "api", CloneURL: &none, LocalPath: &path}); err != nil {
		t.Fatal(err)
	}
	tip := pushElsewhere(t, h, api, "other.go", "package api\n")
	h.GitIn(api.Path, "fetch", "-q", "origin")

	r.Poll()

	tk = h.WantState(id, domain.Accepted)
	if landedIn(tk, "api") == "" || tk.Repositories[0].Landed != nil {
		t.Fatalf("Changed Repositories %+v, want api landed with no CI gate, and the Home Repository still waiting on its own CI", tk.Repositories)
	}
	if got := pushedIn(tk, "api"); got != apiPushed {
		t.Errorf("api's pushed commit is %s, want it left at %s: nothing is pushed in a local-only Repository", got, apiPushed)
	}
	if strings.Contains(r.Stdout(), "CI tracks main at "+tip[:12]) {
		t.Errorf("stdout %q, want api's later tip %s never reported", r.Stdout(), tip)
	}
}

func TestUnderLandPerSpecALocalOnlyRepositorysSpecBranchMergeLandsOnMerge(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	home := h.AddRemote("origin")
	h.MakeRequirement()
	api := h.AddLocalRepository("api")
	id := inTwo(h, "Add an endpoint")
	mergeAndPush(h, "origin")
	r := h.Start()
	h.WantState(id, domain.InReview)

	h.Accept(id)
	r.Poll()

	tk := h.WantState(id, domain.Done)
	homeMerge, apiMerge := h.Git("rev-parse", "main"), h.GitIn(api.Path, "rev-parse", "main")
	want := "Merge spec " + h.SpecID() + ": Work"
	if got := subject(h, api.Path, apiMerge); got != want {
		t.Errorf("api's merge is %q, want %q", got, want)
	}
	if got := home.Git("rev-parse", "main"); got != homeMerge {
		t.Errorf("the Home Repository's remote main is at %s, want its Spec merge %s pushed", got, homeMerge)
	}
	if landedIn(tk, "api") != apiMerge || landedIn(tk, h.Project) != homeMerge {
		t.Fatalf("Changed Repositories %+v, want each landed at its Spec merge", tk.Repositories)
	}
}

func TestUnderMergeAndPushAHandMergeInALocalOnlyRepositoryLandsWithNoPush(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	api := h.AddLocalRepository("api")
	mergeAndPush(h, "origin")
	id := inAPI(h, "Add an endpoint")
	r := h.Start()
	h.WantState(id, domain.InReview)
	h.GitIn(api.Path, "merge", "-q", "--no-ff", "-m", "Merge "+id, id)

	r.Poll()

	tk := h.WantState(id, domain.Done)
	if work := h.GitIn(api.Path, "rev-parse", "refs/heads/"+id); landedIn(tk, "api") != work {
		t.Fatalf("Changed Repositories %+v, want api landed at its work %s", tk.Repositories, work)
	}
	r.WantOutput(id + ": merged into main by hand in Repository api")
}

func TestARepositoryWhoseCloneURLIsClearedWhileItsBaseHasDivergedNoLongerHoldsBackClaims(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	api := h.AddRepository("api")
	mergeAndPush(h, "origin")
	r := h.Start()
	// api's remote and its clone's main each gain a commit the other
	// lacks, and they conflict.
	api.CommitOn("main", "README.md", "ours\n")
	pushElsewhere(t, h, api, "README.md", "theirs\n")
	r.Poll()
	r.WantOutput("warning: the base, main, in Repository api has diverged from the remote's")
	ready := h.ReadyTicket("Waits for the base")
	h.Script(ready, factorytest.Wait())
	r.Poll()
	if tk := h.Ticket(ready); tk.State != domain.Ready {
		t.Fatalf("%s is %s, want it not claimed while api's base has diverged", ready, tk.State)
	}

	// The human stops pushing api: local-only, its base is never levelled.
	path, none := api.Path, ""
	if _, err := h.Human().EditRepository(context.Background(), h.Project, core.EditRepositoryInput{Name: "api", CloneURL: &none, LocalPath: &path}); err != nil {
		t.Fatal(err)
	}
	r.Poll()

	h.WantState(ready, domain.InProgress)
	if rows := h.NeedsMeItems(core.ReasonBaseDiverged); len(rows) != 0 {
		t.Errorf("Base diverged items %+v, want none once api is local-only", rows)
	}
}
