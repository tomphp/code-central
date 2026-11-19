package core_test

import (
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

func TestAnsweringPartOfARoundMovesNothingAndAnswersStayEditable(t *testing.T) {
	f := newFixture(t)
	f.asking(t, qDB, qName)
	f.clock.Set(t0.Add(2 * time.Hour))
	v, err := record(f.core.Answer(human, "acme", api.AnswerInput{Ticket: "2", Question: 1, Text: "Postgres", Full: true}))
	if err != nil {
		t.Fatal(err)
	}
	if v.State != domain.NeedsAnswer {
		t.Fatalf("answering part of a Round moved the Ticket to %s", v.State)
	}
	f.clock.Set(t0.Add(3 * time.Hour))
	v, err = record(f.core.Answer(human, "acme", api.AnswerInput{Ticket: "2", Question: 1, Text: "SQLite after all", Full: true}))
	if err != nil {
		t.Fatal(err)
	}
	closed := t0.Add(3 * time.Hour)
	want := api.QuestionView{Number: 1, Title: qDB.Title, Body: qDB.Body, Recommended: "SQLite", Status: domain.QuestionAnswered, Answer: "SQLite after all", Closed: &closed}
	if q := v.Rounds[0].Questions[0]; !sameQuestion(q, want) {
		t.Fatalf("question %+v, want %+v", q, want)
	}
}

func TestTheLastAnswerMovesToAnsweredKeepingTheClaim(t *testing.T) {
	f := newFixture(t)
	f.asking(t, qDB, qName)
	f.clock.Set(t0.Add(2 * time.Hour))
	if _, err := f.core.DismissQuestion(human, "acme", api.DismissQuestionInput{Ticket: "2", Question: 2}); err != nil {
		t.Fatal(err)
	}
	v, err := record(f.core.Answer(human, "acme", api.AnswerInput{Ticket: "2", Question: 1, Text: "SQLite", Full: true}))
	if err != nil {
		t.Fatal(err)
	}
	if v.State != domain.Answered {
		t.Fatalf("state %s, want Answered", v.State)
	}
	if v.Claims[0].Ended != nil {
		t.Fatalf("Answered keeps the Claim: %+v", v.Claims[0])
	}
	wantMove := api.MoveView{From: domain.NeedsAnswer, To: domain.Answered, Actor: "human", Time: t0.Add(2 * time.Hour), Claim: 1}
	if m := v.Moves[len(v.Moves)-1]; m != wantMove {
		t.Fatalf("move %+v, want %+v", m, wantMove)
	}
	if q := v.Rounds[0].Questions[1]; q.Status != domain.QuestionDismissed || q.Answer != "" {
		t.Fatalf("dismissed question %+v", q)
	}

	_, err = f.core.Answer(human, "acme", api.AnswerInput{Ticket: "2", Question: 1, Text: "Postgres"})
	wantCode(t, err, api.ActionNotAllowed, "")
	_, err = f.core.DismissQuestion(human, "acme", api.DismissQuestionInput{Ticket: "2", Question: 1})
	wantCode(t, err, api.ActionNotAllowed, "")
}

func TestDismissingTheOnlyOpenQuestionMovesToAnswered(t *testing.T) {
	f := newFixture(t)
	f.asking(t, qName)
	v, err := record(f.core.DismissQuestion(human, "acme", api.DismissQuestionInput{Ticket: "2", Question: 1, Full: true}))
	if err != nil || v.State != domain.Answered {
		t.Fatalf("got %s, %v", v.State, err)
	}
}

func TestAcceptRecommendedAnswersEveryOpenQuestionWithItsRecommendation(t *testing.T) {
	f := newFixture(t)
	qCache := api.QuestionInput{Title: "Cache?", Body: "Add a cache?", Recommended: "No"}
	f.asking(t, qDB, qName, qCache)
	if _, err := f.core.Answer(human, "acme", api.AnswerInput{Ticket: "2", Question: 3, Text: "Yes"}); err != nil {
		t.Fatal(err)
	}
	v, err := record(f.core.AcceptRecommended(human, "acme", api.AcceptRecommendedInput{Ticket: "2", Round: 1, Full: true}))
	if err != nil {
		t.Fatal(err)
	}
	qs := v.Rounds[0].Questions
	if qs[0].Answer != "SQLite" || qs[0].Status != domain.QuestionAnswered || qs[1].Status != domain.QuestionOpen || qs[2].Answer != "Yes" {
		t.Fatalf("questions %+v: only open ones with a recommendation are answered", qs)
	}
	if v.State != domain.NeedsAnswer {
		t.Fatalf("state %s: Question 2 is still open", v.State)
	}

	f2 := newFixture(t)
	f2.asking(t, qDB)
	if v, err := record(f2.core.AcceptRecommended(human, "acme", api.AcceptRecommendedInput{Ticket: "2", Round: 1, Full: true})); err != nil || v.State != domain.Answered {
		t.Fatalf("got %s, %v", v.State, err)
	}
	_, err = f2.core.AcceptRecommended(human, "acme", api.AcceptRecommendedInput{Ticket: "2", Round: 1})
	wantCode(t, err, api.ActionNotAllowed, "")
}

func TestRoundActionRejections(t *testing.T) {
	f := newFixture(t)
	f.asking(t, qDB)
	_, err := f.core.Answer(ctx, "acme", api.AnswerInput{Ticket: "2", Question: 1, Text: "SQLite"})
	wantCode(t, err, api.ActionNotAllowed, "")
	_, err = f.core.Answer(human, "acme", api.AnswerInput{Ticket: "2", Question: 1, Text: " "})
	wantCode(t, err, api.Invalid, "text")
	_, err = f.core.Answer(human, "acme", api.AnswerInput{Ticket: "2", Question: 9, Text: "x"})
	wantCode(t, err, api.NotFound, "")
	_, err = f.core.DismissQuestion(ctx, "acme", api.DismissQuestionInput{Ticket: "2", Question: 1})
	wantCode(t, err, api.ActionNotAllowed, "")
	_, err = f.core.AcceptRecommended(human, "acme", api.AcceptRecommendedInput{Ticket: "2", Round: 2})
	wantCode(t, err, api.NotFound, "")
	if v := f.get(t, "2"); v.State != domain.NeedsAnswer || v.Rounds[0].Questions[0].Status != domain.QuestionOpen {
		t.Fatalf("a rejected action changed the Ticket: %+v", v)
	}
}

func TestPickUpAnswersReturnsTheRoundAndCommentsSinceIt(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	f.claim(t, api.ClaimInput{})
	if _, err := f.core.Comment(ctx, "acme", api.CommentInput{Ticket: "2", Body: "Before the Round."}); err != nil {
		t.Fatal(err)
	}
	f.clock.Set(t0.Add(time.Hour))
	if _, err := f.core.PostRound(ctx, "acme", api.PostRoundInput{Ticket: "2", ClaimID: "claim-1", Questions: []api.QuestionInput{qDB, qName}}); err != nil {
		t.Fatal(err)
	}
	f.clock.Set(t0.Add(2 * time.Hour))
	if _, err := f.core.Comment(human, "acme", api.CommentInput{Ticket: "2", Body: "Keep it small."}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.Answer(human, "acme", api.AnswerInput{Ticket: "2", Question: 1, Text: "SQLite"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.DismissQuestion(human, "acme", api.DismissQuestionInput{Ticket: "2", Question: 2}); err != nil {
		t.Fatal(err)
	}

	_, err := f.core.PickUpAnswers(ctx, "acme", api.PickUpAnswersInput{Ticket: "2", ClaimID: "claim-9"})
	wantCode(t, err, api.StaleClaim, "")
	f.clock.Set(t0.Add(3 * time.Hour))
	res, err := f.core.PickUpAnswers(ctx, "acme", api.PickUpAnswersInput{Ticket: "2", ClaimID: "claim-1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Ticket != "acme-2" || res.Round.Number != 1 || res.Round.Questions[0].Answer != "SQLite" || res.Round.Questions[1].Status != domain.QuestionDismissed {
		t.Fatalf("result %+v", res)
	}
	if len(res.Comments) != 1 || res.Comments[0].Body != "Keep it small." {
		t.Fatalf("comments %+v, want only the one since the Round", res.Comments)
	}
	v := f.get(t, "2")
	wantMove := api.MoveView{From: domain.Answered, To: domain.InProgress, Actor: "agent", Time: t0.Add(3 * time.Hour), Claim: 1}
	if v.State != domain.InProgress || v.Moves[len(v.Moves)-1] != wantMove || v.Claims[0].Ended != nil {
		t.Fatalf("state %s, moves %+v, claim %+v", v.State, v.Moves, v.Claims[0])
	}
	_, err = f.core.PickUpAnswers(ctx, "acme", api.PickUpAnswersInput{Ticket: "2", ClaimID: "claim-1"})
	wantCode(t, err, api.ActionNotAllowed, "")
}

func TestPickUpAnswersWaitsForTheLastAnswer(t *testing.T) {
	f := newFixture(t)
	f.asking(t, qDB)
	_, err := f.core.PickUpAnswers(ctx, "acme", api.PickUpAnswersInput{Ticket: "2", ClaimID: "claim-1"})
	wantCode(t, err, api.ActionNotAllowed, "")
}
