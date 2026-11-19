package core_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

// Pull requests per Repository (Spec code-central-848, ADR 0050): one
// pull request per Changed Repository, into that Repository's own Base,
// each body listing its siblings; each merge lands its Repository; send
// back closes the open siblings while none has merged; Cancel is refused
// once one has; and the Pull Request Watcher's status and check name each
// failing Repository.

// pullRequestsAcross is acme under Pull Request with Repositories web (on
// GitHub), acme (the Home Repository, on GitHub) and api (on Azure
// Repos), in that landing order, and acme-2 Accepted with work in acme
// and api, each pushed as acme-2.
func (f fixture) pullRequestsAcross(t *testing.T) {
	t.Helper()
	f.work(t, domain.Ready)
	f.acrossRepositories(t)
}

// acrossRepositories is pullRequestsAcross for acme's Ready acme-2.
func (f fixture) acrossRepositories(t *testing.T) {
	t.Helper()
	if _, err := f.core.ReportPushRemote(ctx, "acme", api.ReportPushRemoteInput{URL: "git@github.com:acme/acme.git", Base: "main"}); err != nil {
		t.Fatal(err)
	}
	f.addRepository(t, api.AddRepositoryInput{Name: "api", CloneURL: "https://dev.azure.com/acme/shop/_git/api", Base: "develop"})
	f.addRepository(t, api.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git"})
	if _, err := f.core.SetLandingOrder(human, "acme", api.SetLandingOrderInput{Names: []string{"web", "acme", "api"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.SetLandingMethod(human, "acme", api.SetLandingMethodInput{Method: domain.LandingPullRequest}); err != nil {
		t.Fatal(err)
	}
	f.claim(t, api.ClaimInput{})
	f.finishWith(t, api.FinishInput{Branch: "acme-2", Commit: "aaa", Repositories: []api.RepositoryWork{{Name: "api", Branch: "acme-2", Commit: "bbb"}}})
	if _, err := f.core.Accept(human, "acme", api.TicketInput{Ticket: "2"}); err != nil {
		t.Fatalf("accept: %v", err)
	}
	f.pushedIn(t, "", "aaa")
	f.pushedIn(t, "api", "bbb")
}

// pushedIn reports commit pushed as acme-2 for acme-2 in repository.
func (f fixture) pushedIn(t *testing.T, repository, commit string) {
	t.Helper()
	f.clock.Advance(time.Minute)
	if _, err := f.core.ReportPushedCommit(ctx, "acme", api.ReportPushedCommitInput{Ticket: "2", Commit: commit, Branch: "acme-2", Repository: repository}); err != nil {
		t.Fatalf("report_pushed_commit %s %s: %v", repository, commit, err)
	}
}

// reportIn reports pull request n in repository, in state.
func (f fixture) reportIn(t *testing.T, repository string, n int, state domain.PullRequestState, merge string) {
	t.Helper()
	link := "https://github.com/acme/acme/pull/" + string(rune('0'+n))
	if repository == "api" {
		link = "https://dev.azure.com/acme/shop/_git/api/pullrequest/" + string(rune('0'+n))
	}
	if _, err := f.core.ReportPullRequest(ctx, "acme", api.ReportPullRequestInput{Repository: repository, Branch: "acme-2", Number: n, Link: link, State: state, MergeCommit: merge}); err != nil {
		t.Fatalf("report_pull_request %s %d: %v", repository, n, err)
	}
}

// workIn is the watch's work in repository.
func workIn(t *testing.T, w api.PullRequestWatch, repository string) api.PushedWorkView {
	t.Helper()
	for _, x := range w.Work {
		if x.Repository == repository {
			return x
		}
	}
	t.Fatalf("no work in %s among %+v", repository, w.Work)
	return api.PushedWorkView{}
}

func TestThePullRequestWatchHasOnePieceOfWorkPerChangedRepositoryOnItsOwnCodeHost(t *testing.T) {
	f := newFixture(t)
	f.pullRequestsAcross(t)
	w := f.watch(t)

	want := []api.PullRequestRepositoryView{
		{Name: "web", CodeHost: api.CodeHostGitHub, Repository: "acme/web", Base: "main"},
		{Name: "acme", CodeHost: api.CodeHostGitHub, Repository: "acme/acme", Base: "main"},
		{Name: "api", CodeHost: api.CodeHostAzureRepos, Repository: "acme/shop/api", Base: "develop"},
	}
	if len(w.Repositories) != len(want) {
		t.Fatalf("repositories = %+v, want %+v", w.Repositories, want)
	}
	for i := range want {
		if w.Repositories[i] != want[i] {
			t.Errorf("repository %d = %+v, want %+v", i, w.Repositories[i], want[i])
		}
	}
	if w.CodeHost != api.CodeHostGitHub || w.Repository != "acme/acme" || w.Base != "main" {
		t.Errorf("top-level host %q repository %q base %q, want the Home Repository's", w.CodeHost, w.Repository, w.Base)
	}
	if len(w.Work) != 2 {
		t.Fatalf("work = %+v, want one piece in acme and one in api", w.Work)
	}
	home, apiRepo := workIn(t, w, "acme"), workIn(t, w, "api")
	if home.Branch != "acme-2" || home.Commit != "aaa" || apiRepo.Branch != "acme-2" || apiRepo.Commit != "bbb" {
		t.Errorf("work acme %+v, api %+v", home, apiRepo)
	}
}

func TestEachPullRequestsBodyListsItsSiblingsInLandingOrderKeptUpToDate(t *testing.T) {
	f := newFixture(t)
	f.pullRequestsAcross(t)

	body := workIn(t, f.watch(t), "acme").Body
	if !strings.HasSuffix(body, "## Pull requests\n\n- acme: this pull request\n- api: not opened yet") {
		t.Fatalf("Home body = %q, want its siblings listed with api not opened yet", body)
	}
	if !strings.Contains(body, "Ticket acme-2 in") {
		t.Errorf("Home body = %q, want the Ticket named", body)
	}

	f.reportIn(t, "api", 4, domain.PROpen, "")
	w := f.watch(t)
	if body := workIn(t, w, "acme").Body; !strings.HasSuffix(body, "- acme: this pull request\n- api: https://dev.azure.com/acme/shop/_git/api/pullrequest/4") {
		t.Errorf("Home body = %q, want api's pull request linked once it is open", body)
	}
	if body := workIn(t, w, "api").Body; !strings.HasSuffix(body, "- acme: not opened yet\n- api: this pull request") {
		t.Errorf("api body = %q", body)
	}
}

func TestAPullRequestBodyMentionsTheSpecsLinkedIssueWithNoClosingKeyword(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	integration := api.WithActor(context.Background(), api.Integration)
	s, err := f.core.CreateLinkedSpec(integration, "acme", api.CreateLinkedSpecInput{Title: "Issue", Full: true,
		Link: api.LinkInput{Integration: "github", ID: "acme/acme#12", URL: "https://github.com/acme/acme/issues/12"}})
	if err != nil {
		t.Fatal(err)
	}
	f.longLived(t, *s.Record)
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "1", Title: "Ta", State: domain.Ready})
	f.acrossRepositories(t)
	w := f.watch(t)
	home, apiRepo := workIn(t, w, "acme").Body, workIn(t, w, "api").Body
	if !strings.Contains(home, "Issue: acme/acme#12") {
		t.Errorf("GitHub body = %q, want the issue as owner/repo#n", home)
	}
	if !strings.Contains(apiRepo, "Issue: https://github.com/acme/acme/issues/12") {
		t.Errorf("Azure Repos body = %q, want the issue's URL", apiRepo)
	}
	for _, b := range []string{home, apiRepo} {
		if strings.Contains(strings.ToLower(b), "close") || strings.Contains(strings.ToLower(b), "fixes") {
			t.Errorf("body = %q, want no closing keyword", b)
		}
	}
}

func TestEachRepositorysPullRequestIsRecordedOnItsOwn(t *testing.T) {
	f := newFixture(t)
	f.pullRequestsAcross(t)
	f.reportIn(t, "", 7, domain.PROpen, "")
	f.reportIn(t, "api", 4, domain.PRConflicting, "")

	tk := f.get(t, "2")
	if tk.PullRequest == nil || tk.PullRequest.Number != 7 {
		t.Errorf("top-level pull request = %+v, want the Home Repository's", tk.PullRequest)
	}
	for _, r := range tk.Repositories {
		switch r.Name {
		case "acme":
			if r.PullRequest == nil || r.PullRequest.Number != 7 {
				t.Errorf("acme's pull request = %+v", r.PullRequest)
			}
		case "api":
			if r.PullRequest == nil || r.PullRequest.Number != 4 || r.PullRequest.State != domain.PRConflicting {
				t.Errorf("api's pull request = %+v", r.PullRequest)
			}
		}
	}
	if w := workIn(t, f.watch(t), "api"); w.PullRequest == nil || w.PullRequest.Number != 4 {
		t.Errorf("api's work = %+v, want its pull request", w)
	}

	_, err := f.core.ReportPullRequest(ctx, "acme", api.ReportPullRequestInput{Repository: "nope", Branch: "acme-2", Number: 1, Link: "l", State: domain.PROpen})
	wantCode(t, err, api.Invalid, "repository")
	_, err = f.core.ReportPullRequest(ctx, "acme", api.ReportPullRequestInput{Repository: "web", Branch: "acme-2", Number: 1, Link: "l", State: domain.PROpen})
	wantCode(t, err, api.NotFound, "")
}

func TestThePullRequestsMergeInAnyOrderEachLandingItsRepository(t *testing.T) {
	f := newFixture(t)
	f.pullRequestsAcross(t)
	f.reportIn(t, "api", 4, domain.PRMerged, "m-api")
	f.reportIn(t, "", 7, domain.PROpen, "")

	tk, err := record(f.core.Land(ctx, "acme", api.LandInput{Ticket: "2", Commit: "m-api", Repository: "api", Full: true}))
	if err != nil {
		t.Fatalf("land api: %v", err)
	}
	if tk.State != domain.Accepted || !tk.PartlyLanded {
		t.Fatalf("after api lands: %s, partly landed %v, want Accepted and Partly landed", tk.State, tk.PartlyLanded)
	}
	f.reportIn(t, "", 7, domain.PRMerged, "m-acme")
	tk, err = record(f.core.Land(ctx, "acme", api.LandInput{Ticket: "2", Commit: "m-acme", Repository: "acme", Full: true}))
	if err != nil {
		t.Fatalf("land acme: %v", err)
	}
	if tk.State != domain.Done {
		t.Fatalf("after both land: %s, want Done", tk.State)
	}
	for _, r := range tk.Repositories {
		want := map[string]string{"acme": "m-acme", "api": "m-api"}[r.Name]
		if r.Landed == nil || r.Landed.Commit != want {
			t.Errorf("%s landed %+v, want at %s", r.Name, r.Landed, want)
		}
	}

	_, err = f.core.Land(ctx, "acme", api.LandInput{Ticket: "2", Commit: "x", Repository: "web"})
	if err == nil {
		t.Error("land in a Repository on a Done Ticket succeeded")
	}
}

func TestLandingInARepositoryTheTicketDidntChangeIsRefused(t *testing.T) {
	f := newFixture(t)
	f.pullRequestsAcross(t)
	_, err := f.core.Land(ctx, "acme", api.LandInput{Ticket: "2", Commit: "x", Repository: "web"})
	wantCode(t, err, api.Invalid, "repository")
	_, err = f.core.Land(ctx, "acme", api.LandInput{Ticket: "2", Commit: "x", Repository: "nope"})
	wantCode(t, err, api.Invalid, "repository")
}

func TestPullRequestClosedNamesEachClosedPullRequestsRepository(t *testing.T) {
	f := newFixture(t)
	f.pullRequestsAcross(t)
	f.reportIn(t, "", 7, domain.PROpen, "")
	f.reportIn(t, "api", 4, domain.PRClosed, "")

	var row *api.NeedsMeRow
	for _, r := range f.needsMe(t) {
		if r.ID == "acme-2" {
			row = &r
		}
	}
	if row == nil || row.Reason != api.ReasonPullRequestClosed {
		t.Fatalf("row = %+v, want Pull request closed", row)
	}
	if len(row.ClosedPullRequests) != 1 || row.ClosedPullRequests[0].Repository != "api" || row.ClosedPullRequests[0].Number != 4 {
		t.Errorf("closed pull requests = %+v, want api's", row.ClosedPullRequests)
	}

	// Reopened, it clears.
	f.reportIn(t, "api", 4, domain.PROpen, "")
	wantReason(t, f.get(t, "2"), api.ReasonToMerge)
}

func TestSendingBackBeforeAnyMergeClosesTheOpenPullRequests(t *testing.T) {
	f := newFixture(t)
	f.pullRequestsAcross(t)
	f.reportIn(t, "", 7, domain.PROpen, "")
	f.reportIn(t, "api", 4, domain.PRClosed, "")

	if _, err := f.core.SendBack(human, "acme", api.SendBackInput{Ticket: "2", Note: "Redo the API."}); err != nil {
		t.Fatalf("send_back: %v", err)
	}
	w := f.watch(t)
	if len(w.Work) != 0 {
		t.Errorf("work = %+v, want none for a Ready Ticket", w.Work)
	}
	if len(w.Close) != 1 || w.Close[0].Repository != "acme" || w.Close[0].Number != 7 || w.Close[0].Branch != "acme-2" {
		t.Fatalf("close = %+v, want acme's open pull request", w.Close)
	}

	f.reportIn(t, "", 7, domain.PRClosed, "")
	if w := f.watch(t); len(w.Close) != 0 {
		t.Errorf("close = %+v after it closed, want none", w.Close)
	}
}

func TestSendingBackAfterASiblingMergedKeepsItsLandedCommitAndClosesNothing(t *testing.T) {
	f := newFixture(t)
	f.pullRequestsAcross(t)
	f.reportIn(t, "api", 4, domain.PRMerged, "m-api")
	f.reportIn(t, "", 7, domain.PRClosed, "")
	if _, err := f.core.Land(ctx, "acme", api.LandInput{Ticket: "2", Commit: "m-api", Repository: "api"}); err != nil {
		t.Fatalf("land api: %v", err)
	}

	if _, err := f.core.SendBack(human, "acme", api.SendBackInput{Ticket: "2", Note: "Redo the Home part."}); err != nil {
		t.Fatalf("send_back: %v", err)
	}
	if w := f.watch(t); len(w.Close) != 0 {
		t.Errorf("close = %+v, want nothing closed once a sibling merged", w.Close)
	}

	// The next Claim's finish names only the Home Repository: api's branch
	// was moved to its Base tip and holds nothing new.
	res := f.claim(t, api.ClaimInput{Ticket: "2"})
	fin, err := f.core.Finish(ctx, "acme", api.FinishInput{Ticket: "2", ClaimID: res.ClaimID, Summary: "Redone.", Branch: "acme-2", Commit: "ccc", Full: true})
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	tk := *fin.Ticket
	if got := changedNames(tk); len(got) != 2 || got[0] != "acme" || got[1] != "api" {
		t.Fatalf("Changed Repositories %v, want acme and api still", got)
	}
	if apiRepo := tk.Repositories[1]; apiRepo.Landed == nil || apiRepo.Landed.Commit != "m-api" {
		t.Errorf("api = %+v, want its landed commit kept", apiRepo)
	}
	if home := tk.Repositories[0]; home.Commit != "ccc" || home.Landed != nil {
		t.Errorf("acme = %+v, want the new work, not landed", home)
	}
}

func TestNewWorkInALandedRepositoryNeedsLandingAgain(t *testing.T) {
	f := newFixture(t)
	f.pullRequestsAcross(t)
	if _, err := f.core.Land(ctx, "acme", api.LandInput{Ticket: "2", Commit: "m-api", Repository: "api"}); err != nil {
		t.Fatalf("land api: %v", err)
	}
	if _, err := f.core.SendBack(human, "acme", api.SendBackInput{Ticket: "2", Note: "More."}); err != nil {
		t.Fatalf("send_back: %v", err)
	}
	res := f.claim(t, api.ClaimInput{Ticket: "2"})
	fin, err := f.core.Finish(ctx, "acme", api.FinishInput{Ticket: "2", ClaimID: res.ClaimID, Summary: "More.", Full: true,
		Repositories: []api.RepositoryWork{{Name: "api", Branch: "acme-2", Commit: "ddd"}}})
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	for _, r := range fin.Ticket.Repositories {
		if r.Name == "api" && r.Landed != nil {
			t.Errorf("api = %+v, want new work there not landed", r)
		}
	}
}

func TestCancelIsRefusedOnceAnyPullRequestHasMerged(t *testing.T) {
	f := newFixture(t)
	f.pullRequestsAcross(t)
	f.reportIn(t, "", 7, domain.PRClosed, "")
	f.reportIn(t, "api", 4, domain.PRMerged, "m-api")

	_, err := f.core.Cancel(human, "acme", api.CancelInput{Ticket: "2"})
	wantCode(t, err, api.ActionNotAllowed, "")
	var e *api.Error
	if errors.As(err, &e) && !strings.Contains(e.Message, "api") {
		t.Errorf("refusal %q, want the merged Repository named", e.Message)
	}
	if got := f.get(t, "2").State; got != domain.Accepted {
		t.Errorf("state %s, want still Accepted", got)
	}
}

func TestAPushedCommitIsRecordedInItsRepository(t *testing.T) {
	f := newFixture(t)
	f.pullRequestsAcross(t)
	tk := f.get(t, "2")
	for _, r := range tk.Repositories {
		want := map[string]string{"acme": "aaa", "api": "bbb"}[r.Name]
		if r.PushedCommit == nil || r.PushedCommit.Commit != want {
			t.Errorf("%s pushed %+v, want %s", r.Name, r.PushedCommit, want)
		}
	}
	// The same commit again in its Repository records nothing new.
	before := len(f.get(t, "2").Moves)
	f.pushedIn(t, "api", "bbb")
	if w := f.watch(t); workIn(t, w, "api").Commit != "bbb" || len(f.get(t, "2").Moves) != before {
		t.Errorf("work %+v after the same commit again", w.Work)
	}
	_, err := f.core.ReportPushedCommit(ctx, "acme", api.ReportPushedCommitInput{Ticket: "2", Commit: "x", Repository: "nope"})
	wantCode(t, err, api.Invalid, "repository")
}

func TestTheWatcherNamesEachFailingRepositoryAndNotifiesOnceOnEnteringError(t *testing.T) {
	f := newFixture(t)
	f.pullRequestsAcross(t)
	report := func(in api.ReportWatcherStatusInput) error {
		_, err := f.core.ReportPullRequestWatcherStatus(ctx, "acme", in)
		return err
	}
	apiRepo := api.RepositoryProblem{Repository: "api", Problem: api.ProblemPATMissing, Message: "ADO_PAT isn't set", Fix: "set ADO_PAT"}
	if err := report(api.ReportWatcherStatusInput{State: api.WatcherError, Problem: apiRepo.Problem, Message: "api: " + apiRepo.Message, Repositories: []api.RepositoryProblem{apiRepo}}); err != nil {
		t.Fatalf("report: %v", err)
	}
	v := f.projectView(t, "acme").Landing.Watcher
	if v.State != api.WatcherError || len(v.Repositories) != 1 || v.Repositories[0] != apiRepo {
		t.Fatalf("watcher = %+v, want in error naming api", v)
	}
	web := api.RepositoryProblem{Repository: "web", Problem: api.ProblemRepositoryUnreadable, Message: "gh can't read acme/web"}
	if err := report(api.ReportWatcherStatusInput{State: api.WatcherError, Problem: apiRepo.Problem, Message: "two", Repositories: []api.RepositoryProblem{web, apiRepo}}); err != nil {
		t.Fatalf("report: %v", err)
	}
	if v := f.projectView(t, "acme").Landing.Watcher; len(v.Repositories) != 2 {
		t.Errorf("watcher = %+v, want both named", v)
	}
	for _, a := range f.core.WatcherAttention(ctx) {
		if a.Project == "acme" && a.Kind == api.WatcherPullRequests && a.Errors != 1 {
			t.Errorf("errors = %d, want one Notification for entering error, none for a further Repository", a.Errors)
		}
	}

	err := report(api.ReportWatcherStatusInput{State: api.WatcherError, Message: "m", Repositories: []api.RepositoryProblem{{Repository: "nope", Message: "m"}}})
	wantCode(t, err, api.Invalid, "repositories[0].repository")
	err = report(api.ReportWatcherStatusInput{State: api.WatcherError, Message: "m", Repositories: []api.RepositoryProblem{{Repository: "api"}}})
	wantCode(t, err, api.Invalid, "repositories[0].message")
	err = report(api.ReportWatcherStatusInput{State: api.WatcherWatching, Repositories: []api.RepositoryProblem{apiRepo}})
	wantCode(t, err, api.Invalid, "repositories")
}

func TestTurningPullRequestOnChecksEveryRepositoryNamingEachThatFails(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	if _, err := f.core.ReportPushRemote(ctx, "acme", api.ReportPushRemoteInput{URL: "git@github.com:acme/acme.git", Base: "main"}); err != nil {
		t.Fatal(err)
	}
	f.addRepository(t, api.AddRepositoryInput{Name: "api", CloneURL: "https://dev.azure.com/acme/shop/_git/api"})
	f.addRepository(t, api.AddRepositoryInput{Name: "docs", CloneURL: "https://gitlab.com/acme/docs.git"})
	var checked api.PullRequestCheckInput
	f.core.SetPullRequestCheck(func(_ context.Context, in api.PullRequestCheckInput) api.WatcherCheckResult {
		checked = in
		p := api.RepositoryProblem{Repository: "api", Problem: api.ProblemPATMissing, Message: "ADO_PAT isn't set", Fix: "set ADO_PAT"}
		return api.WatcherCheckResult{Problem: p.Problem, Reason: p.Message, Fix: p.Fix, Repositories: []api.RepositoryProblem{p}}
	})

	_, err := f.core.SetLandingMethod(human, "acme", api.SetLandingMethodInput{Method: domain.LandingPullRequest})
	wantFix(t, err, api.Invalid, "method")
	var e *api.Error
	if !errors.As(err, &e) || !strings.Contains(e.Message, "docs: ") || !strings.Contains(e.Message, "api: ADO_PAT isn't set") || !strings.Contains(e.Fix, "api: set ADO_PAT") {
		t.Fatalf("refusal %+v, want docs and api named with their reasons and fixes", err)
	}
	want := []api.PullRequestCheckRepository{
		{Name: "acme", CodeHost: api.CodeHostGitHub, Repository: "acme/acme"},
		{Name: "api", CodeHost: api.CodeHostAzureRepos, Repository: "acme/shop/api"},
	}
	if len(checked.Repositories) != 2 || checked.Repositories[0] != want[0] || checked.Repositories[1] != want[1] {
		t.Errorf("checked %+v, want every Repository on a supported host", checked.Repositories)
	}
	if v := f.projectView(t, "acme"); v.Landing.Method == domain.LandingPullRequest {
		t.Error("Pull Request turned on despite the refusal")
	}
}

func TestTurningPullRequestOnIsRefusedNamingEachLocalOnlyRepositoryBeforeTheCheck(t *testing.T) {
	for name, withCheck := range map[string]bool{"with the check": true, "with no check": false} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.project(t, "acme")
			if _, err := f.core.ReportPushRemote(ctx, "acme", api.ReportPushRemoteInput{URL: "git@github.com:acme/acme.git", Base: "main"}); err != nil {
				t.Fatal(err)
			}
			f.addRepository(t, api.AddRepositoryInput{Name: "notes", LocalPath: srcPath("notes")})
			f.addRepository(t, api.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git"})
			f.addRepository(t, api.AddRepositoryInput{Name: "docs", LocalPath: srcPath("docs")})
			checked := false
			if withCheck {
				f.core.SetPullRequestCheck(func(context.Context, api.PullRequestCheckInput) api.WatcherCheckResult {
					checked = true
					return api.WatcherCheckResult{}
				})
			}

			_, err := f.core.SetLandingMethod(human, "acme", api.SetLandingMethodInput{Method: domain.LandingPullRequest})
			wantFix(t, err, api.Invalid, "method")
			var e *api.Error
			errors.As(err, &e)
			message := "2 Repositories fail: notes: local-only, with no Clone URL, and following the Default Landing, so it can't land by Pull Request; " +
				"docs: local-only, with no Clone URL, and following the Default Landing, so it can't land by Pull Request"
			fix := "notes: give notes an Own Landing such as Merge, or a Clone URL; docs: give docs an Own Landing such as Merge, or a Clone URL"
			if e.Message != message || e.Fix != fix {
				t.Fatalf("refusal = %q, fix %q, want %q, fix %q", e.Message, e.Fix, message, fix)
			}
			if checked {
				t.Error("the Pull Request check ran despite the local-only Repositories")
			}
			if v := f.projectView(t, "acme"); v.Landing.Method == domain.LandingPullRequest {
				t.Error("Pull Request turned on despite the refusal")
			}

			// Given a Clone URL each, Pull Request turns on.
			for _, n := range []string{"notes", "docs"} {
				url := "git@github.com:acme/" + n + ".git"
				if _, err := f.core.EditRepository(human, "acme", api.EditRepositoryInput{Name: n, CloneURL: &url}); err != nil {
					t.Fatalf("edit %s: %v", n, err)
				}
			}
			if _, err := f.core.SetLandingMethod(human, "acme", api.SetLandingMethodInput{Method: domain.LandingPullRequest}); err != nil {
				t.Fatalf("turn Pull Request on: %v", err)
			}
		})
	}
}

func TestTurningPullRequestOnNamesALoneLocalOnlyRepository(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	if _, err := f.core.ReportPushRemote(ctx, "acme", api.ReportPushRemoteInput{URL: "git@github.com:acme/acme.git", Base: "main"}); err != nil {
		t.Fatal(err)
	}
	f.addRepository(t, api.AddRepositoryInput{Name: "notes", LocalPath: srcPath("notes")})

	_, err := f.core.SetLandingMethod(human, "acme", api.SetLandingMethodInput{Method: domain.LandingPullRequest})
	wantFix(t, err, api.Invalid, "method")
	var e *api.Error
	if errors.As(err, &e); e.Message != "notes: local-only, with no Clone URL, and following the Default Landing, so it can't land by Pull Request" || e.Fix != "notes: give notes an Own Landing such as Merge, or a Clone URL" {
		t.Fatalf("refusal = %q, fix %q, want notes named", e.Message, e.Fix)
	}
}

func TestSendingBackATicketWithOnePullRequestLeavesItOpenForTheNextPush(t *testing.T) {
	f := newFixture(t)
	f.pullRequestAccepted(t)
	f.reportPR(t, api.ReportPullRequestInput{Branch: "acme-2", State: domain.PROpen})
	if _, err := f.core.SendBack(human, "acme", api.SendBackInput{Ticket: "2", Note: "Redo it."}); err != nil {
		t.Fatalf("send_back: %v", err)
	}
	if w := f.watch(t); len(w.Close) != 0 {
		t.Errorf("close = %+v, want the one pull request left open", w.Close)
	}
}
