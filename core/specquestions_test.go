package core_test

import (
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// The roll-up of a Spec's Questions: each of its Tickets that has Rounds,
// in claim order, with every Round and Question.
func TestGetSpecQuestionsRollsUpItsTicketsRounds(t *testing.T) {
	f := newFixture(t)
	f.asking(t, qDB, qName) // Spec acme-1, Ticket acme-2 asking
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "No questions"})
	other := f.spec(t, "acme", "Other")

	v, err := f.core.GetSpecQuestions(human, "acme", "acme-1")
	if err != nil {
		t.Fatal(err)
	}
	if v.Spec != "acme-1" || len(v.Tickets) != 1 {
		t.Fatalf("got %+v, want only acme-2", v)
	}
	tk := v.Tickets[0]
	if tk.ID != "acme-2" || tk.Title != "Ta" || tk.State != store.NeedsAnswer || len(tk.Rounds) != 1 {
		t.Fatalf("ticket %+v", tk)
	}
	if qs := tk.Rounds[0].Questions; len(qs) != 2 || qs[0].Title != "Which database?" || qs[1].Status != store.QuestionOpen {
		t.Fatalf("questions %+v", qs)
	}

	v, err = f.core.GetSpecQuestions(human, "acme", core.Ref(other.ID))
	if err != nil || v.Tickets == nil || len(v.Tickets) != 0 {
		t.Fatalf("a Spec without Questions: %+v, %v", v, err)
	}
	_, err = f.core.GetSpecQuestions(human, "acme", "acme-2")
	wantCode(t, err, core.NotFound, "")
	_, err = f.core.GetSpecQuestions(human, "nope", "1")
	wantCode(t, err, core.UnknownProject, "")
}
