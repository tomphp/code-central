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
	v, err := f.core.Cancel(human, "acme", core.CancelInput{Ticket: "2"})
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
