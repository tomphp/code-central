package core_test

import (
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// A human move out of Needs Answer dismisses the open Questions and ends
// the Claim. cancel is the human's move here; `move` (ticket 32) goes
// through the same transition.
func TestAHumanMoveOutOfNeedsAnswerDismissesOpenQuestionsAndEndsTheClaim(t *testing.T) {
	f := newFixture(t)
	f.asking(t, qDB, qName)
	if _, err := f.core.Answer(human, "acme", core.AnswerInput{Ticket: "2", Question: 1, Text: "Postgres"}); err != nil {
		t.Fatal(err)
	}
	later := t0.Add(2 * time.Hour)
	f.clock.Set(later)
	v, err := record(f.core.Cancel(human, "acme", core.CancelInput{Ticket: "2", Full: true}))
	if err != nil {
		t.Fatal(err)
	}
	qs := v.Rounds[0].Questions
	if qs[0].Status != store.QuestionAnswered || qs[0].Answer != "Postgres" {
		t.Fatalf("an answered Question changed: %+v", qs[0])
	}
	if qs[1].Status != store.QuestionDismissed || qs[1].Closed == nil || !qs[1].Closed.Equal(later) {
		t.Fatalf("the open Question was not dismissed: %+v", qs[1])
	}
	if c := v.Claims[0]; c.Ended == nil || !c.Ended.Equal(later) || c.EndedBy != "cancel" {
		t.Fatalf("the Claim did not end: %+v", c)
	}
}

// A human move from Needs Answer to In Progress dismisses the open
// Questions but keeps the Claim: In Progress is a Claim State, so the
// Claim can still go Silent.
func TestAHumanMoveFromNeedsAnswerToInProgressKeepsTheClaim(t *testing.T) {
	f := newFixture(t)
	f.asking(t, qDB)
	moved := t0.Add(2 * time.Hour)
	f.clock.Set(moved)
	v, err := record(f.core.Move(human, "acme", core.MoveInput{Ticket: "2", ToState: store.InProgress, Full: true}))
	if err != nil {
		t.Fatal(err)
	}
	if q := v.Rounds[0].Questions[0]; q.Status != store.QuestionDismissed {
		t.Fatalf("the open Question was not dismissed: %+v", q)
	}
	if c := v.Claims[0]; c.Ended != nil {
		t.Fatalf("the Claim ended on a move to a Claim State: %+v", c)
	}
	f.clock.Set(moved.Add(core.SilentAfter))
	wantSilentSince(t, f.get(t, "acme-2"), moved)
}

// A human move from Needs Answer to Answered keeps the Claim, so the
// holder can pick up the answers.
func TestAHumanMoveFromNeedsAnswerToAnsweredLetsTheHolderPickUpAnswers(t *testing.T) {
	f := newFixture(t)
	f.asking(t, qDB)
	if _, err := f.core.Move(human, "acme", core.MoveInput{Ticket: "2", ToState: store.Answered}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.PickUpAnswers(ctx, "acme", core.PickUpAnswersInput{Ticket: "2", ClaimID: "claim-1"}); err != nil {
		t.Fatalf("pick_up_answers after a human move to Answered: %v", err)
	}
	if v := f.get(t, "2"); v.State != store.InProgress || v.Claims[0].Ended != nil {
		t.Fatalf("state %s, claim %+v", v.State, v.Claims[0])
	}
}
