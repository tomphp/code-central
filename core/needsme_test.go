package core_test

import (
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
)

func wantReason(t *testing.T, v api.TicketView, want api.NeedsMeReason) {
	t.Helper()
	if v.NeedsMeReason != want {
		t.Fatalf("%s (%s): needs_me_reason %q, want %q", v.ID, v.State, v.NeedsMeReason, want)
	}
}

func TestATicketWaitingOnARoundNeedsMe(t *testing.T) {
	f := newFixture(t)
	f.asking(t, qDB, qName)
	wantReason(t, f.get(t, "2"), api.ReasonNeedsAnswer)
	f.core.Answer(human, "acme", api.AnswerInput{Ticket: "2", Question: 1, Text: "Postgres"})
	f.core.DismissQuestion(human, "acme", api.DismissQuestionInput{Ticket: "2", Question: 2})
	wantReason(t, f.get(t, "2"), api.ReasonAnswered)
}

func TestASilentClaimNeedsMeOnlyOnceItIsSilent(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	f.claim(t, api.ClaimInput{})
	wantReason(t, f.get(t, "2"), "")
	f.clock.Advance(core.SilentAfter)
	wantReason(t, f.get(t, "2"), api.ReasonSilentClaim)
}

func TestFailedAndInReviewNeedMe(t *testing.T) {
	f := newFixture(t)
	f.failed(t)
	wantReason(t, f.get(t, "2"), api.ReasonFailed)

	g := newFixture(t)
	g.inReview(t)
	wantReason(t, g.get(t, "2"), api.ReasonInReview)
}

func TestWorkThatIsNotWaitingOnTheHumanDoesNotNeedMe(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Draft, domain.Ready, domain.Ready, domain.Ready)
	f.setState(t, 4, domain.Done)
	f.cancel(t, api.CancelInput{Ticket: "5"}, api.Human)
	for _, ref := range []api.Ref{"2", "3", "4", "5"} {
		wantReason(t, f.get(t, ref), "")
	}
}

func TestBlockedByACancelledTicketWithNoReplacementsNeedsMe(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready, domain.Draft, domain.Ready, domain.Ready) // acme-2..acme-5
	f.addBlocker(t, "2", "3")
	f.addBlocker(t, "4", "5")
	wantReason(t, f.get(t, "2"), "") // an open, live Blocker is just waiting
	f.cancel(t, api.CancelInput{Ticket: "3"}, api.Human)
	wantReason(t, f.get(t, "2"), api.ReasonBlockedByCancelled)
	// A Cancelled Blocker with a Replacement is followed to it instead.
	f.cancel(t, api.CancelInput{Ticket: "5", Replacements: []api.Ref{"3"}}, api.Human)
	wantReason(t, f.get(t, "4"), api.ReasonBlockedByCancelled) // its chain ends on Cancelled acme-3
	f.core.RemoveBlocker(ctx, "acme", api.BlockerInput{Ticket: "4", Blocker: "5"})
	wantReason(t, f.get(t, "4"), "")
}

// A Ticket that is both In Review and blocked by a Cancelled Ticket shows
// under the cheaper reason: the earlier group.
func TestTheEarliestGroupWinsWhenTwoReasonsApply(t *testing.T) {
	f := newFixture(t)
	f.inReview(t)
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "1", Title: "Gone", State: domain.Ready})
	f.addBlocker(t, "2", "3")
	f.cancel(t, api.CancelInput{Ticket: "3"}, api.Human)
	wantReason(t, f.get(t, "2"), api.ReasonInReview)
}

// needsMeWorld is two Projects with something in every Needs Me group,
// reached through the workflow at known times, read at t0+2h.
func needsMeWorld(t *testing.T) fixture {
	t.Helper()
	f := newFixture(t)
	must := func(_ any, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, key := range []string{"acme", "web"} {
		f.project(t, key)
		f.spec(t, key, "Login")
	}
	for range 6 { // acme-2..acme-7
		f.ticket(t, "acme", api.CreateTicketInput{Spec: "1", Title: "A", State: domain.Ready})
	}
	for range 2 { // web-2, web-3
		f.ticket(t, "web", api.CreateTicketInput{Spec: "1", Title: "W", State: domain.Ready})
	}
	f.addBlocker(t, "6", "7")
	claims := map[string]string{}
	for _, id := range []string{"acme-2", "acme-3", "acme-4", "acme-5", "web-2", "web-3"} {
		key, _, _ := strings.Cut(id, "-")
		res, err := f.core.Claim(ctx, key, api.ClaimInput{Ticket: api.Ref(id)})
		must(res, err)
		claims[id] = res.ClaimID
	}
	at := func(m int) { f.clock.Set(t0.Add(time.Duration(m) * time.Minute)) }
	at(10)
	must(f.core.PostRound(ctx, "web", api.PostRoundInput{Ticket: "web-2", ClaimID: claims["web-2"], Questions: []api.QuestionInput{qDB, qName}}))
	at(20)
	must(f.core.PostRound(ctx, "acme", api.PostRoundInput{Ticket: "acme-2", ClaimID: claims["acme-2"], Questions: []api.QuestionInput{qDB}}))
	at(25)
	must(f.core.PostRound(ctx, "acme", api.PostRoundInput{Ticket: "acme-5", ClaimID: claims["acme-5"], Questions: []api.QuestionInput{qName}}))
	at(30)
	must(f.core.Finish(ctx, "acme", api.FinishInput{Ticket: "3", ClaimID: claims["acme-3"], Summary: "Built the form.\nOn branch acme-3."}))
	at(40)
	must(f.core.ReportFailure(ctx, "acme", api.ReportFailureInput{Ticket: "4", ClaimID: claims["acme-4"],
		WhatBroke: "Login 500s\nThe token table is missing", Why: "W", WhatToFix: "F", WhereWorkStands: "S"}))
	at(50)
	must(f.core.Answer(human, "acme", api.AnswerInput{Ticket: "2", Question: 1, Text: "SQLite"}))
	must(f.core.Answer(human, "web", api.AnswerInput{Ticket: "2", Question: 1, Text: "SQLite"}))
	at(60)
	f.cancel(t, api.CancelInput{Ticket: "7"}, api.Human)
	at(120)
	return f
}

func TestNeedsMeGroupsEveryProjectByReasonCheapestFirstAndLongestWaitingFirst(t *testing.T) {
	f := needsMeWorld(t)
	v, err := f.core.NeedsMe(human)
	if err != nil {
		t.Fatal(err)
	}
	mins := func(m int) time.Time { return t0.Add(time.Duration(m) * time.Minute) }
	want := []struct {
		reason api.NeedsMeReason
		ids    []string
		since  []time.Time
	}{
		{api.ReasonNeedsAnswer, []string{"web-2", "acme-5"}, []time.Time{mins(10), mins(25)}},
		{api.ReasonAnswered, []string{"acme-2"}, []time.Time{mins(50)}},
		{api.ReasonSilentClaim, []string{"web-3"}, []time.Time{t0}},
		{api.ReasonFailed, []string{"acme-4"}, []time.Time{mins(40)}},
		{api.ReasonInReview, []string{"acme-3"}, []time.Time{mins(30)}},
		{api.ReasonBlockedByCancelled, []string{"acme-6"}, []time.Time{mins(60)}},
	}
	if len(v.Groups) != len(want) || v.Count != 7 {
		t.Fatalf("count %d, groups %+v", v.Count, v.Groups)
	}
	for i, w := range want {
		g := v.Groups[i]
		if g.Reason != w.reason || len(g.Tickets) != len(w.ids) {
			t.Fatalf("group %d = %+v, want %s %v", i, g, w.reason, w.ids)
		}
		for j, row := range g.Tickets {
			if row.ID != w.ids[j] || !row.Since.Equal(w.since[j]) {
				t.Fatalf("%s row %d = %s since %s, want %s since %s", w.reason, j, row.ID, row.Since, w.ids[j], w.since[j])
			}
		}
	}
}

func TestEachNeedsMeRowCarriesItsOneLineDetail(t *testing.T) {
	f := needsMeWorld(t)
	v, _ := f.core.NeedsMe(human)
	rows := map[string]api.NeedsMeRow{}
	for _, g := range v.Groups {
		for _, r := range g.Tickets {
			rows[r.ID] = r
		}
	}
	if r := rows["web-2"]; r.Project != "web" || r.Title != "W" || r.State != domain.NeedsAnswer ||
		r.Round == nil || *r.Round != (api.RoundSummary{Number: 1, Open: 1, Questions: 2}) {
		t.Fatalf("web-2 %+v round %+v", r, r.Round)
	}
	if r := rows["acme-4"]; r.Failure == nil || *r.Failure != (api.FailureSummary{Headline: "Login 500s", Reports: 1}) {
		t.Fatalf("acme-4 failure %+v", r.Failure)
	}
	if r := rows["acme-3"]; r.Summary != "Built the form." {
		t.Fatalf("acme-3 summary %q", r.Summary)
	}
	if r := rows["acme-6"]; r.Blocker == nil || r.Blocker.ID != "acme-7" || r.Blocker.State != domain.Cancelled {
		t.Fatalf("acme-6 blocker %+v", r.Blocker)
	}
	// Rows without that reason carry none of the others' detail.
	if r := rows["acme-2"]; r.Round != nil || r.Failure != nil || r.Summary != "" || r.Blocker != nil {
		t.Fatalf("acme-2 %+v", r)
	}
}

func TestNeedsMeIsEmptyWhenNothingWaits(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	v, err := f.core.NeedsMe(human)
	if err != nil || v.Count != 0 || v.Groups == nil || len(v.Groups) != 0 {
		t.Fatalf("got %+v, %v", v, err)
	}
}
