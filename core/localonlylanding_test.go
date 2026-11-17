package core_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Landing in a local-only Repository (Spec code-central-879, ADR 0039):
// its Landing Policy is the Project's with Push off, so under Merge +
// Push no pushed commit is recorded there, its CI neither gates landing
// nor offers Land anyway, and its base is never Base diverged.

// makeLocalOnly has the human clear the Repository name's Clone URL,
// keeping its clone at its local path: it is local-only from then on.
func (f fixture) makeLocalOnly(t *testing.T, name string) {
	t.Helper()
	none, path := "", srcPath(name)
	if _, err := f.core.EditRepository(human, "acme", core.EditRepositoryInput{Name: name, CloneURL: &none, LocalPath: &path}); err != nil {
		t.Fatal(err)
	}
}

func TestUnderMergeAndPushNoPushedCommitIsRecordedInALocalOnlyRepository(t *testing.T) {
	f := itemWorld(t)
	f.addRepository(t, core.AddRepositoryInput{Name: "api", LocalPath: srcPath("api")})
	f.changes(t, 5, "acme", "api")

	f.pushedFor(t, "api", "acme-5", "def456")

	v := f.getTicket(t, "acme-5")
	if api := changedIn(t, v, "api"); api.PushedCommit != nil || api.CIResult != "" || api.NotLanded != "" {
		t.Fatalf("api %+v, want no pushed commit recorded, so no CI result and nothing waited on", api)
	}
	if home := changedIn(t, v, "acme"); home.PushedCommit == nil || home.PushedCommit.Commit != "abc123" {
		t.Fatalf("Home %+v, want its pushed commit abc123 kept", home)
	}
}

func TestARepositoryWhoseCloneURLIsClearedNeitherWaitsOnItsCINorOffersLandAnyway(t *testing.T) {
	f := twoRepoWorld(t)
	f.changes(t, 5, "acme", "api")
	f.pushedFor(t, "api", "acme-5", "def456")
	f.ciRunIn(t, "api", 1, "def456", "acme-5", store.CIInProgress, "")
	if api := changedIn(t, f.getTicket(t, "acme-5"), "api"); api.NotLanded != "waiting on CI at def456" {
		t.Fatalf("api %+v, want it waiting on its CI while it has a Clone URL", api)
	}

	f.makeLocalOnly(t, "api")

	if api := changedIn(t, f.getTicket(t, "acme-5"), "api"); api.NotLanded != "" || api.CIResult != core.CIRunning {
		t.Fatalf("api %+v, want its CI result shown and not waited on, as under Merge without Push", api)
	}
	f.ciRunIn(t, "api", 1, "def456", "acme-5", store.CICompleted, store.CIFailure)
	v := f.getTicket(t, "acme-5")
	i := slices.IndexFunc(v.CIItems, func(it core.CIItemView) bool { return it.Repository == "api" })
	if i < 0 || slices.Contains(v.CIItems[i].Options, core.OptionLandAnyway) {
		t.Fatalf("CI items %+v, want api's to offer no Land anyway", v.CIItems)
	}
	_, err := f.core.LandAnyway(human, "acme", core.CIResponseInput{Ticket: "acme-5", Repository: "api"})
	notAllowed(t, err)
	if want := "api is local-only, with no Clone URL"; !strings.Contains(err.Error(), want) {
		t.Fatalf("refusal %q, want it to say %q", err, want)
	}
	if home := changedIn(t, v, "acme"); home.NotLanded != "waiting on CI at abc123" {
		t.Fatalf("Home %+v, want it still waiting on its own CI", home)
	}
}

func TestUnderMergeAndPushALocalOnlyRepositorysBaseIsNeverBaseDiverged(t *testing.T) {
	f := newFixture(t)
	f.basesProject(t)
	f.makeLocalOnly(t, "api")

	// As a Factory that hadn't yet seen api turn local-only would.
	f.reportBase(t, core.ReportBaseInput{Repository: "api", State: core.BaseDiverged, Local: "aaa111", Remote: "bbb222"})
	f.reportBase(t, core.ReportBaseInput{Repository: "web", State: core.BaseDiverged, Local: "ccc333", Remote: "ddd444"})

	diverged := f.baseRows(t, core.ReasonBaseDiverged)
	if len(diverged) != 1 || diverged[0].BaseDiverged.Repository != "web" {
		t.Fatalf("Base diverged rows %+v, want web's alone: api is local-only", diverged)
	}
}
