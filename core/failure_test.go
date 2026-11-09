package core_test

import (
	"slices"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

func report(ticket, claimID string) core.ReportFailureInput {
	return core.ReportFailureInput{
		Ticket: core.Ref(ticket), ClaimID: claimID,
		WhatBroke: "Login 500s: the token table is missing", Why: "Migration 12 never ran",
		WhatToFix: "Run migration 12", WhereWorkStands: "Branch acme-2 has the form, pushed",
	}
}

// fullReport is report asking for the Ticket back.
func fullReport(ticket, claimID string) core.ReportFailureInput {
	in := report(ticket, claimID)
	in.Full = true
	return in
}

// failed is acme-2 claimed (claim-1, session s1) and failed at t0+1h.
func (f fixture) failed(t *testing.T) {
	t.Helper()
	f.work(t, store.Ready)
	f.claim(t, core.ClaimInput{SessionID: "s1"})
	f.clock.Set(t0.Add(time.Hour))
	if _, err := f.core.ReportFailure(ctx, "acme", report("2", "claim-1")); err != nil {
		t.Fatalf("report_failure: %v", err)
	}
}

func TestReportFailureMovesToFailedEndsTheClaimAndSaysStop(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	f.claim(t, core.ClaimInput{SessionID: "s1"})
	f.clock.Set(t0.Add(time.Hour))

	res, err := f.core.ReportFailure(ctx, "acme", fullReport("acme-2", "claim-1"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Ticket.State != store.Failed || res.NextStep != "stop working this Ticket" {
		t.Fatalf("state %s, next step %q", res.Ticket.State, res.NextStep)
	}
	want := core.FailureReportView{
		Number: 1, Claim: 1, SessionID: "s1", Actor: "agent", Time: t0.Add(time.Hour),
		WhatBroke: "Login 500s: the token table is missing", Why: "Migration 12 never ran",
		WhatToFix: "Run migration 12", WhereWorkStands: "Branch acme-2 has the form, pushed",
	}
	if r := res.Ticket.FailureReports; len(r) != 1 || r[0] != want {
		t.Fatalf("failure reports %+v, want [%+v]", r, want)
	}
	c := res.Ticket.Claims[0]
	if c.Ended == nil || !c.Ended.Equal(t0.Add(time.Hour)) || c.EndedBy != "report_failure" {
		t.Fatalf("reporting a failure should end the Claim: %+v", c)
	}
	// The Claim has ended, so its ID is stale now.
	_, err = f.core.ReportFailure(ctx, "acme", report("2", "claim-1"))
	wantCode(t, err, core.StaleClaim, "")
}

func TestReportFailureNeedsEveryField(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	f.claim(t, core.ClaimInput{})
	for field, blank := range map[string]func(*core.ReportFailureInput){
		"what_broke":        func(in *core.ReportFailureInput) { in.WhatBroke = "" },
		"why":               func(in *core.ReportFailureInput) { in.Why = " " },
		"what_to_fix":       func(in *core.ReportFailureInput) { in.WhatToFix = "" },
		"where_work_stands": func(in *core.ReportFailureInput) { in.WhereWorkStands = "\n" },
	} {
		in := report("2", "claim-1")
		blank(&in)
		_, err := f.core.ReportFailure(ctx, "acme", in)
		wantCode(t, err, core.Invalid, field)
	}
	if got, _ := f.core.GetTicket(ctx, "acme", whole("2")); got.State != store.InProgress || len(got.FailureReports) != 0 {
		t.Fatalf("state %s, reports %d: want still In Progress with none", got.State, len(got.FailureReports))
	}
}

func TestReportFailureIsOnlyForTheHolderFromInProgress(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready, store.Ready)
	f.claim(t, core.ClaimInput{})
	// Not the holder, even as the human.
	_, err := f.core.ReportFailure(human, "acme", report("2", "claim-9"))
	wantCode(t, err, core.StaleClaim, "")
	// Not claimed at all.
	_, err = f.core.ReportFailure(ctx, "acme", report("3", ""))
	wantCode(t, err, core.StaleClaim, "")
}

func TestFailureReportsAreNumberedAndKeptAcrossClaims(t *testing.T) {
	f := newFixture(t)
	f.failed(t)
	if _, err := f.core.SendBack(human, "acme", core.SendBackInput{Ticket: "2"}); err != nil {
		t.Fatal(err)
	}
	f.claim(t, core.ClaimInput{SessionID: "s2"})
	f.clock.Set(t0.Add(2 * time.Hour))
	res, err := f.core.ReportFailure(ctx, "acme", fullReport("2", "claim-2"))
	if err != nil {
		t.Fatal(err)
	}
	r := res.Ticket.FailureReports
	if len(r) != 2 || r[0].Number != 1 || r[0].SessionID != "s1" || r[1].Number != 2 || r[1].Claim != 2 || r[1].SessionID != "s2" {
		t.Fatalf("failure reports %+v", r)
	}
}

func TestOnlyReportFailureEntersFailed(t *testing.T) {
	for _, a := range core.Workflow {
		if a.To == store.Failed && a.Name != "report_failure" {
			t.Errorf("%s moves to Failed", a.Name)
		}
		if a.To == "" && !slices.Contains(a.Except, store.Failed) {
			t.Errorf("%s can choose Failed", a.Name)
		}
	}
}

func TestSendBackMovesFailedToReadyKeepingItsPlace(t *testing.T) {
	f := newFixture(t)
	f.failed(t)
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Later", State: store.Ready})
	f.clock.Set(t0.Add(2 * time.Hour))

	v, err := record(f.core.SendBack(human, "acme", core.SendBackInput{Ticket: "2", Note: "Migration 12 is in now.", Full: true}))
	if err != nil {
		t.Fatal(err)
	}
	if v.State != store.Ready {
		t.Fatalf("state %s", v.State)
	}
	want := core.CommentView{Number: 1, Actor: "human", Time: t0.Add(2 * time.Hour), Body: "Migration 12 is in now."}
	if len(v.Comments) != 1 || v.Comments[0] != want {
		t.Fatalf("comments %+v, want [%+v]", v.Comments, want)
	}
	// It comes before the Ticket created after it.
	if res := f.claim(t, core.ClaimInput{}); res.Ticket.ID != "acme-2" {
		t.Fatalf("claimed %s, want acme-2 first", res.Ticket.ID)
	}
}

func TestSendBackWithoutANoteStoresNoComment(t *testing.T) {
	f := newFixture(t)
	f.failed(t)
	v, err := record(f.core.SendBack(human, "acme", core.SendBackInput{Ticket: "2", Full: true}))
	if err != nil {
		t.Fatal(err)
	}
	if v.State != store.Ready || len(v.Comments) != 0 {
		t.Fatalf("state %s, comments %+v", v.State, v.Comments)
	}
}

func TestToDraftMovesFailedToDraft(t *testing.T) {
	f := newFixture(t)
	f.failed(t)
	v, err := record(f.core.ToDraft(human, "acme", core.TicketInput{Ticket: "2", Full: true}))
	if err != nil {
		t.Fatal(err)
	}
	if v.State != store.Draft {
		t.Fatalf("state %s", v.State)
	}
}

func TestSendBackAndToDraftAreHumanOnlyFromFailed(t *testing.T) {
	f := newFixture(t)
	f.failed(t)
	_, err := f.core.SendBack(ctx, "acme", core.SendBackInput{Ticket: "2"})
	wantCode(t, err, core.ActionNotAllowed, "")
	_, err = f.core.ToDraft(ctx, "acme", core.TicketInput{Ticket: "2"})
	wantCode(t, err, core.ActionNotAllowed, "")

	f.core.SendBack(human, "acme", core.SendBackInput{Ticket: "2"})
	_, err = f.core.ToDraft(human, "acme", core.TicketInput{Ticket: "2"})
	wantCode(t, err, core.ActionNotAllowed, "")
}

func TestTheHumanMayCancelAFailedTicket(t *testing.T) {
	var ok bool
	for _, a := range core.Workflow {
		if a.Name == "cancel" && a.Who == core.HumanActor && a.To == store.Cancelled && slices.Contains(a.From, store.Failed) {
			ok = true
		}
	}
	if !ok {
		t.Fatal("the human's cancel should run from Failed")
	}
}

func TestGiveUpMovesInProgressToReadyWithTheNote(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	f.claim(t, core.ClaimInput{})
	f.clock.Set(t0.Add(time.Hour))

	res, err := f.core.GiveUp(ctx, "acme", core.GiveUpInput{Ticket: "2", ClaimID: "claim-1", Note: "Out of context; nothing is wrong.", Full: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Ticket.State != store.Ready || res.NextStep != "stop working this Ticket" {
		t.Fatalf("state %s, next step %q", res.Ticket.State, res.NextStep)
	}
	want := core.CommentView{Number: 1, Actor: "agent", Time: t0.Add(time.Hour), Claim: 1, Body: "Out of context; nothing is wrong."}
	if len(res.Ticket.Comments) != 1 || res.Ticket.Comments[0] != want {
		t.Fatalf("comments %+v, want [%+v]", res.Ticket.Comments, want)
	}
	if c := res.Ticket.Claims[0]; c.EndedBy != "give_up" {
		t.Fatalf("claim %+v", c)
	}
}

func TestGiveUpNeedsANoteAndTheHolder(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	f.claim(t, core.ClaimInput{})
	_, err := f.core.GiveUp(ctx, "acme", core.GiveUpInput{Ticket: "2", ClaimID: "claim-1", Note: " "})
	wantCode(t, err, core.Invalid, "note")
	_, err = f.core.GiveUp(ctx, "acme", core.GiveUpInput{Ticket: "2", ClaimID: "claim-9", Note: "n"})
	wantCode(t, err, core.StaleClaim, "")
	if got, _ := f.core.GetTicket(ctx, "acme", whole("2")); got.State != store.InProgress {
		t.Fatalf("state %s", got.State)
	}
}

func TestTheNextClaimSeesTheWholeHistory(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	f.claim(t, core.ClaimInput{SessionID: "s1"}) // t0+1m
	f.clock.Set(t0.Add(10 * time.Minute))
	f.core.Comment(ctx, "acme", core.CommentInput{Ticket: "2", Body: "Halfway."})
	f.clock.Set(t0.Add(time.Hour))
	f.core.ReportFailure(ctx, "acme", report("2", "claim-1"))
	f.clock.Set(t0.Add(2 * time.Hour))
	f.core.SendBack(human, "acme", core.SendBackInput{Ticket: "2", Note: "Fixed the migration."})
	f.clock.Set(t0.Add(3 * time.Hour))

	res := f.claim(t, core.ClaimInput{SessionID: "s2"})
	v := res.Ticket
	if len(v.Claims) != 2 {
		t.Fatalf("claims %+v", v.Claims)
	}
	past := v.Claims[0]
	if !past.Started.Equal(t0.Add(time.Minute)) || !past.LastActivity.Equal(t0.Add(time.Hour)) || past.EndedBy != "report_failure" {
		t.Fatalf("past claim %+v", past)
	}
	if len(v.FailureReports) != 1 {
		t.Fatalf("failure reports %+v", v.FailureReports)
	}
	if len(v.Comments) != 2 || v.Comments[1].Body != "Fixed the migration." {
		t.Fatalf("comments %+v", v.Comments)
	}
	got, _ := f.core.GetTicket(ctx, "acme", whole("2"))
	if len(got.Claims) != 2 || len(got.FailureReports) != 1 || len(got.Comments) != 2 {
		t.Fatalf("get_ticket %+v", got)
	}
}

func TestAClaimsLastActivityIsTheLatestAgentWriteDuringIt(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	f.claim(t, core.ClaimInput{}) // t0+1m
	got, _ := f.core.GetTicket(ctx, "acme", whole("2"))
	if la := got.Claims[0].LastActivity; !la.Equal(t0.Add(time.Minute)) {
		t.Fatalf("last activity %v, want the claim itself", la)
	}
	f.clock.Set(t0.Add(20 * time.Minute))
	f.core.Comment(ctx, "acme", core.CommentInput{Ticket: "2", Body: "Working."})
	f.clock.Set(t0.Add(30 * time.Minute))
	f.core.Comment(human, "acme", core.CommentInput{Ticket: "2", Body: "How is it going?"})
	got, _ = f.core.GetTicket(ctx, "acme", whole("2"))
	if la := got.Claims[0].LastActivity; !la.Equal(t0.Add(20 * time.Minute)) {
		t.Fatalf("last activity %v, want the agent's comment, not the human's", la)
	}
}
