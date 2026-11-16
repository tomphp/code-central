package core_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// A Ticket's Changed Repositories (Spec code-central-848, ADR 0046):
// reported live while it is In Progress, settled by each finish, failed by
// the Factory's finish check, and shown only while the Project has more
// than one Repository.

// severalRepositories is a Project acme whose Repositories are, in
// landing order, web, acme (the Home Repository) and api, with acme-2
// claimed under claim-1.
func (f fixture) severalRepositories(t *testing.T) {
	t.Helper()
	f.work(t, store.Ready)
	f.addRepository(t, core.AddRepositoryInput{Name: "api", CloneURL: "git@github.com:acme/api.git"})
	f.addRepository(t, core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git"})
	if _, err := f.core.SetLandingOrder(human, "acme", core.SetLandingOrderInput{Names: []string{"web", "acme", "api"}}); err != nil {
		t.Fatalf("set_landing_order: %v", err)
	}
	f.claim(t, core.ClaimInput{})
}

// changedNames is the Names of tk's Changed Repositories, in order.
func changedNames(tk core.TicketView) []string {
	var out []string
	for _, r := range tk.Repositories {
		out = append(out, r.Name)
	}
	return out
}

// finishWith finishes acme-2 under claim-1 with in's branch, commit and
// repositories, returning the Ticket.
func (f fixture) finishWith(t *testing.T, in core.FinishInput) core.TicketView {
	t.Helper()
	in.Ticket, in.ClaimID, in.Summary, in.Full = "2", "claim-1", "Done.", true
	res, err := f.core.Finish(ctx, "acme", in)
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	return *res.Ticket
}

func TestAFinishRecordsEachRepositoryItNamesInLandingOrder(t *testing.T) {
	f := newFixture(t)
	f.severalRepositories(t)

	tk := f.finishWith(t, core.FinishInput{Branch: "acme-2", Commit: "aaa", Repositories: []core.RepositoryWork{
		{Name: "api", Branch: "acme-2", Commit: "bbb"}, {Name: "web", Branch: "acme-2", Commit: "ccc"},
	}})

	if got := changedNames(tk); !slices.Equal(got, []string{"web", "acme", "api"}) {
		t.Fatalf("Changed Repositories %v, want web, acme, api: the landing order", got)
	}
	if tk.Branch != "acme-2" || tk.Commit != "aaa" || tk.Repositories[1].Commit != "aaa" || tk.Repositories[2].Commit != "bbb" {
		t.Fatalf("ticket %+v, want the Home Repository's work in branch and commit and in its entry", tk)
	}
}

func TestTheHomeRepositoryMayBeGivenInRepositoriesInstead(t *testing.T) {
	f := newFixture(t)
	f.severalRepositories(t)

	tk := f.finishWith(t, core.FinishInput{Repositories: []core.RepositoryWork{{Name: "acme", Branch: "acme-2", Commit: "aaa"}}})

	if tk.Branch != "acme-2" || tk.Commit != "aaa" || !slices.Equal(changedNames(tk), []string{"acme"}) {
		t.Fatalf("ticket branch %q commit %q repositories %v, want the Home Repository's work mirrored", tk.Branch, tk.Commit, changedNames(tk))
	}
}

func TestAFinishRefusesABadRepositoriesListAndKeepsTheClaim(t *testing.T) {
	for name, in := range map[string]core.FinishInput{
		"an unknown name": {Repositories: []core.RepositoryWork{{Name: "docs", Branch: "b", Commit: "c"}}},
		"a name twice":    {Repositories: []core.RepositoryWork{{Name: "api", Branch: "b", Commit: "c"}, {Name: "api", Branch: "b", Commit: "d"}}},
		"no commit":       {Repositories: []core.RepositoryWork{{Name: "api", Branch: "b"}}},
		"no branch":       {Repositories: []core.RepositoryWork{{Name: "api", Commit: "c"}}},
		"the Home twice":  {Branch: "acme-2", Repositories: []core.RepositoryWork{{Name: "acme", Branch: "acme-2", Commit: "c"}}},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.severalRepositories(t)
			in.Ticket, in.ClaimID, in.Summary = "2", "claim-1", "Done."
			_, err := f.core.Finish(ctx, "acme", in)
			var e *core.Error
			if !errors.As(err, &e) || e.Code != core.Invalid || !strings.HasPrefix(e.Field, "repositories") {
				t.Fatalf("finish: %v, want invalid repositories", err)
			}
			if tk := f.get(t, "2"); tk.State != store.InProgress {
				t.Fatalf("acme-2 is %s, want it still In Progress under its Claim", tk.State)
			}
		})
	}
}

func TestAFinishReplacesTheChangedRepositories(t *testing.T) {
	f := newFixture(t)
	f.severalRepositories(t)
	f.reportChanged(t, core.RepositoryWork{Name: "api", Branch: "acme-2", Commit: "bbb"}, core.RepositoryWork{Name: "web", Branch: "acme-2", Commit: "ccc"})

	tk := f.finishWith(t, core.FinishInput{Repositories: []core.RepositoryWork{{Name: "api", Branch: "acme-2", Commit: "ddd"}}})

	if got := changedNames(tk); !slices.Equal(got, []string{"api"}) || tk.Repositories[0].Commit != "ddd" {
		t.Fatalf("Changed Repositories %+v, want only api at ddd: the finish settles them", tk.Repositories)
	}
}

func TestAcceptingWorkOnlyInAnotherRepositoryWaitsToLand(t *testing.T) {
	f := newFixture(t)
	f.severalRepositories(t)
	f.finishWith(t, core.FinishInput{Repositories: []core.RepositoryWork{{Name: "api", Branch: "acme-2", Commit: "bbb"}}})

	v, err := f.core.Accept(human, "acme", core.TicketInput{Ticket: "2", Full: true})
	if err != nil {
		t.Fatal(err)
	}
	if v.Record.State != store.Accepted {
		t.Fatalf("acme-2 is %s, want Accepted: it changed api", v.Record.State)
	}
}

// A finish that drops a Repository the live report named would go
// straight to Done under Auto-accept, out of the finish check's reach, so
// it waits In Review for the check instead.
func TestAFinishDroppingAReportedRepositoryIsntAutoAccepted(t *testing.T) {
	f := newFixture(t)
	f.severalRepositories(t)
	f.setAutoAccept(t, map[store.AutoAcceptKind]bool{store.AutoAcceptImplementation: true})
	f.reportChanged(t, core.RepositoryWork{Name: "api", Branch: "acme-2", Commit: "bbb"})

	if tk := f.finishWith(t, core.FinishInput{}); tk.State != store.InReview {
		t.Fatalf("acme-2 is %s, want In Review for the finish check", tk.State)
	}
	if tk := f.get(t, "2"); tk.State != store.InReview {
		t.Fatalf("acme-2 is %s after the finish, want In Review: not auto-accepted", tk.State)
	}
}

func TestAFinishKeepingItsRepositoriesIsAutoAccepted(t *testing.T) {
	f := newFixture(t)
	f.severalRepositories(t)
	f.setAutoAccept(t, map[store.AutoAcceptKind]bool{store.AutoAcceptImplementation: true})
	f.reportChanged(t, core.RepositoryWork{Name: "api", Branch: "acme-2", Commit: "bbb"})

	f.finishWith(t, core.FinishInput{Repositories: []core.RepositoryWork{{Name: "api", Branch: "acme-2", Commit: "ccc"}}})

	if tk := f.get(t, "2"); tk.State != store.Accepted {
		t.Fatalf("acme-2 is %s, want Accepted on the human's behalf", tk.State)
	}
}

func TestATicketFinishedBeforeRepositoriesExistedReadsAsTheHomeRepositorys(t *testing.T) {
	f := newFixture(t)
	f.inReviewOnBranch(t)
	if tk := f.get(t, "2"); tk.Repositories != nil {
		t.Fatalf("with the Home Repository alone, repositories %+v, want none shown", tk.Repositories)
	}

	f.addRepository(t, core.AddRepositoryInput{Name: "api", CloneURL: "git@github.com:acme/api.git"})

	tk := f.get(t, "2")
	if len(tk.Repositories) != 1 || tk.Repositories[0] != (core.ChangedRepositoryView{Name: "acme", Branch: "acme-2", Commit: "abc123"}) {
		t.Fatalf("repositories %+v, want the Home Repository's branch and commit", tk.Repositories)
	}
	p, _ := f.core.GetProject(ctx, "acme")
	if by := repository(t, p, "acme").ChangedBy; !slices.Equal(by, []string{"acme-2"}) {
		t.Fatalf("acme changed by %v, want acme-2", by)
	}
}

func TestASingleRepositoryFinishStoresNoRepositoriesList(t *testing.T) {
	f := newFixture(t)
	f.inReviewOnBranch(t)
	f.store.View(func(r store.Reader) error {
		if tk, _ := r.Ticket("acme", 2); tk.Repositories != nil {
			t.Fatalf("stored repositories %+v, want none: the branch and commit say it all", tk.Repositories)
		}
		return nil
	})
}

func TestADoneTicketsHomeRepositoryShowsTheCommitItLandedAt(t *testing.T) {
	f := newFixture(t)
	f.severalRepositories(t)
	f.finishWith(t, core.FinishInput{Branch: "acme-2", Commit: "aaa"})
	if _, err := f.core.Land(ctx, "acme", core.LandInput{Ticket: "2", Commit: "fff"}); err != nil {
		t.Fatal(err)
	}

	tk := f.get(t, "2")
	if l := tk.Repositories[0].Landed; l == nil || l.Commit != "fff" {
		t.Fatalf("acme landed %+v, want fff", l)
	}
}

// reportChanged reports acme-2's live Changed Repositories under claim-1.
func (f fixture) reportChanged(t *testing.T, rs ...core.RepositoryWork) core.TicketView {
	t.Helper()
	v, err := f.core.ReportChangedRepositories(ctx, "acme", core.ReportChangedRepositoriesInput{Ticket: "2", ClaimID: "claim-1", Repositories: rs, Full: true})
	if err != nil {
		t.Fatalf("report_changed_repositories: %v", err)
	}
	return *v.Record
}

func TestTheFactoryReportsTheChangedRepositoriesLive(t *testing.T) {
	f := newFixture(t)
	f.severalRepositories(t)

	tk := f.reportChanged(t, core.RepositoryWork{Name: "api", Branch: "acme-2", Commit: "bbb"})
	if !slices.Equal(changedNames(tk), []string{"api"}) {
		t.Fatalf("live Changed Repositories %v, want api", changedNames(tk))
	}
	rows, _ := f.core.FindTickets(ctx, "acme", core.FindTicketsInput{State: []store.State{store.InProgress}})
	if len(rows) != 1 || !slices.Equal(rows[0].Repositories, []string{"api"}) {
		t.Fatalf("rows %+v, want acme-2's card naming api", rows)
	}

	// A report the same as the record writes nothing.
	f.clock.Advance(time.Minute)
	if again := f.reportChanged(t, core.RepositoryWork{Name: "api", Branch: "acme-2", Commit: "bbb"}); !again.Updated.Equal(tk.Updated) {
		t.Fatalf("updated %v, want %v: nothing changed", again.Updated, tk.Updated)
	}
	tk = f.reportChanged(t, core.RepositoryWork{Name: "api", Branch: "acme-2", Commit: "bbb"}, core.RepositoryWork{Name: "acme", Branch: "acme-2", Commit: "aaa"})
	if !slices.Equal(changedNames(tk), []string{"acme", "api"}) || tk.Branch != "" {
		t.Fatalf("ticket %+v, want acme and api live, with no branch recorded until the finish", tk)
	}
}

func TestOnlyTheLiveClaimReportsTheChangedRepositoriesWhileInProgress(t *testing.T) {
	f := newFixture(t)
	f.severalRepositories(t)
	work := core.ReportChangedRepositoriesInput{Ticket: "2", ClaimID: "claim-9", Repositories: []core.RepositoryWork{{Name: "api", Branch: "acme-2", Commit: "bbb"}}}
	_, err := f.core.ReportChangedRepositories(ctx, "acme", work)
	wantCode(t, err, core.StaleClaim, "")

	if _, err := f.core.PostRound(ctx, "acme", core.PostRoundInput{Ticket: "2", ClaimID: "claim-1", Questions: []core.QuestionInput{{Title: "Q", Body: "Q?"}}}); err != nil {
		t.Fatal(err)
	}
	work.ClaimID = "claim-1"
	_, err = f.core.ReportChangedRepositories(ctx, "acme", work)
	wantCode(t, err, core.ActionNotAllowed, "")
}

// failFinish fails acme-2's finish as the Factory.
func (f fixture) failFinish() (core.TicketView, error) {
	return record(f.core.FailFinish(ctx, "acme", core.FailFinishInput{Ticket: "2", WhatBroke: "api is left out", Why: "api's branch has commits", WhatToFix: "finish with api", WhereWorkStands: "api at bbb", Full: true}))
}

func TestFailFinishMovesAFinishedTicketToFailedWithAFailureReportOnTheFinishingClaim(t *testing.T) {
	f := newFixture(t)
	f.severalRepositories(t)
	f.finishWith(t, core.FinishInput{Branch: "acme-2", Commit: "aaa"})

	tk, err := f.failFinish()
	if err != nil {
		t.Fatal(err)
	}
	if tk.State != store.Failed || len(tk.FailureReports) != 1 {
		t.Fatalf("acme-2 is %s with %d Failure Reports, want Failed with one", tk.State, len(tk.FailureReports))
	}
	if r := tk.FailureReports[0]; r.Claim != 1 || r.WhatBroke != "api is left out" {
		t.Fatalf("Failure Report %+v, want it on the finishing Claim 1", r)
	}
	if _, err := f.core.SendBack(human, "acme", core.SendBackInput{Ticket: "2"}); err != nil {
		t.Fatalf("send_back: %v: the human sends a failed finish back as any other", err)
	}
}

func TestFailFinishMovesAnAcceptedTicketThatHasLandedNowhere(t *testing.T) {
	f := newFixture(t)
	f.severalRepositories(t)
	f.finishWith(t, core.FinishInput{Branch: "acme-2", Commit: "aaa"})
	if _, err := f.core.Accept(human, "acme", core.TicketInput{Ticket: "2"}); err != nil {
		t.Fatal(err)
	}

	if tk, err := f.failFinish(); err != nil || tk.State != store.Failed {
		t.Fatalf("fail_finish: %v, %+v, want Failed", err, tk.State)
	}
}

func TestFailFinishRefusesAnAcceptedTicketThatHasLandedSomewhere(t *testing.T) {
	f := newFixture(t)
	f.severalRepositories(t)
	f.finishWith(t, core.FinishInput{Branch: "acme-2", Commit: "aaa", Repositories: []core.RepositoryWork{{Name: "api", Branch: "acme-2", Commit: "bbb"}}})
	if _, err := f.core.Accept(human, "acme", core.TicketInput{Ticket: "2"}); err != nil {
		t.Fatal(err)
	}
	// Landed in api alone, as the Factory records a landing per
	// Repository.
	if err := f.store.Update(func(w store.Writer) error {
		tk, _ := w.Ticket("acme", 2)
		tk.Repositories[1].Landed = &store.Landed{Commit: "eee", Time: t0}
		return w.PutTicket("acme", tk)
	}); err != nil {
		t.Fatal(err)
	}
	if tk := f.get(t, "2"); !tk.PartlyLanded {
		t.Fatalf("acme-2 %+v, want it Partly landed", tk.Repositories)
	}

	_, err := f.failFinish()
	wantCode(t, err, core.ActionNotAllowed, "")
}

func TestFailFinishIsTheFactorysAloneAndOfferedToNoActor(t *testing.T) {
	f := newFixture(t)
	f.severalRepositories(t)
	_, err := f.failFinish()
	wantCode(t, err, core.ActionNotAllowed, "")

	f.finishWith(t, core.FinishInput{Branch: "acme-2", Commit: "aaa"})
	_, err = f.core.Reject(ctx, "acme", core.RejectInput{Ticket: "2", Comment: "no"})
	var e *core.Error
	if !errors.As(err, &e) || slices.Contains(e.AllowedActions, "fail_finish") {
		t.Fatalf("allowed actions %v, want no fail_finish among them", e.AllowedActions)
	}
}

func TestASpecShowsTheUnionOfItsTicketsChangedRepositoriesInLandingOrder(t *testing.T) {
	f := newFixture(t)
	f.severalRepositories(t)
	f.finishWith(t, core.FinishInput{Repositories: []core.RepositoryWork{{Name: "api", Branch: "acme-2", Commit: "bbb"}}})
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Other", State: store.Ready}) // acme-3
	if _, err := f.core.Claim(ctx, "acme", core.ClaimInput{Ticket: "3"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.Finish(ctx, "acme", core.FinishInput{Ticket: "3", ClaimID: "claim-2", Summary: "Done.", Repositories: []core.RepositoryWork{
		{Name: "web", Branch: "acme-3", Commit: "ccc"}, {Name: "api", Branch: "acme-3", Commit: "ddd"},
	}}); err != nil {
		t.Fatal(err)
	}

	sp, err := f.core.GetSpec(ctx, "acme", core.GetSpecInput{Spec: "1", Parts: []string{"outline"}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(sp.Repositories, []string{"web", "api"}) {
		t.Fatalf("the Spec's Changed Repositories %v, want web, api", sp.Repositories)
	}
}

func TestARepositoryWithLiveWorkCantBeRemoved(t *testing.T) {
	f := newFixture(t)
	f.severalRepositories(t)
	f.reportChanged(t, core.RepositoryWork{Name: "api", Branch: "acme-2", Commit: "bbb"})

	_, err := f.core.RemoveRepository(human, "acme", core.RemoveRepositoryInput{Name: "api"})
	var e *core.Error
	if !errors.As(err, &e) || !strings.Contains(e.Message, "acme-2") {
		t.Fatalf("remove_repository: %v, want it refused naming acme-2", err)
	}
}
