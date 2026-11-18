package core_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// CI Tracking across Repositories (Spec code-central-848): each
// Repository's CI entry, a Ticket's CI result per watched Repository and
// combined, CI items per Repository and pushed commit labelled Name@sha,
// and options acting on that Repository only.

// twoRepoWorld is itemWorld with a second Repository, api, on GitHub, so
// watched by GitHub Actions on acme/api: acme-5 and acme-8 Accepted, with
// abc123 pushed for both in the Home Repository.
func twoRepoWorld(t *testing.T) fixture {
	t.Helper()
	f := itemWorld(t)
	f.addRepository(t, core.AddRepositoryInput{Name: "api", CloneURL: "git@github.com:acme/api.git"})
	return f
}

// pushedFor reports, as the Factory, commit as ticket's pushed commit in
// the Repository repo, a minute after the last report.
func (f fixture) pushedFor(t *testing.T, repo, ticket, commit string) {
	t.Helper()
	f.clock.Advance(time.Minute)
	if _, err := f.core.ReportPushedCommit(ctx, "acme", core.ReportPushedCommitInput{Ticket: core.Ref(ticket), Commit: commit, Branch: "main", Repository: repo}); err != nil {
		t.Fatal(err)
	}
}

// ciRunIn reports, as the CI Watcher, attempt 1 of run id of ci.yml on
// commit in the Repository repo, tracked against ticket.
func (f fixture) ciRunIn(t *testing.T, repo string, id int64, commit, ticket string, status domain.CIRunStatus, conclusion domain.CIConclusion) {
	t.Helper()
	started := f.clock.Now()
	run := core.CIRunView{Repository: repo, ID: id, Attempt: 1, Workflow: "ci.yml", Name: "CI", Branch: "main", Commit: commit,
		Status: status, Conclusion: conclusion, Started: &started, URL: "https://github.com/acme/" + repo + "/actions/runs/1", Ticket: ticket}
	if status == domain.CICompleted {
		run.Finished = &started
	}
	if _, err := f.core.ReportCIRuns(ctx, "acme", core.ReportCIRunsInput{Runs: []core.CIRunView{run}}); err != nil {
		t.Fatal(err)
	}
}

// changedIn is ticket's Changed Repository named name, as its view shows
// it.
func changedIn(t *testing.T, v core.TicketView, name string) core.ChangedRepositoryView {
	t.Helper()
	i := slices.IndexFunc(v.Repositories, func(r core.ChangedRepositoryView) bool { return r.Name == name })
	if i < 0 {
		t.Fatalf("%s has no Changed Repository %s in %+v", v.ID, name, v.Repositories)
	}
	return v.Repositories[i]
}

// changes records repos as ticket's Changed Repositories, as a finish
// settles them.
func (f fixture) changes(t *testing.T, n int, repos ...string) {
	t.Helper()
	err := f.store.Update(func(w store.Writer) error {
		tk, _ := w.Ticket("acme", n)
		tk.Repositories = nil
		for _, r := range repos {
			tk.Repositories = append(tk.Repositories, domain.ChangedRepository{Name: r, Branch: "acme-x", Commit: "head-" + r})
		}
		return w.PutTicket("acme", tk)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAddingARepositoryFillsItsCIEntryInFromItsCloneURL(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.addRepository(t, core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git"})
	f.addRepository(t, core.AddRepositoryInput{Name: "infra", CloneURL: "https://dev.azure.com/acme/ops/_git/infra"})
	f.addRepository(t, core.AddRepositoryInput{Name: "docs", CloneURL: "https://gitlab.com/acme/docs.git"})
	p := f.addRepository(t, core.AddRepositoryInput{Name: "site", CloneURL: "git@github.com:acme/site.git", CI: &core.RepositoryCI{NotWatched: true}})
	want := []core.CIRepositoryView{
		{Name: "web", Provider: domain.CIGitHubActions, Repository: "acme/web"},
		{Name: "infra", Provider: domain.CIAzurePipelines, ProjectURL: "https://dev.azure.com/acme/ops"},
		{Name: "docs", NotWatched: true},
		{Name: "site", NotWatched: true},
	}
	if !slices.Equal(p.CI.Repositories, want) {
		t.Fatalf("CI entries %+v, want %+v", p.CI.Repositories, want)
	}

	// Removing a Repository drops its entry.
	w, err := f.core.RemoveRepository(human, "acme", core.RemoveRepositoryInput{Name: "docs", Full: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := w.Record.CI.Repositories; len(got) != 3 || slices.ContainsFunc(got, func(e core.CIRepositoryView) bool { return e.Name == "docs" }) {
		t.Fatalf("CI entries %+v, want docs gone", got)
	}
}

func TestALocalOnlyRepositoryStartsNotWatchedAndCanBeGivenACIProviderByHand(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	p := f.addRepository(t, core.AddRepositoryInput{Name: "web", LocalPath: srcPath("web")})
	if want := []core.CIRepositoryView{{Name: "web", NotWatched: true}}; !slices.Equal(p.CI.Repositories, want) {
		t.Fatalf("CI entries %+v, want %+v", p.CI.Repositories, want)
	}
	w, err := f.core.SetRepositoryCI(human, "acme", core.SetRepositoryCIInput{Name: "web", Full: true,
		RepositoryCI: core.RepositoryCI{Provider: domain.CIGitHubActions, Repository: "acme/web"}})
	if err != nil {
		t.Fatal(err)
	}
	if want := []core.CIRepositoryView{{Name: "web", Provider: domain.CIGitHubActions, Repository: "acme/web"}}; !slices.Equal(w.Record.CI.Repositories, want) {
		t.Fatalf("CI entries %+v, want %+v", w.Record.CI.Repositories, want)
	}
}

func TestAProjectWithTheHomeRepositoryAloneHasNoCIEntries(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	v, err := f.core.GetProject(human, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if v.CI.Repositories != nil {
		t.Fatalf("CI entries %+v, want none", v.CI.Repositories)
	}
}

func TestSetRepositoryCIEditsARepositorysEntry(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.addRepository(t, core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git"})
	w, err := f.core.SetRepositoryCI(human, "acme", core.SetRepositoryCIInput{Name: "web", Full: true,
		RepositoryCI: core.RepositoryCI{Provider: domain.CIAzurePipelines, ProjectURL: "https://dev.azure.com/acme/web/"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := w.Record.CI.Repositories; len(got) != 1 || got[0].Provider != domain.CIAzurePipelines || got[0].ProjectURL != "https://dev.azure.com/acme/web" {
		t.Fatalf("CI entries %+v, want web on Azure Pipelines", got)
	}
	_, err = f.core.SetRepositoryCI(human, "acme", core.SetRepositoryCIInput{Name: "nope", RepositoryCI: core.RepositoryCI{NotWatched: true}})
	wantCode(t, err, core.Invalid, "name")
	_, err = f.core.SetRepositoryCI(human, "acme", core.SetRepositoryCIInput{Name: "web", RepositoryCI: core.RepositoryCI{Repository: "web"}})
	wantCode(t, err, core.Invalid, "repository")
	_, err = f.core.SetRepositoryCI(ctx, "acme", core.SetRepositoryCIInput{Name: "web", RepositoryCI: core.RepositoryCI{NotWatched: true}})
	if err == nil {
		t.Fatal("an agent set a Repository's CI entry")
	}
}

func TestWhileCITrackingIsOnAWatchedRepositoryMustPassTheCIWatchersCheck(t *testing.T) {
	f := itemWorld(t)
	var checked []string
	f.core.SetCICheck(func(_ context.Context, in core.CICheckInput) core.WatcherCheckResult {
		checked = append(checked, in.Repository)
		if in.Repository == "acme/private" {
			return core.WatcherCheckResult{Problem: core.ProblemRepositoryUnreadable, Reason: "gh can't see acme/private", Fix: "grant gh access to acme/private"}
		}
		return core.WatcherCheckResult{}
	})

	_, err := f.core.AddRepository(human, "acme", core.AddRepositoryInput{Name: "private", CloneURL: "git@github.com:acme/private.git"})
	wantFix(t, err, core.Invalid, "ci.repository")
	if e := (*core.Error)(nil); !errors.As(err, &e) || !strings.Contains(e.Message, "gh can't see acme/private") || e.Fix != "grant gh access to acme/private" {
		t.Fatalf("refusal %v, want the check's reason and fix", err)
	}
	// Not watched needs no check.
	f.addRepository(t, core.AddRepositoryInput{Name: "private", CloneURL: "git@github.com:acme/private.git", CI: &core.RepositoryCI{NotWatched: true}})
	f.addRepository(t, core.AddRepositoryInput{Name: "api", CloneURL: "git@github.com:acme/api.git"})
	_, err = f.core.SetRepositoryCI(human, "acme", core.SetRepositoryCIInput{Name: "private", RepositoryCI: core.RepositoryCI{Repository: "acme/private"}})
	wantFix(t, err, core.Invalid, "repository")
	if !slices.Equal(checked, []string{"acme/private", "acme/api", "acme/private"}) {
		t.Fatalf("checked %v, want private, api, then private again", checked)
	}
}

func TestTurningCITrackingOnChecksEveryWatchedRepository(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.addRepository(t, core.AddRepositoryInput{Name: "api", CloneURL: "git@github.com:acme/api.git"})
	f.addRepository(t, core.AddRepositoryInput{Name: "docs", CloneURL: "git@github.com:acme/docs.git", CI: &core.RepositoryCI{NotWatched: true}})
	var checked []string
	f.core.SetCICheck(func(_ context.Context, in core.CICheckInput) core.WatcherCheckResult {
		checked = append(checked, in.Repository)
		if in.Repository == "acme/api" {
			return core.WatcherCheckResult{Problem: core.ProblemRepositoryUnreadable, Reason: "gh can't see acme/api", Fix: "grant gh access"}
		}
		return core.WatcherCheckResult{}
	})
	_, err := f.core.SetCITracking(human, "acme", core.SetCITrackingInput{Tracking: true, Repository: "acme/acme"})
	wantFix(t, err, core.Invalid, "repositories.repository")
	if !slices.Equal(checked, []string{"acme/acme", "acme/api"}) {
		t.Fatalf("checked %v, want the Home's and api's, never Not watched docs", checked)
	}
}

func TestATicketHasACIResultPerWatchedRepositoryAndACombinedOne(t *testing.T) {
	f := twoRepoWorld(t)
	f.changes(t, 5, "acme", "api")
	f.pushedFor(t, "api", "acme-5", "def456")
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", domain.CICompleted, domain.CISuccess)
	f.ciRunIn(t, "api", 1, "def456", "acme-5", domain.CIInProgress, "")

	v := f.getTicket(t, "acme-5")
	if v.CIResult != core.CIRunning || v.CICommit != "abc123" || v.PushedCommit.Commit != "abc123" {
		t.Fatalf("CI result %q from %q, pushed %+v: want running combined, with the Home's passed commit", v.CIResult, v.CICommit, v.PushedCommit)
	}
	if home, api := changedIn(t, v, "acme"), changedIn(t, v, "api"); home.CIResult != core.CIPassed || api.CIResult != core.CIRunning || api.CICommit != "def456" {
		t.Fatalf("per Repository %+v and %+v, want acme passed and api running at def456", home, api)
	}
	if len(v.CIRuns) != 2 || v.CIRuns[0].Repository != "" || v.CIRuns[1].Repository != "api" {
		t.Fatalf("runs %+v, want the Home's then api's", v.CIRuns)
	}

	// Running, else failed, else cancelled, else passed.
	f.ciRunIn(t, "api", 1, "def456", "acme-5", domain.CICompleted, domain.CIFailure)
	if v := f.getTicket(t, "acme-5"); v.CIResult != core.CIFailed || changedIn(t, v, "acme").CIResult != core.CIPassed {
		t.Fatalf("CI result %q, want failed combined while acme passed", v.CIResult)
	}
	f.ciRunIn(t, "api", 1, "def456", "acme-5", domain.CICompleted, domain.CICancelled)
	if v := f.getTicket(t, "acme-5"); v.CIResult != core.CICancelled {
		t.Fatalf("CI result %q, want cancelled", v.CIResult)
	}
	f.ciRunIn(t, "api", 1, "def456", "acme-5", domain.CICompleted, domain.CISuccess)
	if v := f.getTicket(t, "acme-5"); v.CIResult != core.CIPassed {
		t.Fatalf("CI result %q, want passed", v.CIResult)
	}
}

func TestARedBuildInOneRepositoryLeavesAnothersCIResultItsOwn(t *testing.T) {
	f := twoRepoWorld(t)
	f.changes(t, 5, "acme", "api")
	f.pushedFor(t, "api", "acme-5", "def456")
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", domain.CICompleted, domain.CISuccess)
	f.ciRunIn(t, "api", 1, "def456", "acme-5", domain.CICompleted, domain.CIFailure)

	// The Home Repository's own result, which the Factory lands it on, is
	// passed: api's red build doesn't hold it back.
	v := f.getTicket(t, "acme-5")
	if home := changedIn(t, v, "acme"); home.CIResult != core.CIPassed || home.CICommit != "abc123" || v.CICommit != "abc123" {
		t.Fatalf("Home %+v, CI commit %q: want passed at abc123", home, v.CICommit)
	}
	row := f.theItem(t, core.ReasonCIFailed)
	if row.Title != "CI failed at api@def456" || row.CIItem.Repository != "api" || row.CIItem.Commit != "def456" || row.CIResult != core.CIFailed {
		t.Fatalf("row %+v, want CI failed at api@def456", row)
	}
	if !slices.Equal(listed(row.CIItem), []string{"acme-5"}) {
		t.Fatalf("listed %v, want acme-5 alone: acme-8 pushed nothing in api", listed(row.CIItem))
	}
}

func TestNotWatchedRepositoriesNeverAffectAResult(t *testing.T) {
	f := itemWorld(t)
	f.addRepository(t, core.AddRepositoryInput{Name: "docs", CloneURL: "git@github.com:acme/docs.git", CI: &core.RepositoryCI{NotWatched: true}})
	f.changes(t, 5, "acme", "docs")
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", domain.CICompleted, domain.CISuccess)
	f.pushedFor(t, "docs", "acme-5", "fed789")
	f.ciRunIn(t, "docs", 1, "fed789", "acme-5", domain.CICompleted, domain.CIFailure)

	v := f.getTicket(t, "acme-5")
	if v.CIResult != core.CIPassed || v.NeedsMeReason != "" {
		t.Fatalf("CI result %q, reason %q: want passed, with docs never counting", v.CIResult, v.NeedsMeReason)
	}
	if docs := changedIn(t, v, "docs"); docs.CIResult != "" || !docs.CINotWatched {
		t.Fatalf("docs %+v, want Not watched with no CI result", docs)
	}
	watch, err := f.core.GetCIWatch(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(watch.Repositories) != 0 {
		t.Fatalf("watched Repositories %+v, want none: docs is Not watched", watch.Repositories)
	}

	// A Ticket that pushed only in a Not watched Repository has no CI
	// result, and is never CI unavailable.
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-1", Title: "Docs"})
	f.moveTo(t, 9, domain.Accepted)
	f.changes(t, 9, "docs")
	f.pushedFor(t, "docs", "acme-9", "aaa111")
	f.clock.Advance(core.CIUnavailableAfter + time.Minute)
	if v := f.getTicket(t, "acme-9"); v.CIResult != "" || v.CIItem != nil || v.NeedsMeReason == core.ReasonCIUnavailable {
		t.Fatalf("acme-9: CI result %q, item %+v, reason %q: want none", v.CIResult, v.CIItem, v.NeedsMeReason)
	}
}

func TestATicketFailedInTwoRepositoriesIsOnTwoItemsAndEachOptionActsOnItsRepositoryOnly(t *testing.T) {
	f := twoRepoWorld(t)
	f.changes(t, 5, "acme", "api")
	f.pushedFor(t, "api", "acme-5", "def456")
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", domain.CICompleted, domain.CIFailure)
	f.ciRunIn(t, "api", 1, "def456", "acme-5", domain.CICompleted, domain.CIFailure)

	var titles []string
	for _, row := range f.needsMe(t) {
		titles = append(titles, row.Title)
	}
	if !slices.Equal(titles, []string{"CI failed at acme@abc123", "CI failed at api@def456"}) {
		t.Fatalf("needs me %v, want one item per Repository, labelled Name@sha", titles)
	}
	v := f.getTicket(t, "acme-5")
	if len(v.CIItems) != 2 || v.CIItems[0].Repository != "acme" || v.CIItems[1].Repository != "api" || v.CIItem.Repository != "acme" {
		t.Fatalf("items %+v, first %+v: want acme's then api's", v.CIItems, v.CIItem)
	}

	// With two items, the Repository must be named.
	_, err := f.core.LandAnyway(human, "acme", core.CIResponseInput{Ticket: "acme-5"})
	wantCode(t, err, core.Invalid, "repository")

	if _, err := f.core.LandAnyway(human, "acme", core.CIResponseInput{Ticket: "acme-5", Repository: "api"}); err != nil {
		t.Fatal(err)
	}
	v = f.getTicket(t, "acme-5")
	if api := changedIn(t, v, "api"); !api.LandAnyway || v.LandAnyway || changedIn(t, v, "acme").LandAnyway {
		t.Fatalf("land anyway: api %v, Home %v: want api's alone", api.LandAnyway, v.LandAnyway)
	}
	if rows := f.needsMe(t); len(rows) != 1 || rows[0].Title != "CI failed at acme@abc123" {
		t.Fatalf("needs me %+v, want only acme's item left", rows)
	}

	// Re-run failed jobs re-runs the Home's commit's runs only.
	if _, err := f.core.RerunCI(human, "acme", core.CIResponseInput{Ticket: "acme-5"}); err != nil {
		t.Fatal(err)
	}
	watch, err := f.core.GetCIWatch(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(watch.Reruns) != 1 || watch.Reruns[0].Commit != "abc123" || len(watch.Repositories) != 1 || len(watch.Repositories[0].Reruns) != 0 {
		t.Fatalf("reruns %+v, api's %+v: want the Home's abc123 alone", watch.Reruns, watch.Repositories)
	}
}

func TestNewFixTicketNamesTheRepository(t *testing.T) {
	f := twoRepoWorld(t)
	f.changes(t, 5, "acme", "api")
	f.pushedFor(t, "api", "acme-5", "def456")
	f.ciRunIn(t, "api", 1, "def456", "acme-5", domain.CICompleted, domain.CIFailure)
	w, err := f.core.NewFixTicket(human, "acme", core.NewFixTicketInput{Ticket: "acme-5", Note: "the api tests fail", Full: true})
	if err != nil {
		t.Fatal(err)
	}
	if fix := w.Record; !strings.HasPrefix(fix.Title, "Fix the CI failure of acme-5 in api: ") || !strings.HasPrefix(fix.Description, "acme-5's CI failed in api: ") {
		t.Fatalf("fix Ticket %q: %q, want it to name api", fix.Title, fix.Description)
	}
	if rows := f.needsMe(t); len(rows) != 0 {
		t.Fatalf("needs me %+v, want the item left out while its fix Ticket is open", rows)
	}
}

func TestTheCIWatchHasEachWatchedRepositorysTargetAndPushedCommits(t *testing.T) {
	f := twoRepoWorld(t)
	f.changes(t, 5, "acme", "api")
	f.pushedFor(t, "api", "acme-5", "def456")
	watch, err := f.core.GetCIWatch(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if watch.Repository != "acme/web" || len(watch.Waiting) != 1 || watch.Waiting[0].Commit != "abc123" {
		t.Fatalf("Home %s waiting %+v, want acme/web waiting on abc123", watch.Repository, watch.Waiting)
	}
	if len(watch.Repositories) != 1 {
		t.Fatalf("Repositories %+v, want api", watch.Repositories)
	}
	api := watch.Repositories[0]
	if api.Name != "api" || api.Provider != domain.CIGitHubActions || api.Repository != "acme/api" || len(api.Waiting) != 1 || api.Waiting[0].Commit != "def456" || api.Waiting[0].Ticket != "acme-5" {
		t.Fatalf("api %+v, want acme/api waiting on def456 for acme-5", api)
	}
	if i := slices.IndexFunc(watch.Pushed, func(c core.CITicketCommit) bool { return c.Commit == "def456" }); i < 0 || watch.Pushed[i].Repository != "api" {
		t.Fatalf("pushed %+v, want def456 in api", watch.Pushed)
	}
}

func TestReportsNamingNoRepositoryOfTheProjectAreRefused(t *testing.T) {
	f := twoRepoWorld(t)
	_, err := f.core.ReportPushedCommit(ctx, "acme", core.ReportPushedCommitInput{Ticket: "acme-5", Commit: "fed", Repository: "nope"})
	wantCode(t, err, core.Invalid, "repository")
	run := core.CIRunView{Repository: "nope", ID: 1, Attempt: 1, Workflow: "ci.yml", Commit: "fed", Status: domain.CIQueued, Ticket: "acme-5"}
	_, err = f.core.ReportCIRuns(ctx, "acme", core.ReportCIRunsInput{Runs: []core.CIRunView{run}})
	wantCode(t, err, core.Invalid, "runs[0].repository")
}

func TestRunsInTwoRepositoriesWithOneRunIDAreTwoRuns(t *testing.T) {
	f := twoRepoWorld(t)
	f.changes(t, 5, "acme", "api")
	f.pushedFor(t, "api", "acme-5", "def456")
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", domain.CICompleted, domain.CISuccess)
	f.ciRunIn(t, "api", 1, "def456", "acme-5", domain.CICompleted, domain.CIFailure)
	if v := f.getTicket(t, "acme-5"); len(v.CIRuns) != 2 || changedIn(t, v, "acme").CIResult != core.CIPassed {
		t.Fatalf("runs %+v, want each Repository's run 1 kept", v.CIRuns)
	}
}

func TestTheCIWatcherInErrorAboutOneRepositoryStillWatchesTheOthers(t *testing.T) {
	f := twoRepoWorld(t)
	f.changes(t, 5, "acme", "api")
	f.pushedFor(t, "api", "acme-5", "def456")
	w, err := f.core.ReportCIWatcherStatus(ctx, "acme", core.ReportWatcherStatusInput{State: core.WatcherError, Full: true,
		Repositories: []core.RepositoryProblem{{Repository: "api", Problem: core.ProblemRepositoryUnreadable, Message: "gh can't read acme/api", Fix: "grant gh access to acme/api"}}})
	if err != nil {
		t.Fatal(err)
	}
	watcher := w.Record.CI.Watcher
	if watcher.State != core.WatcherError || watcher.Message != "api: gh can't read acme/api" || len(watcher.Repositories) != 1 || watcher.Repositories[0].Fix != "grant gh access to acme/api" {
		t.Fatalf("watcher %+v, want in error about api, with its fix", watcher)
	}
	f.clock.Advance(core.CIUnavailableAfter + time.Minute)
	// The Home Repository is watched: its commit has no runs yet. api isn't.
	rows := f.needsMe(t)
	if len(rows) != 2 {
		t.Fatalf("needs me %+v, want one CI unavailable item per Repository", rows)
	}
	for _, row := range rows {
		want := core.CINoRuns
		if row.CIItem.Repository == "api" {
			want = core.CIWatcherNotWatching
		}
		if row.Reason != core.ReasonCIUnavailable || row.CIUnavailable.Cause != want {
			t.Errorf("%s: %s because %s, want CI unavailable because %s", row.Title, row.Reason, row.CIUnavailable.Cause, want)
		}
	}

	_, err = f.core.ReportCIWatcherStatus(ctx, "acme", core.ReportWatcherStatusInput{State: core.WatcherError,
		Repositories: []core.RepositoryProblem{{Repository: "nope", Message: "x"}}})
	wantCode(t, err, core.Invalid, "repositories[0].repository")
}

func TestASecondRepositoryFailingWhileInErrorSendsNoSecondNotification(t *testing.T) {
	f := twoRepoWorld(t)
	report := func(names ...string) {
		t.Helper()
		in := core.ReportWatcherStatusInput{State: core.WatcherError}
		for _, n := range names {
			in.Repositories = append(in.Repositories, core.RepositoryProblem{Repository: n, Problem: core.ProblemRepositoryUnreadable, Message: "can't read " + n})
		}
		if _, err := f.core.ReportCIWatcherStatus(ctx, "acme", in); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.core.ReportCIWatcherStatus(ctx, "acme", core.ReportWatcherStatusInput{State: core.WatcherWatching}); err != nil {
		t.Fatal(err)
	}
	report("api")
	report("acme", "api")
	for _, a := range f.core.WatcherAttention(ctx) {
		if a.Kind == core.WatcherCI && a.Errors != 1 {
			t.Fatalf("errors %d, want one Notification for going into error", a.Errors)
		}
	}
}
