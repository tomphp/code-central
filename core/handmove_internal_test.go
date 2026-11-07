package core

import (
	"context"
	"testing"
	"time"

	"github.com/tomoram/code-central/store"
)

// A human move out of Needs Answer dismisses the open Questions and ends
// the Claim. No human action leaving Needs Answer has an operation yet
// (cancel arrives with ticket 28, move with 32), so this drives the one
// transition every action goes through. Those tickets should add a test
// at the operation.
func TestAHumanMoveOutOfNeedsAnswerDismissesOpenQuestionsAndEndsTheClaim(t *testing.T) {
	t0 := time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC)
	tk := store.Ticket{
		Number: 2, State: store.NeedsAnswer,
		Claims: []store.Claim{{Number: 1, ID: "c1", Started: t0}},
		Rounds: []store.Round{{Number: 1, Claim: 1, Posted: t0, Questions: []store.Question{
			{Number: 1, Status: store.QuestionAnswered, Answer: "yes", Closed: t0},
			{Number: 2, Status: store.QuestionOpen},
		}}},
	}
	later := t0.Add(time.Hour)
	if err := transition(WithActor(context.Background(), Human), &tk, "acme", "cancel", "", later); err != nil {
		t.Fatal(err)
	}
	qs := tk.Rounds[0].Questions
	if qs[0].Status != store.QuestionAnswered || qs[0].Answer != "yes" {
		t.Fatalf("an answered Question changed: %+v", qs[0])
	}
	if qs[1].Status != store.QuestionDismissed || !qs[1].Closed.Equal(later) {
		t.Fatalf("the open Question was not dismissed: %+v", qs[1])
	}
	if c := tk.Claims[0]; !c.Ended.Equal(later) || c.EndedBy != "cancel" {
		t.Fatalf("the Claim did not end: %+v", c)
	}
}
