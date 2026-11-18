package core_test

import (
	"context"
	"slices"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
)

// A local-only Home Repository (Spec code-central-890): the Factory
// reports that the Home's clone has no Push remote with an empty URL,
// which clears its Clone URL and makes it local-only, Not watched to
// start with. Its CI choice is a per-Repository CI entry like the
// others', and any Repository, the Home included, may be Not watched.

// trackingOn is CI settings with CI Tracking on and the Home Repository,
// acme, watched by GitHub Actions on repository.
func trackingOn(repository string) domain.CI {
	return domain.CI{Tracking: true, Repositories: []domain.CIRepository{{Name: "acme", Provider: domain.CIGitHubActions, Repository: repository}}}
}

// reportPushRemote reports, as the Factory, url as the Home's Push
// remote's URL: empty for none.
func (f fixture) reportPushRemote(t *testing.T, url string) {
	t.Helper()
	if _, err := f.core.ReportPushRemote(ctx, "acme", core.ReportPushRemoteInput{URL: url}); err != nil {
		t.Fatalf("report_push_remote %q: %v", url, err)
	}
}

// home is the Home Repository's view.
func (f fixture) home(t *testing.T) core.RepositoryView {
	t.Helper()
	return repository(t, f.projectView(t, "acme"), "acme")
}

func TestReportingNoPushRemoteMakesTheHomeLocalOnlyAndALaterURLMakesItPushedAgain(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	if h := f.home(t); h.LocalOnly || h.CloneURL != "" {
		t.Fatalf("Home %+v, want it not local-only before any report: a Factory that never reports none leaves it pushed", h)
	}
	f.reportPushRemote(t, "git@github.com:acme/acme.git")
	if h := f.home(t); h.LocalOnly || h.CloneURL != "git@github.com:acme/acme.git" || h.CI.NotWatched || h.CI.Repository != "acme/acme" {
		t.Fatalf("Home %+v, want it pushed to its reported URL and watched on acme/acme", h)
	}

	f.reportPushRemote(t, "")
	h := f.home(t)
	if !h.LocalOnly || h.CloneURL != "" || !h.CI.NotWatched {
		t.Fatalf("Home %+v, want it local-only with no Clone URL, and Not watched", h)
	}
	if _, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: domain.LandingMerge, Push: true, Remote: "origin"}); err != nil {
		t.Fatalf("Merge + Push with a local-only Home: %v", err)
	}
	if lp := f.projectView(t, "acme").LandingPolicies().In("acme"); lp.PushesBase() || lp.CIGateHolds() || lp.OffersLandAnyway() || !lp.FlagsNotLanded() {
		t.Fatalf("the local-only Home's policy %+v, want Merge with Push off: no push, no CI gate, no Land anyway, flagged when not landed", lp)
	}

	f.reportPushRemote(t, "git@github.com:acme/acme.git")
	h = f.home(t)
	if h.LocalOnly || h.CloneURL != "git@github.com:acme/acme.git" || !h.CI.NotWatched {
		t.Fatalf("Home %+v, want it pushed again, its CI entry still Not watched as the human left it", h)
	}
	if lp := f.projectView(t, "acme").LandingPolicies().In("acme"); !lp.PushesBase() || !lp.OffersLandAnyway() {
		t.Fatalf("the Home's policy %+v, want Merge + Push again", lp)
	}
}

func TestALocalOnlyHomeIsRefusedPullRequestAndFlaggedNotReroutedWhenItLosesItsRemoteUnderIt(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.reportPushRemote(t, "")

	_, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: domain.LandingPullRequest, Remote: "origin"})
	wantRefusalNaming(t, err, "method", "acme", "an Own Landing such as Merge")
	pr := core.RepositoryLandingPullRequest
	_, err = f.core.EditRepository(human, "acme", core.EditRepositoryInput{Name: "acme", Landing: &pr})
	wantRefusalNaming(t, err, "landing", "acme", "a Clone URL")

	// Pushed, it may land by Pull Request; losing its remote then can't
	// be refused, as the Factory reports it.
	f.reportPushRemote(t, "git@github.com:acme/acme.git")
	if _, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: domain.LandingPullRequest, Remote: "origin"}); err != nil {
		t.Fatalf("Pull Request with the Home pushed: %v", err)
	}
	f.reportPushRemote(t, "")
	h := f.home(t)
	if !h.LocalOnly || !h.LocalOnlyUnderPullRequest || h.Landing != core.RepositoryLandingPullRequest {
		t.Fatalf("Home %+v, want it local-only under Pull Request, flagged and still on Pull Request", h)
	}
	if lp := f.projectView(t, "acme").LandingPolicies().In("acme"); !lp.LandsByPullRequest() {
		t.Fatalf("the Home's policy %+v, want it still Pull Request, not re-routed to Merge", lp)
	}
	// A change to the Default Landing that leaves it there is not refused.
	off := false
	if _, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: domain.LandingPullRequest, Remote: "origin", LandPerSpec: &off}); err != nil {
		t.Fatalf("Land per Spec off while the Home is local-only under Pull Request: %v", err)
	}
	// Its fix: another Landing.
	merge := core.RepositoryLandingMerge
	if _, err := f.core.EditRepository(human, "acme", core.EditRepositoryInput{Name: "acme", Landing: &merge}); err != nil {
		t.Fatal(err)
	}
	if h := f.home(t); h.LocalOnlyUnderPullRequest {
		t.Fatalf("Home %+v, want it no longer flagged on its Own Landing of Merge", h)
	}
}

func TestEveryRepositoryLocalOnlyIsNotRefused(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.reportPushRemote(t, "")
	f.addRepository(t, core.AddRepositoryInput{Name: "web", LocalPath: srcPath("web")})
	for _, m := range []domain.LandingMethod{domain.LandingMerge, domain.LandingBranch} {
		if _, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: m, Push: true, Remote: "origin"}); err != nil {
			t.Fatalf("%s + Push with every Repository local-only: %v", m, err)
		}
	}
	for _, r := range f.projectView(t, "acme").Repositories {
		if !r.LocalOnly || !r.CI.NotWatched {
			t.Fatalf("%s %+v, want it local-only and Not watched", r.Name, r)
		}
	}
}

func TestTheHomeCanBeNotWatchedWhileCITrackingIsOn(t *testing.T) {
	f := twoRepoWorld(t)
	f.changes(t, 5, "acme")
	var checked []string
	f.core.SetCICheck(func(_ context.Context, in core.CICheckInput) core.WatcherCheckResult {
		checked = append(checked, in.Repository)
		return core.WatcherCheckResult{}
	})
	if _, err := f.core.SetRepositoryCI(human, "acme", core.SetRepositoryCIInput{Name: "acme", RepositoryCI: core.RepositoryCI{NotWatched: true}}); err != nil {
		t.Fatalf("set_repository_ci on the Home: %v", err)
	}
	p := f.projectView(t, "acme")
	if !p.CI.Tracking || !repository(t, p, "acme").CI.NotWatched {
		t.Fatalf("CI %+v, Home %+v; want CI Tracking still on with the Home Not watched", p.CI, repository(t, p, "acme"))
	}
	if home := changedIn(t, f.getTicket(t, "acme-5"), "acme"); !home.CINotWatched || home.CIResult != "" {
		t.Fatalf("Home %+v, want its CI Not watched, with no CI result", home)
	}
	if watch, err := f.core.GetCIWatch(ctx, "acme"); err != nil || !watch.NotWatched || !watch.Tracking {
		t.Fatalf("CI watch %+v (%v), want the Home Not watched while CI Tracking is on", watch, err)
	}

	if _, err := f.core.SetRepositoryCI(human, "acme", core.SetRepositoryCIInput{Name: "acme", RepositoryCI: core.RepositoryCI{Repository: "acme/web"}}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(checked, []string{"acme/web"}) {
		t.Fatalf("checked %v, want the Home's acme/web checked as it is watched again", checked)
	}
	if home := changedIn(t, f.getTicket(t, "acme-5"), "acme"); home.CINotWatched || home.CIResult != core.CIRunning {
		t.Fatalf("Home %+v, want it watched again, its CI running", home)
	}
}

func TestSetCITrackingTakesAnOlderClientsHomeTargetAsTheHomesCIEntry(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.reportPushRemote(t, "git@github.com:acme/acme.git")
	if _, err := f.core.SetCITracking(human, "acme", core.SetCITrackingInput{Tracking: true}); err != nil {
		t.Fatal(err)
	}
	if ci := f.home(t).CI; ci.NotWatched || ci.Repository != "acme/acme" {
		t.Fatalf("Home's CI %+v, want it filled in from its Clone URL: set_ci_tracking's Project-wide fields leave it", ci)
	}
	if watch, err := f.core.GetCIWatch(ctx, "acme"); err != nil || watch.NotWatched || watch.Provider != domain.CIGitHubActions || watch.Repository != "acme/acme" {
		t.Fatalf("CI watch %+v (%v), want the CI Watcher to watch the Home's filled-in acme/acme", watch, err)
	}

	azure := core.SetCITrackingInput{Tracking: true, Provider: domain.CIAzurePipelines, ProjectURL: "https://dev.azure.com/acme/ops/"}
	if _, err := f.core.SetCITracking(human, "acme", azure); err != nil {
		t.Fatal(err)
	}
	want := core.CIRepositoryView{Name: "acme", Provider: domain.CIAzurePipelines, ProjectURL: "https://dev.azure.com/acme/ops"}
	if ci := f.home(t).CI; ci != want {
		t.Fatalf("Home's CI %+v, want %+v from the older client's provider and project URL", ci, want)
	}
	// A newer client names only the Project-wide fields, which leave the
	// Home's entry.
	if _, err := f.core.SetCITracking(human, "acme", core.SetCITrackingInput{IgnoredWorkflows: []string{"Nightly"}}); err != nil {
		t.Fatal(err)
	}
	p := f.projectView(t, "acme")
	if ci := repository(t, p, "acme").CI; ci != want || p.CI.Tracking {
		t.Fatalf("Home's CI %+v with tracking %v, want %+v kept and CI Tracking off", ci, p.CI.Tracking, want)
	}
	if p.CI.Provider != domain.CIAzurePipelines || p.CI.ProjectURL != "https://dev.azure.com/acme/ops" {
		t.Fatalf("CI %+v, want the Home's entry in the CI settings' own fields for an older client", p.CI)
	}
	// An older client turning CI Tracking on with nothing to watch is
	// refused, as before.
	_, err := f.core.SetCITracking(human, "acme", core.SetCITrackingInput{Tracking: true, Provider: domain.CIGitHubActions})
	wantCode(t, err, core.Invalid, "repository")

	// Saving off on GitHub Actions with nothing named, as an older client
	// does, leaves a Home the human made Not watched so.
	if _, err := f.core.SetRepositoryCI(human, "acme", core.SetRepositoryCIInput{Name: "acme", RepositoryCI: core.RepositoryCI{NotWatched: true}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.SetCITracking(human, "acme", core.SetCITrackingInput{Provider: domain.CIGitHubActions}); err != nil {
		t.Fatal(err)
	}
	if ci := f.home(t).CI; !ci.NotWatched {
		t.Fatalf("Home's CI %+v, want it still Not watched", ci)
	}
}

func TestALocalOnlyHomeNeitherWaitsOnCINorOffersLandAnywayWhileAPushedRepositoryDoes(t *testing.T) {
	f := twoRepoWorld(t)
	f.changes(t, 5, "acme", "api")
	f.reportPushRemote(t, "")

	f.pushedFor(t, "api", "acme-5", "def456")
	f.ciRunIn(t, "api", 1, "def456", "acme-5", domain.CIInProgress, "")
	v := f.getTicket(t, "acme-5")
	if api := changedIn(t, v, "api"); api.NotLanded != "waiting on CI at def456" {
		t.Fatalf("api %+v, want it waiting on its CI under Merge + Push", api)
	}
	if home := changedIn(t, v, "acme"); home.NotLanded != "" || home.CIResult != "" {
		t.Fatalf("Home %+v, want it waiting on nothing, local-only and Not watched", home)
	}

	f.ciRunIn(t, "api", 1, "def456", "acme-5", domain.CICompleted, domain.CIFailure)
	v = f.getTicket(t, "acme-5")
	if len(v.CIItems) != 1 || v.CIItems[0].Repository != "api" || !slices.Contains(v.CIItems[0].Options, core.OptionLandAnyway) {
		t.Fatalf("CI items %+v, want api's alone, offering Land anyway", v.CIItems)
	}
	_, err := f.core.LandAnyway(human, "acme", core.CIResponseInput{Ticket: "acme-5", Repository: "acme"})
	notAllowed(t, err)
	if _, err := f.core.LandAnyway(human, "acme", core.CIResponseInput{Ticket: "acme-5", Repository: "api"}); err != nil {
		t.Fatalf("Land anyway in api: %v", err)
	}
	// Under Merge, the Ticket is flagged once it hasn't landed after a
	// while, whichever Repository holds it.
	f.clock.Advance(core.NotLandedAfter)
	wantReason(t, f.getTicket(t, "acme-5"), core.ReasonAcceptedNotLanded)
}
