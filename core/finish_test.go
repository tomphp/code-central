package core_test

import (
	"slices"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// inReview is acme-2 claimed (claim-1) and finished, at t0+1h.
func (f fixture) inReview(t *testing.T) {
	t.Helper()
	f.work(t, store.Ready)
	f.claim(t, core.ClaimInput{})
	f.clock.Set(t0.Add(time.Hour))
	if _, err := f.core.Finish(ctx, "acme", core.FinishInput{Ticket: "2", ClaimID: "claim-1", Summary: "Done on acme-2."}); err != nil {
		t.Fatalf("finish: %v", err)
	}
}

func TestFinishMovesToInReviewWithTheSummaryAsAComment(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	f.claim(t, core.ClaimInput{})
	f.clock.Set(t0.Add(time.Hour))

	res, err := f.core.Finish(ctx, "acme", core.FinishInput{Ticket: "acme-2", ClaimID: "claim-1", Summary: "Built on branch acme-2."})
	if err != nil {
		t.Fatal(err)
	}
	if res.Ticket.State != store.InReview || res.NextStep != "stop working this Ticket" {
		t.Fatalf("state %s, next step %q", res.Ticket.State, res.NextStep)
	}
	wantComment := core.CommentView{Number: 1, Actor: "agent", Time: t0.Add(time.Hour), Claim: 1, Body: "Built on branch acme-2."}
	if len(res.Ticket.Comments) != 1 || res.Ticket.Comments[0] != wantComment {
		t.Fatalf("comments %+v, want [%+v]", res.Ticket.Comments, wantComment)
	}
	wantMove := core.MoveView{From: store.InProgress, To: store.InReview, Actor: "agent", Time: t0.Add(time.Hour), Claim: 1}
	if m := res.Ticket.Moves; len(m) != 2 || m[1] != wantMove {
		t.Fatalf("moves %+v, want the finish %+v last", m, wantMove)
	}
	c := res.Ticket.Claims[0]
	if c.Ended == nil || !c.Ended.Equal(t0.Add(time.Hour)) || c.EndedBy != "finish" {
		t.Fatalf("finishing should end the Claim: %+v", c)
	}
}

func TestFinishNeedsASummary(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	f.claim(t, core.ClaimInput{})
	_, err := f.core.Finish(ctx, "acme", core.FinishInput{Ticket: "2", ClaimID: "claim-1", Summary: " "})
	wantCode(t, err, core.Invalid, "summary")
	if got, _ := f.core.GetTicket(ctx, "acme", "2"); got.State != store.InProgress {
		t.Fatalf("state %s, want still In Progress", got.State)
	}
}

func TestAWriteWithAClaimIDThatIsNotTheLiveClaimIsStale(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	f.claim(t, core.ClaimInput{})
	for _, id := range []string{"", "claim-9"} {
		_, err := f.core.Finish(ctx, "acme", core.FinishInput{Ticket: "2", ClaimID: id, Summary: "S"})
		wantCode(t, err, core.StaleClaim, "")
	}
	// Once finished, the Claim has ended: its own ID is stale too.
	f.core.Finish(ctx, "acme", core.FinishInput{Ticket: "2", ClaimID: "claim-1", Summary: "S"})
	_, err := f.core.Finish(ctx, "acme", core.FinishInput{Ticket: "2", ClaimID: "claim-1", Summary: "S"})
	wantCode(t, err, core.StaleClaim, "")
}

func TestAcceptMovesInReviewToDoneAsTheHuman(t *testing.T) {
	f := newFixture(t)
	f.inReview(t)
	f.clock.Set(t0.Add(2 * time.Hour))

	v, err := f.core.Accept(human, "acme", core.TicketInput{Ticket: "2"})
	if err != nil {
		t.Fatal(err)
	}
	want := core.MoveView{From: store.InReview, To: store.Done, Actor: "human", Time: t0.Add(2 * time.Hour)}
	if v.State != store.Done || v.Moves[len(v.Moves)-1] != want {
		t.Fatalf("state %s, moves %+v; want last move %+v", v.State, v.Moves, want)
	}

	_, err = f.core.Accept(human, "acme", core.TicketInput{Ticket: "2"})
	wantCode(t, err, core.ActionNotAllowed, "")
}

func TestAcceptAndRejectAreHumanActions(t *testing.T) {
	f := newFixture(t)
	f.inReview(t)
	_, err := f.core.Accept(ctx, "acme", core.TicketInput{Ticket: "2"})
	wantCode(t, err, core.ActionNotAllowed, "")
	_, err = f.core.Reject(core.WithActor(ctx, core.Integration), "acme", core.RejectInput{Ticket: "2", Comment: "No."})
	wantCode(t, err, core.ActionNotAllowed, "")
	if got, _ := f.core.GetTicket(ctx, "acme", "2"); got.State != store.InReview {
		t.Fatalf("state %s, want still In Review", got.State)
	}
}

func TestRejectSendsTheTicketBackToReadyWithTheComment(t *testing.T) {
	f := newFixture(t)
	f.inReview(t)
	f.clock.Set(t0.Add(2 * time.Hour))

	_, err := f.core.Reject(human, "acme", core.RejectInput{Ticket: "2", Comment: ""})
	wantCode(t, err, core.Invalid, "comment")

	v, err := f.core.Reject(human, "acme", core.RejectInput{Ticket: "2", Comment: "The button is missing."})
	if err != nil {
		t.Fatal(err)
	}
	wantComment := core.CommentView{Number: 2, Actor: "human", Time: t0.Add(2 * time.Hour), Body: "The button is missing."}
	if v.State != store.Ready || v.Comments[1] != wantComment {
		t.Fatalf("state %s, comments %+v; want Ready with %+v", v.State, v.Comments, wantComment)
	}
	if !v.Claimable {
		t.Fatal("a rejected Ticket should be claimable again")
	}
}

func TestARejectedTicketNeedsAFreshClaimAndKeepsItsPlace(t *testing.T) {
	f := newFixture(t)
	f.inReview(t)                                                                              // acme-2, created first
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Newer", State: store.Ready}) // acme-3
	f.core.Reject(human, "acme", core.RejectInput{Ticket: "2", Comment: "Again."})

	_, err := f.core.Finish(ctx, "acme", core.FinishInput{Ticket: "2", ClaimID: "claim-1", Summary: "S"})
	wantCode(t, err, core.StaleClaim, "")

	res := f.claim(t, core.ClaimInput{})
	if res.Ticket.ID != "acme-2" || res.ClaimID != "claim-2" {
		t.Fatalf("claimed %s with %q, want acme-2 (front of the claim order) with a fresh claim-2", res.Ticket.ID, res.ClaimID)
	}
	if n := len(res.Ticket.Claims); n != 2 || res.Ticket.Claims[1].Number != 2 {
		t.Fatalf("claims %+v, want the past Claim and a new one", res.Ticket.Claims)
	}
}

func TestEveryActionIsCheckedAgainstTheOneTable(t *testing.T) {
	if got := len(store.States); got != 9 {
		t.Fatalf("%d States, want nine", got)
	}
	for _, a := range core.Workflow {
		for _, s := range append(slices.Clone(a.From), a.To) {
			if s != "" && !s.Valid() {
				t.Errorf("%s names unknown State %q", a.Name, s)
			}
		}
	}
	// A rejected action names what is allowed from the Ticket's State.
	f := newFixture(t)
	f.inReview(t)
	_, err := f.core.Finish(ctx, "acme", core.FinishInput{Ticket: "2", ClaimID: "claim-1", Summary: "S"})
	wantCode(t, err, core.StaleClaim, "")
	_, err = f.core.Accept(ctx, "acme", core.TicketInput{Ticket: "2"})
	e := err.(*core.Error)
	if len(e.AllowedActions) != 0 {
		t.Fatalf("an agent may do nothing from In Review, got %v", e.AllowedActions)
	}
	f.core.Accept(human, "acme", core.TicketInput{Ticket: "2"})
	_, err = f.core.Reject(human, "acme", core.RejectInput{Ticket: "2", Comment: "late"})
	e = err.(*core.Error)
	if !slices.Equal(e.AllowedActions, []string{"move"}) {
		t.Fatalf("allowed from Done for the human: %v, want [move]", e.AllowedActions)
	}
	// approve and defer share the table, so their rejections are complete too.
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "R", State: store.Ready}) // acme-3
	_, err = f.core.Approve(ctx, "acme", core.TicketInput{Ticket: "3"})
	e = err.(*core.Error)
	if !slices.Equal(e.AllowedActions, []string{"defer", "claim", "cancel"}) {
		t.Fatalf("allowed from Ready for an agent: %v, want [defer claim cancel]", e.AllowedActions)
	}
}
