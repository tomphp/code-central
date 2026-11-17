package core_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Per-Repository Landing (Spec code-central-890): each Repository, the
// Home included, follows the Project's Default Landing or has its Own
// Landing, and may name its own Push remote. Its Landing Policy is its
// Own Landing, else the Default Landing, with Push off when it is
// local-only; Pull Request is refused only where a local-only Repository
// would end up under it.

// landingIn is the Repository name's effective Landing, whether it is
// its own, and its effective Push remote, as the Project shows them.
func (f fixture) landingIn(t *testing.T, name string) (core.RepositoryLanding, bool, string) {
	t.Helper()
	r := repository(t, f.projectView(t, "acme"), name)
	return r.Landing, r.OwnLanding, r.PushRemote
}

func (f fixture) editLanding(t *testing.T, name string, l core.RepositoryLanding) {
	t.Helper()
	if _, err := f.core.EditRepository(human, "acme", core.EditRepositoryInput{Name: name, Landing: &l}); err != nil {
		t.Fatalf("edit_repository %s landing %s: %v", name, l, err)
	}
}

// wantRefusalNaming checks err is Invalid on field, with a message and
// fix that name the Repository name and a fix saying want.
func wantRefusalNaming(t *testing.T, err error, field, name, want string) {
	t.Helper()
	wantFix(t, err, core.Invalid, field)
	var e *core.Error
	errors.As(err, &e)
	if !strings.Contains(e.Message, name) || !strings.Contains(e.Fix, name) || !strings.Contains(e.Fix, want) {
		t.Fatalf("refusal %q, fix %q, want both to name %s and the fix to say %q", e.Message, e.Fix, name, want)
	}
}

func TestARepositoryFollowsTheDefaultLandingUntilItHasItsOwn(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.addRepository(t, core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git"})
	for _, name := range []string{"acme", "web"} {
		if l, own, remote := f.landingIn(t, name); l != core.RepositoryLandingMerge || own || remote != "origin" {
			t.Fatalf("%s: landing %s, own %v, push remote %q; want the Default Landing, Merge, with origin", name, l, own, remote)
		}
	}

	if _, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingMerge, Push: true, Remote: "github"}); err != nil {
		t.Fatal(err)
	}
	if l, own, remote := f.landingIn(t, "web"); l != core.RepositoryLandingMergePush || own || remote != "github" {
		t.Fatalf("web: landing %s, own %v, push remote %q; want it following the Default Landing, Merge + Push to github", l, own, remote)
	}

	branch, upstream := core.RepositoryLandingBranchPush, "upstream"
	if _, err := f.core.EditRepository(human, "acme", core.EditRepositoryInput{Name: "web", Landing: &branch, PushRemote: &upstream}); err != nil {
		t.Fatal(err)
	}
	if l, own, remote := f.landingIn(t, "web"); l != core.RepositoryLandingBranchPush || !own || remote != "upstream" {
		t.Fatalf("web: landing %s, own %v, push remote %q; want its Own Landing, Branch + Push to upstream", l, own, remote)
	}
	if l, own, _ := f.landingIn(t, "acme"); l != core.RepositoryLandingMergePush || own {
		t.Fatalf("the Home: landing %s, own %v; want it still following the Default Landing", l, own)
	}

	f.editLanding(t, "web", core.RepositoryLandingDefault)
	none := ""
	if _, err := f.core.EditRepository(human, "acme", core.EditRepositoryInput{Name: "web", PushRemote: &none}); err != nil {
		t.Fatal(err)
	}
	if l, own, remote := f.landingIn(t, "web"); l != core.RepositoryLandingMergePush || own || remote != "github" {
		t.Fatalf("web: landing %s, own %v, push remote %q; want it back on the Default Landing and the Project's remote", l, own, remote)
	}
}

func TestTheHomeTakesAnOwnLandingAndPushRemoteButNothingElseItRefusedBefore(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	branch, upstream := core.RepositoryLandingBranch, "upstream"
	if _, err := f.core.EditRepository(human, "acme", core.EditRepositoryInput{Name: "acme", Landing: &branch, PushRemote: &upstream}); err != nil {
		t.Fatalf("edit the Home's landing and push remote: %v", err)
	}
	if l, own, remote := f.landingIn(t, "acme"); l != core.RepositoryLandingBranch || !own || remote != "upstream" {
		t.Fatalf("the Home: landing %s, own %v, push remote %q; want its Own Landing, Branch, with upstream", l, own, remote)
	}
	base := "develop"
	_, err := f.core.EditRepository(human, "acme", core.EditRepositoryInput{Name: "acme", Base: &base, Landing: &branch})
	wantFix(t, err, core.Invalid, "base")
}

func TestEditAndAddRepositoryRefuseAnUnknownLandingOrPushRemote(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	_, err := f.core.AddRepository(human, "acme", core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git", Landing: "squash"})
	wantFix(t, err, core.Invalid, "landing")
	_, err = f.core.AddRepository(human, "acme", core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git", PushRemote: "my remote"})
	wantFix(t, err, core.Invalid, "push_remote")
}

func TestARepositorysOwnLandingDecidesItsLandingRules(t *testing.T) {
	f := twoRepoWorld(t) // Merge + Push with CI Tracking on
	f.changes(t, 5, "acme", "api")
	f.pushedFor(t, "api", "acme-5", "def456")
	f.ciRunIn(t, "api", 1, "def456", "acme-5", store.CIInProgress, "")
	if api := changedIn(t, f.getTicket(t, "acme-5"), "api"); api.NotLanded != "waiting on CI at def456" {
		t.Fatalf("api %+v, want it waiting on its CI under the Default Landing, Merge + Push", api)
	}

	f.editLanding(t, "api", core.RepositoryLandingMerge)

	v := f.getTicket(t, "acme-5")
	if api := changedIn(t, v, "api"); api.NotLanded == "waiting on CI at def456" {
		t.Fatalf("api %+v, want no CI gate under its Own Landing, Merge", api)
	}
	if home := changedIn(t, v, "acme"); home.NotLanded != "waiting on CI at abc123" {
		t.Fatalf("Home %+v, want it still gated by the Default Landing", home)
	}
}

func TestATicketChangedOnlyInARepositoryUnderBranchIsToMerge(t *testing.T) {
	f := twoRepoWorld(t)
	f.editLanding(t, "api", core.RepositoryLandingBranch)
	f.changes(t, 8, "api")
	f.changes(t, 5, "acme")
	wantReason(t, f.get(t, "acme-8"), core.ReasonToMerge)
	if v := f.get(t, "acme-5"); v.NeedsMeReason == core.ReasonToMerge {
		t.Fatalf("acme-5, changed only in the Home under Merge + Push, is To merge")
	}
}

// prWorld is a Project whose Home reports a GitHub Push remote, with a
// Pull Request check that passes, recording what it checks.
func prWorld(t *testing.T) (fixture, *[]core.PullRequestCheckInput) {
	t.Helper()
	f := newFixture(t)
	f.project(t, "acme")
	if _, err := f.core.ReportPushRemote(ctx, "acme", core.ReportPushRemoteInput{URL: "git@github.com:acme/acme.git"}); err != nil {
		t.Fatal(err)
	}
	checked := &[]core.PullRequestCheckInput{}
	f.core.SetPullRequestCheck(func(_ context.Context, in core.PullRequestCheckInput) core.WatcherCheckResult {
		*checked = append(*checked, in)
		return core.WatcherCheckResult{}
	})
	return f, checked
}

func TestThePullRequestWatcherRunsWhileAnyRepositoryIsUnderPullRequest(t *testing.T) {
	f, checked := prWorld(t)
	f.addRepository(t, core.AddRepositoryInput{Name: "web", CloneURL: "https://dev.azure.com/acme/shop/_git/web"})
	watch := func() core.PullRequestWatch {
		t.Helper()
		w, err := f.core.GetPullRequestWatch(ctx, "acme")
		if err != nil {
			t.Fatal(err)
		}
		return w
	}
	if watch().On || f.landingOf(t).UsesAzureReposPAT {
		t.Fatalf("the Pull Request Watcher runs, or the PAT variable is used, with no Repository under Pull Request")
	}

	f.editLanding(t, "web", core.RepositoryLandingPullRequest)

	if len(*checked) != 1 || (*checked)[0].Repositories[0].Name != "web" {
		t.Fatalf("checked %+v, want web alone checked as it came under Pull Request", *checked)
	}
	w := watch()
	if !w.On || len(w.Repositories) != 1 || w.Repositories[0].Name != "web" {
		t.Fatalf("watch on %v with %+v, want it on for web alone", w.On, w.Repositories)
	}
	if !f.landingOf(t).UsesAzureReposPAT {
		t.Fatalf("the PAT variable is unused with web, on Azure Repos, under Pull Request")
	}
	if v := f.projectView(t, "acme"); !slices.ContainsFunc(v.Repositories, func(r core.RepositoryView) bool {
		return r.Name == "web" && r.Landing == core.RepositoryLandingPullRequest
	}) {
		t.Fatalf("repositories %+v, want web under Pull Request", v.Repositories)
	}
}

func TestPullRequestIsRefusedAsALocalOnlyRepositorysOwnLanding(t *testing.T) {
	f, _ := prWorld(t)
	pr := core.RepositoryLandingPullRequest
	_, err := f.core.AddRepository(human, "acme", core.AddRepositoryInput{Name: "notes", LocalPath: srcPath("notes"), Landing: pr})
	wantRefusalNaming(t, err, "landing", "notes", "Clone URL")

	f.addRepository(t, core.AddRepositoryInput{Name: "notes", LocalPath: srcPath("notes")})
	_, err = f.core.EditRepository(human, "acme", core.EditRepositoryInput{Name: "notes", Landing: &pr})
	wantRefusalNaming(t, err, "landing", "notes", "Clone URL")
}

func TestTheDefaultLandingOfPullRequestIsRefusedWhileALocalOnlyRepositoryFollowsIt(t *testing.T) {
	f, _ := prWorld(t)
	f.addRepository(t, core.AddRepositoryInput{Name: "notes", LocalPath: srcPath("notes")})
	f.addRepository(t, core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git"})

	_, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingPullRequest})
	wantRefusalNaming(t, err, "method", "notes", "Own Landing such as Merge")
	if strings.Contains(err.Error(), "web") {
		t.Fatalf("refusal %q names web, which has a Clone URL", err)
	}

	f.editLanding(t, "notes", core.RepositoryLandingMerge)
	if _, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingPullRequest}); err != nil {
		t.Fatalf("Pull Request with the local-only notes on its Own Landing: %v", err)
	}
	if l, own, _ := f.landingIn(t, "notes"); l != core.RepositoryLandingMerge || !own {
		t.Fatalf("notes: landing %s, own %v, want its Own Landing, Merge", l, own)
	}

	// Back on the Default Landing, notes would be under Pull Request.
	def := core.RepositoryLandingDefault
	_, err = f.core.EditRepository(human, "acme", core.EditRepositoryInput{Name: "notes", Landing: &def})
	wantRefusalNaming(t, err, "landing", "notes", "Clone URL")
}

func TestAClonelessRepositoryUnderPullRequestIsRefused(t *testing.T) {
	f, _ := prWorld(t)
	f.addRepository(t, core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git", LocalPath: srcPath("web"), Landing: core.RepositoryLandingPullRequest})
	none := ""
	_, err := f.core.EditRepository(human, "acme", core.EditRepositoryInput{Name: "web", CloneURL: &none})
	wantRefusalNaming(t, err, "clone_url", "web", "Clone URL")

	if _, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingPullRequest}); err != nil {
		t.Fatal(err)
	}
	_, err = f.core.AddRepository(human, "acme", core.AddRepositoryInput{Name: "notes", LocalPath: srcPath("notes")})
	wantRefusalNaming(t, err, "clone_url", "notes", "Own Landing such as Merge")
	f.addRepository(t, core.AddRepositoryInput{Name: "notes", LocalPath: srcPath("notes"), Landing: core.RepositoryLandingBranch})
}

func TestAProjectWhoseOtherRepositoriesAreAllLocalOnlyIsNotRefusedPushOrCITracking(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.addRepository(t, core.AddRepositoryInput{Name: "notes", LocalPath: srcPath("notes")})
	f.addRepository(t, core.AddRepositoryInput{Name: "docs", LocalPath: srcPath("docs")})
	if _, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingMerge, Push: true, Remote: "origin"}); err != nil {
		t.Fatalf("Merge + Push: %v", err)
	}
	if _, err := f.core.SetCITracking(human, "acme", core.SetCITrackingInput{Tracking: true, Repository: "acme/acme"}); err != nil {
		t.Fatalf("CI Tracking: %v", err)
	}
}

func TestARepositoryLocalOnlyUnderPullRequestIsShownAsSuchAndNotReRouted(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	// Only a Factory's report can bring this about (a Home that lost its
	// remote), so the store is written directly.
	err := f.store.Update(func(w store.Writer) error {
		p, _ := w.Project("acme")
		p.SetRepositories(append(p.RepositoryList(), store.Repository{Name: "notes", Base: "main", LocalPath: srcPath("notes"), OwnLanding: store.OwnLanding{Method: store.LandingPullRequest}}))
		return w.PutProject(p)
	})
	if err != nil {
		t.Fatal(err)
	}
	r := repository(t, f.projectView(t, "acme"), "notes")
	if !r.LocalOnlyUnderPullRequest || r.Landing != core.RepositoryLandingPullRequest {
		t.Fatalf("notes %+v, want it shown local-only under Pull Request, its Landing kept", r)
	}
	if home := repository(t, f.projectView(t, "acme"), "acme"); home.LocalOnlyUnderPullRequest {
		t.Fatalf("the Home %+v is flagged", home)
	}
}
