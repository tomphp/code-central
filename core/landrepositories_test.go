package core_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Landing across Repositories (Spec code-central-848, ADR 0049): land
// records a landed commit per Repository, and a Ticket is Done once every
// Changed Repository has landed and Partly landed until then; the base is
// reported per Repository; and Accepted but not landed names the
// Repositories that have landed and those that haven't, with why.

// acceptedInTwo is severalRepositories with acme-2 finished in the Home
// Repository (at aaa) and api (at bbb), then accepted.
func (f fixture) acceptedInTwo(t *testing.T) {
	t.Helper()
	f.severalRepositories(t)
	f.finishWith(t, core.FinishInput{Branch: "acme-2", Commit: "aaa", Repositories: []core.RepositoryWork{{Name: "api", Branch: "acme-2", Commit: "bbb"}}})
	if _, err := f.core.Accept(human, "acme", core.TicketInput{Ticket: "2"}); err != nil {
		t.Fatal(err)
	}
}

// landIn lands acme-2 as in says, as the Factory does.
func (f fixture) landIn(t *testing.T, in core.LandInput) core.TicketView {
	t.Helper()
	in.Ticket, in.Full = "2", true
	res, err := f.core.Land(ctx, "acme", in)
	if err != nil {
		t.Fatalf("land %+v: %v", in, err)
	}
	return *res.Record
}

// landedAt is the commit tk landed at in the Repository name, or "".
func landedAt(tk core.TicketView, name string) string {
	for _, r := range tk.Repositories {
		if r.Name == name && r.Landed != nil {
			return r.Landed.Commit
		}
	}
	return ""
}

func TestLandingInOneRepositoryLeavesTheTicketPartlyLandedUntilEveryOneHas(t *testing.T) {
	f := newFixture(t)
	f.acceptedInTwo(t)

	tk := f.landIn(t, core.LandInput{Repository: "api", Commit: "eee"})
	if tk.State != store.Accepted || !tk.PartlyLanded || landedAt(tk, "api") != "eee" || landedAt(tk, "acme") != "" {
		t.Fatalf("acme-2 %s partly landed %v %+v, want it Accepted and Partly landed, landed in api alone", tk.State, tk.PartlyLanded, tk.Repositories)
	}

	// Without a Repository, land means the Home Repository.
	tk = f.landIn(t, core.LandInput{Commit: "fff"})
	if tk.State != store.Done || tk.PartlyLanded || landedAt(tk, "acme") != "fff" || landedAt(tk, "api") != "eee" {
		t.Fatalf("acme-2 %s partly landed %v %+v, want it Done, landed in both", tk.State, tk.PartlyLanded, tk.Repositories)
	}
	if m := tk.Moves[len(tk.Moves)-1]; m.Via != "merge" || m.Commit != "fff" {
		t.Fatalf("last move %+v, want the land at fff", m)
	}
}

func TestLandRefusesARepositoryTheTicketDidntChangeOrTheProjectHasnt(t *testing.T) {
	for name, repository := range map[string]string{"unchanged": "web", "unknown": "docs"} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.acceptedInTwo(t)
			_, err := f.core.Land(ctx, "acme", core.LandInput{Ticket: "2", Repository: repository, Commit: "eee"})
			wantCode(t, err, core.Invalid, "repository")
			if tk := f.get(t, "2"); tk.State != store.Accepted || landedAt(tk, repository) != "" {
				t.Fatalf("acme-2 %s %+v, want it as it was", tk.State, tk.Repositories)
			}
		})
	}
}

func TestWorkInAnotherRepositoryAloneLandsByItsName(t *testing.T) {
	f := newFixture(t)
	f.severalRepositories(t)
	f.finishWith(t, core.FinishInput{Repositories: []core.RepositoryWork{{Name: "api", Branch: "acme-2", Commit: "bbb"}}})
	if _, err := f.core.Accept(human, "acme", core.TicketInput{Ticket: "2"}); err != nil {
		t.Fatal(err)
	}

	_, err := f.core.Land(ctx, "acme", core.LandInput{Ticket: "2", Commit: "eee"})
	wantCode(t, err, core.Invalid, "repository")
	if tk := f.landIn(t, core.LandInput{Repository: "api", Commit: "eee"}); tk.State != store.Done {
		t.Fatalf("acme-2 %s, want Done once api, its one Changed Repository, has landed", tk.State)
	}
}

func TestAPartlyLandedTicketCantBeCancelled(t *testing.T) {
	f := newFixture(t)
	f.acceptedInTwo(t)
	f.landIn(t, core.LandInput{Repository: "api", Commit: "eee"})

	_, err := f.core.Cancel(human, "acme", core.CancelInput{Ticket: "2"})
	wantCode(t, err, core.ActionNotAllowed, "")
	if !strings.Contains(err.Error(), "Partly landed") {
		t.Fatalf("refusal %q, want it to say the Ticket is Partly landed", err)
	}
	if tk := f.get(t, "2"); tk.State != store.Accepted {
		t.Fatalf("acme-2 %s, want it still Accepted", tk.State)
	}
}

// refinish sends Accepted acme-2 back as the Factory does on a conflict,
// claims it again, finishes it with in and accepts it.
func (f fixture) refinish(t *testing.T, in core.FinishInput) core.TicketView {
	t.Helper()
	if _, err := f.core.SendBack(ctx, "acme", core.SendBackInput{Ticket: "2", Note: "web conflicts"}); err != nil {
		t.Fatalf("send_back: %v", err)
	}
	res := f.claim(t, core.ClaimInput{Ticket: "2"})
	in.Ticket, in.ClaimID, in.Summary = "2", res.ClaimID, "Done again."
	if _, err := f.core.Finish(ctx, "acme", in); err != nil {
		t.Fatalf("finish: %v", err)
	}
	if _, err := f.core.Accept(human, "acme", core.TicketInput{Ticket: "2"}); err != nil {
		t.Fatal(err)
	}
	return f.get(t, "2")
}

func TestAFinishAfterAPartialLandingKeepsTheLandingsItsWorkStillHolds(t *testing.T) {
	f := newFixture(t)
	f.acceptedInTwo(t)
	f.landIn(t, core.LandInput{Repository: "api", Commit: "eee"})

	// api's work is unchanged, so it stays landed; the Home Repository's
	// changed.
	tk := f.refinish(t, core.FinishInput{Branch: "acme-2", Commit: "aa2", Repositories: []core.RepositoryWork{{Name: "api", Branch: "acme-2", Commit: "bbb"}}})
	if landedAt(tk, "api") != "eee" {
		t.Fatalf("Changed Repositories %+v, want api still landed at eee", tk.Repositories)
	}

	// api's work is in its base, so a finish can leave it out: it stays
	// a Changed Repository, landed.
	tk = f.refinish(t, core.FinishInput{Branch: "acme-2", Commit: "aa3"})
	if !slices.Equal(changedNames(tk), []string{"acme", "api"}) || landedAt(tk, "api") != "eee" {
		t.Fatalf("Changed Repositories %+v, want acme and api, api still landed", tk.Repositories)
	}

	// api's work changed since it landed: it needs landing again.
	tk = f.refinish(t, core.FinishInput{Branch: "acme-2", Commit: "aa3", Repositories: []core.RepositoryWork{{Name: "api", Branch: "acme-2", Commit: "bb2"}}})
	if landedAt(tk, "api") != "" {
		t.Fatalf("Changed Repositories %+v, want api not landed: its work changed", tk.Repositories)
	}
}

// basesProject is acme under Merge + Push with the Repositories api, whose
// Base is develop, and web, and a Factory online.
func (f fixture) basesProject(t *testing.T) {
	t.Helper()
	f.project(t, "acme")
	f.addRepository(t, core.AddRepositoryInput{Name: "api", CloneURL: "git@github.com:acme/api.git", Base: "develop"})
	f.addRepository(t, core.AddRepositoryInput{Name: "web", CloneURL: "git@github.com:acme/web.git"})
	if _, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: store.LandingMerge, Push: true, Remote: "origin"}); err != nil {
		t.Fatal(err)
	}
	beat(f, t, "cc", "box", core.HeartbeatOnline, 10)
}

// baseRows are the Needs Me rows of reason.
func (f fixture) baseRows(t *testing.T, reason core.NeedsMeReason) []core.NeedsMeRow {
	t.Helper()
	var out []core.NeedsMeRow
	for _, row := range f.needsMe(t) {
		if row.Reason == reason {
			out = append(out, row)
		}
	}
	return out
}

func TestTheBaseIsReportedPerRepositoryAndEachBrokenOneIsAnItemNamingIt(t *testing.T) {
	f := newFixture(t)
	f.basesProject(t)
	f.reportBase(t, core.ReportBaseInput{State: core.BaseCurrent, Local: "a", Remote: "a"})
	f.reportBase(t, core.ReportBaseInput{Repository: "api", State: core.BaseDiverged, Local: "aaa111", Remote: "bbb222"})
	f.reportBase(t, core.ReportBaseInput{Repository: "web", State: core.BaseMissing})

	diverged := f.baseRows(t, core.ReasonBaseDiverged)
	if len(diverged) != 1 || diverged[0].BaseDiverged.Repository != "api" || diverged[0].BaseDiverged.Base != "develop" || diverged[0].BaseDiverged.Local != "aaa111" {
		t.Fatalf("Base diverged rows %+v, want one for api's develop", diverged)
	}
	missing := f.baseRows(t, core.ReasonBaseMissing)
	if len(missing) != 1 || missing[0].BaseMissing.Repository != "web" || missing[0].BaseMissing.Base != "main" || !strings.Contains(missing[0].BaseMissing.Fix, "web") {
		t.Fatalf("Base missing rows %+v, want one for web's main, its fix naming web", missing)
	}

	// The Home Repository's base breaking too is an item of its own.
	f.reportBase(t, core.ReportBaseInput{State: core.BaseDiverged, Local: "c", Remote: "d"})
	if diverged = f.baseRows(t, core.ReasonBaseDiverged); len(diverged) != 2 || diverged[0].BaseDiverged.Repository != "acme" || diverged[1].BaseDiverged.Repository != "api" {
		t.Fatalf("Base diverged rows %+v, want acme's then api's, in landing order", diverged)
	}

	f.reportBase(t, core.ReportBaseInput{Repository: "api", State: core.BaseCurrent, Local: "bbb222", Remote: "bbb222"})
	if diverged = f.baseRows(t, core.ReasonBaseDiverged); len(diverged) != 1 || diverged[0].BaseDiverged.Repository != "acme" {
		t.Fatalf("Base diverged rows %+v, want acme's alone once api's is current", diverged)
	}
}

func TestReportBaseRefusesARepositoryTheProjectHasnt(t *testing.T) {
	f := newFixture(t)
	f.basesProject(t)
	_, err := f.core.ReportBase(ctx, "acme", core.ReportBaseInput{Repository: "docs", State: core.BaseMissing})
	wantCode(t, err, core.Invalid, "repository")
}

func TestAReportForARemovedRepositoryNoLongerShows(t *testing.T) {
	f := newFixture(t)
	f.basesProject(t)
	f.reportBase(t, core.ReportBaseInput{Repository: "web", State: core.BaseMissing})
	if _, err := f.core.RemoveRepository(human, "acme", core.RemoveRepositoryInput{Name: "web"}); err != nil {
		t.Fatal(err)
	}
	if rows := f.baseRows(t, core.ReasonBaseMissing); len(rows) != 0 {
		t.Fatalf("Base missing rows %+v, want none for a removed Repository", rows)
	}
}

func TestAHomeRepositoryAloneNamesNoRepositoryOnItsBaseItems(t *testing.T) {
	f := newFixture(t)
	f.pullRequestProject(t)
	f.reportBase(t, core.ReportBaseInput{State: core.BaseDiverged, Local: "a", Remote: "b"})
	if row, ok := baseRow(t, f); !ok || row.BaseDiverged.Repository != "" {
		t.Fatalf("row %+v, want Base diverged naming no Repository", row)
	}
}

func TestAcceptedButNotLandedNamesTheRepositoriesLandedAndNotLandedWithWhy(t *testing.T) {
	f := newFixture(t)
	f.acceptedInTwo(t)
	f.landIn(t, core.LandInput{Repository: "api", Commit: "eee"})
	why := "couldn't push main to origin:\n\n```\nrejected\n```"
	if _, err := f.core.Comment(ctx, "acme", core.CommentInput{Ticket: "2", Body: core.CantLandComment("acme", why, store.Accepted)}); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(core.NotLandedAfter + time.Second)

	row, ok := longRow(t, f, "acme-2")
	if !ok || row.Reason != core.ReasonAcceptedNotLanded || row.NotLanded == nil {
		t.Fatalf("row %+v, want Accepted but not landed naming the Repositories", row)
	}
	if !slices.Equal(row.NotLanded.Landed, []string{"api"}) || len(row.NotLanded.NotLanded) != 1 ||
		row.NotLanded.NotLanded[0] != (core.NotLandedRepositoryView{Name: "acme", Reason: "couldn't push main to origin"}) {
		t.Fatalf("not landed %+v, want api landed and acme not, as the push was refused", row.NotLanded)
	}
	tk := f.get(t, "2")
	if r := tk.Repositories[0]; r.Name != "acme" || r.NotLanded != "couldn't push main to origin" {
		t.Fatalf("acme's row %+v, want why it hasn't landed", r)
	}
}

func TestABrokenBaseIsWhyARepositoryHasntLanded(t *testing.T) {
	f := newFixture(t)
	f.acceptedInTwo(t)
	beat(f, t, "cc", "box", core.HeartbeatOnline, 10)
	f.reportBase(t, core.ReportBaseInput{Repository: "api", State: core.BaseMissing})

	tk := f.get(t, "2")
	if r := tk.Repositories[1]; r.Name != "api" || r.NotLanded != "its base, main, is missing" {
		t.Fatalf("api's row %+v, want its missing base as why it hasn't landed", r)
	}
}

func TestAReasonFromBeforeTheTicketWasLastAcceptedIsntShown(t *testing.T) {
	f := newFixture(t)
	f.acceptedInTwo(t)
	if _, err := f.core.Comment(ctx, "acme", core.CommentInput{Ticket: "2", Body: core.CantLandComment("api", "an old reason", store.Accepted)}); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Minute)
	tk := f.refinish(t, core.FinishInput{Branch: "acme-2", Commit: "aa2", Repositories: []core.RepositoryWork{{Name: "api", Branch: "acme-2", Commit: "bb2"}}})

	if r := tk.Repositories[1]; r.Name != "api" || r.NotLanded != "" {
		t.Fatalf("api's row %+v, want no reason from before the last accept", r)
	}
}

func TestAHomeRepositoryAloneLandsAsItAlwaysHas(t *testing.T) {
	f := newFixture(t)
	f.accepted(t)
	f.clock.Advance(core.NotLandedAfter + time.Second)
	if row, ok := longRow(t, f, "acme-2"); !ok || row.NotLanded != nil {
		t.Fatalf("row %+v, want Accepted but not landed naming no Repository", row)
	}
	tk := f.landIn(t, core.LandInput{Commit: "fff"})
	if tk.State != store.Done || tk.Repositories != nil || tk.PartlyLanded {
		t.Fatalf("acme-2 %s %+v, want it Done with no Repositories shown", tk.State, tk.Repositories)
	}
}

func TestUnderTheCIGateARepositoryWaitingOnItsCISaysSoAsWhyItHasntLanded(t *testing.T) {
	f := twoRepoWorld(t)
	f.changes(t, 5, "acme", "api")
	f.pushedFor(t, "api", "acme-5", "def456")
	f.ciRunIn(t, "api", 1, "def456", "acme-5", store.CIInProgress, "")

	v := f.getTicket(t, "acme-5")
	if home, api := changedIn(t, v, "acme"), changedIn(t, v, "api"); home.NotLanded != "waiting on CI at abc123" || api.NotLanded != "waiting on CI at def456" {
		t.Fatalf("Home %+v, api %+v: want each waiting on its own CI", home, api)
	}

	f.ciRunIn(t, "api", 1, "def456", "acme-5", store.CICompleted, store.CIFailure)
	if api := changedIn(t, f.getTicket(t, "acme-5"), "api"); api.NotLanded != "CI failed at def456" {
		t.Fatalf("api %+v, want its red build as why it hasn't landed", api)
	}
}

func TestAnOpenPullRequestIsWhyARepositoryHasntLanded(t *testing.T) {
	f := newFixture(t)
	f.pullRequestsAcross(t)
	f.reportIn(t, "api", 4, store.PROpen, "")

	tk := f.get(t, "2")
	if home, api := changedIn(t, tk, "acme"), changedIn(t, tk, "api"); api.NotLanded != "its pull request 4 is open" || home.NotLanded != "" {
		t.Fatalf("Home %+v, api %+v: want api's open pull request as why it hasn't landed, and no reason for the Home Repository, which has no pull request yet", home, api)
	}
}
