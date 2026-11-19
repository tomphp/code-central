package core_test

import (
	"slices"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

// asking is acme-2 claimed as claim-1 (session sess-1) and asking a Round
// of the given Questions at t0+1h.
func (f fixture) asking(t *testing.T, qs ...api.QuestionInput) api.PostRoundResult {
	t.Helper()
	f.work(t, domain.Ready)
	f.claim(t, api.ClaimInput{SessionID: "sess-1"})
	f.clock.Set(t0.Add(time.Hour))
	res, err := f.core.PostRound(ctx, "acme", api.PostRoundInput{Ticket: "acme-2", ClaimID: "claim-1", Questions: qs})
	if err != nil {
		t.Fatalf("post_round: %v", err)
	}
	return res
}

var (
	qDB   = api.QuestionInput{Title: "Which database?", Body: "Postgres or SQLite?", Recommended: "SQLite"}
	qName = api.QuestionInput{Title: "Name?", Body: "What is it called?"}
)

func TestPostRoundStoresANumberedRoundAndMovesToNeedsAnswer(t *testing.T) {
	f := newFixture(t)
	res := f.asking(t, qDB, qName)
	if want := "End your turn now, after running `code-central wait acme-2` in the background if you can run background commands."; res.Round != 1 || res.NextStep != want {
		t.Fatalf("got round %d, next step %q; want round 1, %q", res.Round, res.NextStep, want)
	}
	v := f.get(t, "2")
	if v.State != domain.NeedsAnswer {
		t.Fatalf("state %s", v.State)
	}
	if len(v.Rounds) != 1 {
		t.Fatalf("rounds %+v", v.Rounds)
	}
	r := v.Rounds[0]
	if r.Number != 1 || r.Claim != 1 || r.SessionID != "sess-1" || !r.Posted.Equal(t0.Add(time.Hour)) {
		t.Fatalf("round %+v: the session ID defaults to the Claim's", r)
	}
	wantQs := []api.QuestionView{
		{Number: 1, Title: "Which database?", Body: "Postgres or SQLite?", Recommended: "SQLite", Status: domain.QuestionOpen},
		{Number: 2, Title: "Name?", Body: "What is it called?", Status: domain.QuestionOpen},
	}
	if !slices.EqualFunc(r.Questions, wantQs, sameQuestion) {
		t.Fatalf("questions %+v, want %+v", r.Questions, wantQs)
	}
	if c := v.Claims[0]; c.Ended != nil {
		t.Fatalf("posting a Round keeps the Claim: %+v", c)
	}
	wantMove := api.MoveView{From: domain.InProgress, To: domain.NeedsAnswer, Actor: "agent", Time: t0.Add(time.Hour), Claim: 1}
	if m := v.Moves[len(v.Moves)-1]; m != wantMove {
		t.Fatalf("move %+v, want %+v", m, wantMove)
	}
}

func sameQuestion(a, b api.QuestionView) bool {
	return a.Number == b.Number && a.Title == b.Title && a.Body == b.Body && a.Recommended == b.Recommended &&
		a.Status == b.Status && a.Answer == b.Answer && (a.Closed == nil) == (b.Closed == nil) &&
		(a.Closed == nil || a.Closed.Equal(*b.Closed))
}

func TestPostRoundRecordsAGivenSessionIDAndNumbersQuestionsAcrossRounds(t *testing.T) {
	f := newFixture(t)
	f.asking(t, qDB)
	if _, err := f.core.Answer(human, "acme", api.AnswerInput{Ticket: "2", Question: 1, Text: "SQLite"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.PickUpAnswers(ctx, "acme", api.PickUpAnswersInput{Ticket: "2", ClaimID: "claim-1"}); err != nil {
		t.Fatal(err)
	}
	res, err := f.core.PostRound(ctx, "acme", api.PostRoundInput{Ticket: "2", ClaimID: "claim-1", SessionID: "sess-2", Questions: []api.QuestionInput{qName}})
	if err != nil || res.Round != 2 {
		t.Fatalf("got %+v, %v", res, err)
	}
	r := f.get(t, "2").Rounds[1]
	if r.SessionID != "sess-2" || r.Questions[0].Number != 2 {
		t.Fatalf("round %+v: want session sess-2 and Question 2", r)
	}
	if c := f.get(t, "2").Claims[0]; c.SessionID != "sess-2" {
		t.Fatalf("claim %+v: a Round's session ID becomes the Claim's", c)
	}
}

func TestPostRoundRejections(t *testing.T) {
	cases := []struct {
		name  string
		in    api.PostRoundInput
		code  api.Code
		field string
	}{
		{"no questions", api.PostRoundInput{ClaimID: "claim-1"}, api.Invalid, "questions"},
		{"question without a title", api.PostRoundInput{ClaimID: "claim-1", Questions: []api.QuestionInput{qDB, {Body: "b"}}}, api.Invalid, "questions[1].title"},
		{"question without a body", api.PostRoundInput{ClaimID: "claim-1", Questions: []api.QuestionInput{{Title: "t"}}}, api.Invalid, "questions[0].body"},
		{"wrong Claim ID", api.PostRoundInput{ClaimID: "claim-9", Questions: []api.QuestionInput{qDB}}, api.StaleClaim, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			f.work(t, domain.Ready)
			f.claim(t, api.ClaimInput{})
			c.in.Ticket = "2"
			_, err := f.core.PostRound(ctx, "acme", c.in)
			wantCode(t, err, c.code, c.field)
			if v := f.get(t, "2"); v.State != domain.InProgress || len(v.Rounds) != 0 {
				t.Fatalf("a rejected Round changed the Ticket: %s, %+v", v.State, v.Rounds)
			}
		})
	}
}

func TestPostRoundOnlyFromInProgress(t *testing.T) {
	f := newFixture(t)
	f.asking(t, qDB)
	_, err := f.core.PostRound(ctx, "acme", api.PostRoundInput{Ticket: "2", ClaimID: "claim-1", Questions: []api.QuestionInput{qName}})
	wantCode(t, err, api.ActionNotAllowed, "")
}
