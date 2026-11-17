package core_test

import (
	"testing"

	"github.com/tomoram/code-central/core"
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
	f.addRepository(t, core.AddRepositoryInput{Name: "api", CloneURL: "git@github.com:acme/api.git", Base: "trunk"})
	return f
}

func TestOpenSpecMergeNamesEachConflictingRepositoryInLandingOrder(t *testing.T) {
	f := specMergeInTwo(t)
	if _, err := f.core.SetLandingOrder(human, "acme", core.SetLandingOrderInput{Names: []string{"api", "acme"}}); err != nil {
		t.Fatal(err)
	}

	res, err := f.core.OpenSpecMerge(ctx, "acme", core.OpenSpecMergeInput{Spec: "acme-1", Repositories: []core.SpecMergeConflict{
		{Name: "acme", Conflicts: []string{"README"}, BaseTip: "aaa111", SpecBranchTip: "bbb222"},
		{Name: "api", Conflicts: []string{"endpoint.go", "go.mod"}, BaseTip: "ccc333", SpecBranchTip: "ddd444"},
	}})
	if err != nil {
		t.Fatalf("open_spec_merge: %v", err)
	}

	if res != (core.OpenSpecMergeResult{Ticket: "acme-3", Outcome: core.SpecMergeOpened}) {
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
	api := core.SpecMergeConflict{Name: "api", Conflicts: []string{"endpoint.go"}, BaseTip: "ccc333", SpecBranchTip: "ddd444"}
	f.core.OpenSpecMerge(ctx, "acme", core.OpenSpecMergeInput{Spec: "acme-1", Conflicts: []string{"README"}, BaseTip: "aaa111", SpecBranchTip: "bbb222", Repositories: []core.SpecMergeConflict{api}})

	// The same conflicts, the Home Repository's named in repositories.
	res, err := f.core.OpenSpecMerge(ctx, "acme", core.OpenSpecMergeInput{Spec: "acme-1", Repositories: []core.SpecMergeConflict{
		{Name: "acme", Conflicts: []string{"README"}, BaseTip: "aaa111", SpecBranchTip: "bbb222"}, api,
	}})
	if err != nil || res.Outcome != core.SpecMergeUnchanged {
		t.Fatalf("the same conflicts again gave %+v (%v), want acme-3 unchanged", res, err)
	}
}

func TestOpenSpecMergeRefusesABadRepositoryConflict(t *testing.T) {
	home := core.SpecMergeConflict{Name: "acme", Conflicts: []string{"README"}, BaseTip: "aaa111", SpecBranchTip: "bbb222"}
	for name, c := range map[string]struct {
		in    core.OpenSpecMergeInput
		field string
	}{
		"an unknown Repository": {core.OpenSpecMergeInput{Repositories: []core.SpecMergeConflict{{Name: "web", Conflicts: []string{"a"}, BaseTip: "a", SpecBranchTip: "b"}}}, "repositories[0].name"},
		"a Repository twice":    {core.OpenSpecMergeInput{Repositories: []core.SpecMergeConflict{home, home}}, "repositories[1].name"},
		"no files":              {core.OpenSpecMergeInput{Repositories: []core.SpecMergeConflict{{Name: "api", BaseTip: "a", SpecBranchTip: "b"}}}, "repositories[0].conflicts"},
		"no base tip":           {core.OpenSpecMergeInput{Repositories: []core.SpecMergeConflict{{Name: "api", Conflicts: []string{"a"}, SpecBranchTip: "b"}}}, "repositories[0].base_tip"},
		"no Spec Branch tip":    {core.OpenSpecMergeInput{Repositories: []core.SpecMergeConflict{{Name: "api", Conflicts: []string{"a"}, BaseTip: "a"}}}, "repositories[0].spec_branch_tip"},
		"the Home Repository in both forms": {core.OpenSpecMergeInput{Conflicts: []string{"README"}, BaseTip: "aaa111", SpecBranchTip: "bbb222",
			Repositories: []core.SpecMergeConflict{home}}, "repositories"},
	} {
		t.Run(name, func(t *testing.T) {
			f := specMergeInTwo(t)
			c.in.Spec = "acme-1"
			_, err := f.core.OpenSpecMerge(ctx, "acme", c.in)
			wantCode(t, err, core.Invalid, c.field)
			if rows, _ := f.core.FindTickets(ctx, "acme", core.FindTicketsInput{Title: specMergeTitle}); len(rows) != 0 {
				t.Errorf("opened %+v", rows)
			}
		})
	}
}

func TestASpecLandedTicketPushedInSomeOfItsRepositoriesStillWaitsForItsSpec(t *testing.T) {
	f := landWorld(t, true)
	f.addRepository(t, core.AddRepositoryInput{Name: "api", CloneURL: "git@github.com:acme/api.git"})
	f.landing(t, store.Landing{Method: store.LandingMerge, Push: true, Remote: "origin", LandPerSpec: true})
	err := f.store.Update(func(w store.Writer) error {
		p, _ := w.Project("acme")
		p.CI.Tracking = true
		return w.PutProject(p)
	})
	if err != nil {
		t.Fatal(err)
	}
	f.moveTo(t, 6, store.Cancelled)
	f.changes(t, 5, "acme", "api")
	f.moveTo(t, 5, store.Accepted)
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

func TestReportSpecMergeRecordsTheRepositoryItNames(t *testing.T) {
	f := landWorld(t, true)
	f.addRepository(t, core.AddRepositoryInput{Name: "api", CloneURL: "git@github.com:acme/api.git"})
	f.moveTo(t, 5, store.Accepted)

	if _, err := f.core.ReportSpecMerge(ctx, "acme", core.ReportSpecMergeInput{Ticket: "acme-5", Commit: "abc123", Repository: "api"}); err != nil {
		t.Fatalf("report_spec_merge: %v", err)
	}

	if sm := f.getTicket(t, "acme-5").SpecMerge; sm == nil || sm.Commit != "abc123" || sm.Repository != "api" || !sm.Counts {
		t.Fatalf("spec merge %+v, want abc123 in api, counting", sm)
	}
	_, err := f.core.ReportSpecMerge(ctx, "acme", core.ReportSpecMergeInput{Ticket: "acme-5", Commit: "abc123", Repository: "web"})
	wantCode(t, err, core.Invalid, "repository")
}
