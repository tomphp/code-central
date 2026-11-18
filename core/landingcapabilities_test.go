package core_test

import (
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
)

// Landing capabilities (Spec code-central-931): the Project's landing
// view says what the Options form offers for each Landing Method, by its
// Landing Policy, so the UI never branches on the method itself.

// setDefaultLanding sets acme's Default Landing to method with push.
func (f fixture) setDefaultLanding(t *testing.T, method domain.LandingMethod, push bool) {
	t.Helper()
	if _, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: method, Push: push, Remote: "origin"}); err != nil {
		t.Fatalf("set_landing_method %s push %v: %v", method, push, err)
	}
}

func TestEachLandingMethodSaysWhatTheOptionsFormOffersAsIfItWereTheDefault(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	// "Under Pull Request, Push is implied"; "Land per Spec applies under
	// Merge and Pull Request, and has no effect under Branch"; the Code
	// Host is worked out for Pull Request, "the Ticket lands when the host
	// reports it merged".
	want := core.LandingMethodsCapabilities{
		Merge:       core.LandingMethodCapabilities{OffersPush: true, OffersLandPerSpec: true},
		Branch:      core.LandingMethodCapabilities{OffersPush: true},
		PullRequest: core.LandingMethodCapabilities{OffersLandPerSpec: true, ShowsCodeHost: true},
	}
	if got := f.landingOf(t).Methods; got != want {
		t.Fatalf("methods %+v, want %+v", got, want)
	}

	// The table is the same whatever the saved Default Landing and Land
	// per Spec: Land per Spec is offered under Merge with it off.
	off := false
	if _, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: domain.LandingBranch, Push: true, Remote: "origin", LandPerSpec: &off}); err != nil {
		t.Fatal(err)
	}
	if got := f.landingOf(t).Methods; got != want {
		t.Fatalf("methods under Branch + Push without Land per Spec %+v, want %+v", got, want)
	}
}

func TestThePATColumnSaysWhetherAnAzureReposRepositoryWouldBeUnderPullRequest(t *testing.T) {
	// The PAT variable's "shown while any Azure Repos Repository is under
	// Pull Request", with each method as the Default Landing.
	pat := func(v core.LandingView) [3]bool {
		return [3]bool{v.Methods.Merge.UsesAzureReposPAT, v.Methods.Branch.UsesAzureReposPAT, v.Methods.PullRequest.UsesAzureReposPAT}
	}
	f, _ := prWorld(t)
	f.addRepository(t, core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git", Landing: core.RepositoryLandingPullRequest})
	if got := pat(f.landingOf(t)); got != [3]bool{} {
		t.Fatalf("PAT column %v with no Azure Repos Repository, want it unused under every method", got)
	}

	f.addRepository(t, core.AddRepositoryInput{Name: "infra", CloneURL: "https://dev.azure.com/acme/ops/_git/infra"})
	if got := pat(f.landingOf(t)); got != [3]bool{false, false, true} {
		t.Fatalf("PAT column %v with infra following the default, want it used under Pull Request alone", got)
	}

	f.editLanding(t, "infra", core.RepositoryLandingPullRequest)
	if got := pat(f.landingOf(t)); got != [3]bool{true, true, true} {
		t.Fatalf("PAT column %v with infra's Own Landing Pull Request, want it used under every method", got)
	}

	f.editLanding(t, "infra", core.RepositoryLandingMerge)
	if got := pat(f.landingOf(t)); got != [3]bool{} {
		t.Fatalf("PAT column %v with infra's Own Landing Merge, want it unused under every method", got)
	}
}

func TestThePullRequestWatcherRunsAsSavedNotAsDrafted(t *testing.T) {
	// "The service runs one per Project while any Repository is under
	// Pull Request".
	f, _ := prWorld(t)
	f.addRepository(t, core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git"})
	if f.landingOf(t).RunsPullRequestWatcher {
		t.Fatal("the Pull Request Watcher runs with no Repository under Pull Request")
	}
	f.editLanding(t, "web", core.RepositoryLandingPullRequest)
	if !f.landingOf(t).RunsPullRequestWatcher {
		t.Fatal("the Pull Request Watcher doesn't run with web's Own Landing Pull Request")
	}
	f.editLanding(t, "web", core.RepositoryLandingDefault)
	f.setDefaultLanding(t, domain.LandingPullRequest, false)
	if !f.landingOf(t).RunsPullRequestWatcher {
		t.Fatal("the Pull Request Watcher doesn't run with a Default Landing of Pull Request")
	}
}

func TestTheDefaultLandingIsNamedAsARepositoryLanding(t *testing.T) {
	f, _ := prWorld(t)
	for _, c := range []struct {
		method domain.LandingMethod
		push   bool
		want   core.RepositoryLanding
	}{
		{domain.LandingMerge, false, core.RepositoryLandingMerge},
		{domain.LandingMerge, true, core.RepositoryLandingMergePush},
		{domain.LandingBranch, false, core.RepositoryLandingBranch},
		{domain.LandingBranch, true, core.RepositoryLandingBranchPush},
		// "Under Pull Request, Push is implied".
		{domain.LandingPullRequest, true, core.RepositoryLandingPullRequest},
	} {
		f.setDefaultLanding(t, c.method, c.push)
		if got := f.landingOf(t).RepositoryLanding; got != c.want {
			t.Errorf("repository_landing under %s push %v = %s, want %s", c.method, c.push, got, c.want)
		}
	}
}

func TestPushPushesNothingWhileEveryRepositoryIsLocalOnly(t *testing.T) {
	// A local-only Repository's policy has "Push off when the Repository
	// is local-only", so Push on in some Landing pushes nothing while
	// every Repository is.
	f := newFixture(t)
	f.project(t, "acme")
	if _, err := f.core.ReportPushRemote(ctx, "acme", core.ReportPushRemoteInput{URL: ""}); err != nil {
		t.Fatal(err)
	}
	f.addRepository(t, core.AddRepositoryInput{Name: "docs", LocalPath: srcPath("docs")})
	if f.landingOf(t).PushesNothing {
		t.Fatal("pushes_nothing with Push off everywhere")
	}

	f.setDefaultLanding(t, domain.LandingMerge, true)
	if !f.landingOf(t).PushesNothing {
		t.Fatal("not pushes_nothing with a Default Landing of Merge + Push and every Repository local-only")
	}

	f.setDefaultLanding(t, domain.LandingMerge, false)
	f.editLanding(t, "docs", core.RepositoryLandingBranchPush)
	if !f.landingOf(t).PushesNothing {
		t.Fatal("not pushes_nothing with docs's Own Landing Branch + Push and every Repository local-only")
	}

	f.addRepository(t, core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git"})
	if f.landingOf(t).PushesNothing {
		t.Fatal("pushes_nothing with web, which has a Clone URL")
	}
}
