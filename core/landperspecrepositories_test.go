package core_test

import (
	"slices"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// Land per Spec across Repositories (Spec code-central-848, ADR 0049):
// one Spec merge Ticket per Spec names each conflicting Repository with
// its files and both tips.

// specMergeInTwo is specMergeWorld with the Repository api, after the
// Home Repository in the landing order.
func specMergeInTwo(t *testing.T) fixture {
	t.Helper()
	f := specMergeWorld(t)
	f.addRepository(t, api.AddRepositoryInput{Name: "api", CloneURL: "git@github.com:acme/api.git", Base: "trunk"})
	return f
}

func TestOpenSpecMergeNamesEachConflictingRepositoryInLandingOrder(t *testing.T) {
	f := specMergeInTwo(t)
	if _, err := f.core.SetLandingOrder(human, "acme", api.SetLandingOrderInput{Names: []string{"api", "acme"}}); err != nil {
		t.Fatal(err)
	}

	res, err := f.core.OpenSpecMerge(ctx, "acme", api.OpenSpecMergeInput{Spec: "acme-1", Repositories: []api.SpecMergeConflict{
		{Name: "acme", Conflicts: []string{"README"}, BaseTip: "aaa111", SpecBranchTip: "bbb222"},
		{Name: "api", Conflicts: []string{"endpoint.go", "go.mod"}, BaseTip: "ccc333", SpecBranchTip: "ddd444"},
	}})
	if err != nil {
		t.Fatalf("open_spec_merge: %v", err)
	}

	if res != (api.OpenSpecMergeResult{Ticket: "acme-3", Outcome: api.SpecMergeOpened}) {
		t.Fatalf("result %+v, want acme-3 opened", res)
	}
	want := "The Spec Branch spec/acme-1 conflicts with the base in these Repositories:\n\n" +
		"- api: merging spec/acme-1 at ddd444 into trunk at ccc333 conflicts in these files:\n  - endpoint.go\n  - go.mod\n" +
		"- acme: merging spec/acme-1 at bbb222 into main at aaa111 conflicts in these files:\n  - README\n\n" +
		"In each, merge the base into this branch and resolve the conflicts.\n"
	if tk := f.getTicket(t, "acme-3"); tk.Description != want || !tk.SpecMergeTicket {
		t.Errorf("opened %+v, want it marked, describing\n%s", tk, want)
	}
}

func TestOpenSpecMergeTakesTheHomeRepositorysConflictInEitherForm(t *testing.T) {
	f := specMergeInTwo(t)
	apiRepo := api.SpecMergeConflict{Name: "api", Conflicts: []string{"endpoint.go"}, BaseTip: "ccc333", SpecBranchTip: "ddd444"}
	f.core.OpenSpecMerge(ctx, "acme", api.OpenSpecMergeInput{Spec: "acme-1", Conflicts: []string{"README"}, BaseTip: "aaa111", SpecBranchTip: "bbb222", Repositories: []api.SpecMergeConflict{apiRepo}})

	// The same conflicts, the Home Repository's named in repositories.
	res, err := f.core.OpenSpecMerge(ctx, "acme", api.OpenSpecMergeInput{Spec: "acme-1", Repositories: []api.SpecMergeConflict{
		{Name: "acme", Conflicts: []string{"README"}, BaseTip: "aaa111", SpecBranchTip: "bbb222"}, apiRepo,
	}})
	if err != nil || res.Outcome != api.SpecMergeUnchanged {
		t.Fatalf("the same conflicts again gave %+v (%v), want acme-3 unchanged", res, err)
	}
}

func TestOpenSpecMergeRefusesABadRepositoryConflict(t *testing.T) {
	home := api.SpecMergeConflict{Name: "acme", Conflicts: []string{"README"}, BaseTip: "aaa111", SpecBranchTip: "bbb222"}
	for name, c := range map[string]struct {
		in    api.OpenSpecMergeInput
		field string
	}{
		"an unknown Repository": {api.OpenSpecMergeInput{Repositories: []api.SpecMergeConflict{{Name: "web", Conflicts: []string{"a"}, BaseTip: "a", SpecBranchTip: "b"}}}, "repositories[0].name"},
		"a Repository twice":    {api.OpenSpecMergeInput{Repositories: []api.SpecMergeConflict{home, home}}, "repositories[1].name"},
		"no files":              {api.OpenSpecMergeInput{Repositories: []api.SpecMergeConflict{{Name: "api", BaseTip: "a", SpecBranchTip: "b"}}}, "repositories[0].conflicts"},
		"no base tip":           {api.OpenSpecMergeInput{Repositories: []api.SpecMergeConflict{{Name: "api", Conflicts: []string{"a"}, SpecBranchTip: "b"}}}, "repositories[0].base_tip"},
		"no Spec Branch tip":    {api.OpenSpecMergeInput{Repositories: []api.SpecMergeConflict{{Name: "api", Conflicts: []string{"a"}, BaseTip: "a"}}}, "repositories[0].spec_branch_tip"},
		"the Home Repository in both forms": {api.OpenSpecMergeInput{Conflicts: []string{"README"}, BaseTip: "aaa111", SpecBranchTip: "bbb222",
			Repositories: []api.SpecMergeConflict{home}}, "repositories"},
	} {
		t.Run(name, func(t *testing.T) {
			f := specMergeInTwo(t)
			c.in.Spec = "acme-1"
			_, err := f.core.OpenSpecMerge(ctx, "acme", c.in)
			wantCode(t, err, api.Invalid, c.field)
			if rows, _ := f.core.FindTickets(ctx, "acme", api.FindTicketsInput{Title: specMergeTitle}); len(rows) != 0 {
				t.Errorf("opened %+v", rows)
			}
		})
	}
}

func TestASpecLandedTicketPushedInSomeOfItsRepositoriesStillWaitsForItsSpec(t *testing.T) {
	f := landWorld(t, true)
	f.addRepository(t, api.AddRepositoryInput{Name: "api", CloneURL: "git@github.com:acme/api.git"})
	f.landing(t, domain.Landing{Method: domain.LandingMerge, Push: true, Remote: "origin", LandPerSpec: true})
	err := f.store.Update(func(w store.Writer) error {
		p, _ := w.Project("acme")
		p.CI.Tracking = true
		return w.PutProject(p)
	})
	if err != nil {
		t.Fatal(err)
	}
	f.moveTo(t, 6, domain.Cancelled)
	f.changes(t, 5, "acme", "api")
	f.moveTo(t, 5, domain.Accepted)
	f.specMerge(t, "acme-5", "abc123")
	// The Spec's merge was pushed in the Home Repository, and its merge
	// in api is still to make.
	f.pushedFor(t, "", "acme-5", "def456")

	if !f.getTicket(t, "acme-5").WaitingForSpec {
		t.Fatal("acme-5 isn't waiting for its Spec, though nothing was pushed for it in api")
	}
	if s := f.landStatus(t); !s.Ready || len(s.Waiting) != 1 {
		t.Fatalf("Land per Spec %+v, want the Spec ready with acme-5 waiting, to land in api", s)
	}

	f.pushedFor(t, "api", "acme-5", "fed654")
	if f.getTicket(t, "acme-5").WaitingForSpec {
		t.Fatal("acme-5 still waits for its Spec once pushed in each of its Repositories: it waits on CI")
	}
}

func TestASpecLandedTicketWaitsForItsSpecOnlyInRepositoriesWhereLandPerSpecIsInForce(t *testing.T) {
	f := landWorld(t, true)
	f.addRepository(t, api.AddRepositoryInput{Name: "api", CloneURL: "git@github.com:acme/api.git"})
	// Under Branch in api Land per Spec has no effect: a Ticket lands
	// there once the human merges it (Spec code-central-890).
	f.editLanding(t, "api", api.RepositoryLandingBranch)
	f.changes(t, 5, "api")
	f.moveTo(t, 5, domain.Accepted)
	f.changes(t, 6, "acme", "api")
	f.moveTo(t, 6, domain.Accepted)

	if tk := f.getTicket(t, "acme-5"); !tk.SpecLanded || tk.WaitingForSpec {
		t.Fatalf("acme-5 Spec-landed %v, waiting for its Spec %v; want it Spec-landed, as Land per Spec is in force in the Home, but not waiting: it changed api alone", tk.SpecLanded, tk.WaitingForSpec)
	}
	if !f.getTicket(t, "acme-6").WaitingForSpec {
		t.Fatal("acme-6 isn't waiting for its Spec, though it is to land with it in the Home")
	}
	f.specMerge(t, "acme-6", "abc123")
	if s := f.landStatus(t); !s.Ready || len(s.Waiting) != 1 || s.Waiting[0] != "acme-6" {
		t.Fatalf("Land per Spec %+v, want the Spec ready with acme-6 alone waiting", s)
	}

	if _, err := f.core.Land(ctx, "acme", api.LandInput{Ticket: "acme-6", Commit: "abc123", Repository: "acme"}); err != nil {
		t.Fatal(err)
	}
	if tk := f.getTicket(t, "acme-6"); !tk.PartlyLanded || tk.WaitingForSpec {
		t.Fatalf("acme-6 partly landed %v, waiting for its Spec %v; want it landed in the Home, waiting for the human's merge in api alone", tk.PartlyLanded, tk.WaitingForSpec)
	}
}

func TestASpecLandedTicketLandedInItsRepositoryUnderPullRequestNoLongerWaitsForItsSpec(t *testing.T) {
	f := landWorld(t, true)
	f.addRepository(t, api.AddRepositoryInput{Name: "api", CloneURL: "git@github.com:acme/api.git"})
	f.addRepository(t, api.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git"})
	f.editLanding(t, "api", api.RepositoryLandingPullRequest)
	f.editLanding(t, "web", api.RepositoryLandingBranch)
	f.changes(t, 5, "api", "web")
	f.moveTo(t, 5, domain.Accepted)
	if !f.getTicket(t, "acme-5").WaitingForSpec {
		t.Fatal("acme-5 isn't waiting for its Spec, though it is to land with the Spec's pull request in api")
	}

	if _, err := f.core.Land(ctx, "acme", api.LandInput{Ticket: "acme-5", Commit: "abc123", Repository: "api"}); err != nil {
		t.Fatal(err)
	}

	if tk := f.getTicket(t, "acme-5"); !tk.PartlyLanded || tk.WaitingForSpec {
		t.Fatalf("acme-5 partly landed %v, waiting for its Spec %v; want it landed in api, waiting for the human's merge in web alone", tk.PartlyLanded, tk.WaitingForSpec)
	}
}

func TestASpecLandedTicketPushedInARepositoryUnderBranchCanBeSentBackToFixItsCI(t *testing.T) {
	f := landWorld(t, true)
	f.addRepository(t, api.AddRepositoryInput{Name: "api", CloneURL: "git@github.com:acme/api.git"})
	f.editLanding(t, "api", api.RepositoryLandingBranchPush)
	err := f.store.Update(func(w store.Writer) error {
		p, _ := w.Project("acme")
		p.CI.Tracking = true
		return w.PutProject(p)
	})
	if err != nil {
		t.Fatal(err)
	}
	f.changes(t, 5, "api")
	f.moveTo(t, 5, domain.Accepted)
	// Its own branch in api, pushed: under Branch Land per Spec has no
	// effect there, so its work isn't shared with its Spec's.
	f.pushedFor(t, "api", "acme-5", "abc123")
	f.ciRunIn(t, "api", 7, "abc123", "acme-5", domain.CICompleted, domain.CIFailure)

	if tk := f.getTicket(t, "acme-5"); !tk.SpecLanded || !slices.Contains(tk.CIFailureOptions, api.OptionSendBack) {
		t.Fatalf("acme-5 Spec-landed %v with CI failure options %v; want Send back to fix offered", tk.SpecLanded, tk.CIFailureOptions)
	}
	if _, err := f.core.SendBack(human, "acme", api.SendBackInput{Ticket: "acme-5", Note: "Fix the build."}); err != nil {
		t.Fatalf("send_back: %v", err)
	}
}

func TestReportSpecMergeRecordsTheRepositoryItNames(t *testing.T) {
	f := landWorld(t, true)
	f.addRepository(t, api.AddRepositoryInput{Name: "api", CloneURL: "git@github.com:acme/api.git"})
	f.moveTo(t, 5, domain.Accepted)

	if _, err := f.core.ReportSpecMerge(ctx, "acme", api.ReportSpecMergeInput{Ticket: "acme-5", Commit: "abc123", Repository: "api"}); err != nil {
		t.Fatalf("report_spec_merge: %v", err)
	}

	if sm := f.getTicket(t, "acme-5").SpecMerge; sm == nil || sm.Commit != "abc123" || sm.Repository != "api" || !sm.Counts {
		t.Fatalf("spec merge %+v, want abc123 in api, counting", sm)
	}
	_, err := f.core.ReportSpecMerge(ctx, "acme", api.ReportSpecMergeInput{Ticket: "acme-5", Commit: "abc123", Repository: "web"})
	wantCode(t, err, api.Invalid, "repository")
}
