package core_test

import (
	"context"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/events"
)

func TestEveryWriteSendsAChangeNotice(t *testing.T) {
	f := newFixture(t)
	notices, stop := f.core.Events().Subscribe()
	defer stop()
	f.work(t, domain.Ready)
	want := []events.Event{
		{Type: events.ProjectChanged, Project: "acme"},
		{Type: events.SpecChanged, Project: "acme", ID: "acme-1"},
		{Type: events.SpecChanged, Project: "acme", ID: "acme-1"}, // the long-lived Label
		{Type: events.TicketChanged, Project: "acme", ID: "acme-2"},
	}
	for _, w := range want {
		if got := <-notices; got != w {
			t.Fatalf("got %+v, want %+v", got, w)
		}
	}
	// A rejected write sends nothing.
	if _, err := f.core.Comment(ctx, "acme", api.CommentInput{Ticket: "2", Body: ""}); err == nil {
		t.Fatal("empty comment accepted")
	}
	if _, err := f.core.Comment(ctx, "acme", api.CommentInput{Ticket: "2", Body: "hi"}); err != nil {
		t.Fatal(err)
	}
	if got := <-notices; got != want[3] {
		t.Fatalf("got %+v, want %+v", got, want[3])
	}
}

func TestWaitReturnsAtOnceWhenTheTicketHasLeftTheState(t *testing.T) {
	f := newFixture(t)
	f.asking(t, qDB)
	res, err := f.core.Wait(ctx, "acme", "acme-2", domain.InProgress)
	if err != nil {
		t.Fatal(err)
	}
	wantMove := api.MoveView{From: domain.InProgress, To: domain.NeedsAnswer, Actor: "agent", Time: t0.Add(time.Hour), Claim: 1}
	if res.Ticket != "acme-2" || res.State != domain.NeedsAnswer || res.LastMove == nil || *res.LastMove != wantMove {
		t.Fatalf("got %+v", res)
	}
}

func (f fixture) waitAsync(ref api.Ref, from domain.State) chan api.WaitResult {
	out := make(chan api.WaitResult, 1)
	go func() {
		res, err := f.core.Wait(ctx, "acme", ref, from)
		if err != nil {
			res.State = domain.State("error: " + err.Error())
		}
		out <- res
	}()
	return out
}

func TestWaitWakesWhenTheTicketLeavesTheState(t *testing.T) {
	f := newFixture(t)
	f.asking(t, qDB)
	done := f.waitAsync("2", domain.NeedsAnswer)
	f.clock.BlockUntil(1)
	// A change that leaves the State alone doesn't end the wait.
	if _, err := f.core.Comment(human, "acme", api.CommentInput{Ticket: "2", Body: "Thinking."}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.Answer(human, "acme", api.AnswerInput{Ticket: "2", Question: 1, Text: "SQLite"}); err != nil {
		t.Fatal(err)
	}
	if res := <-done; res.State != domain.Answered {
		t.Fatalf("got %+v", res)
	}
}

func TestWaitGivesUpAfterSixtySecondsOnTheClock(t *testing.T) {
	f := newFixture(t)
	f.asking(t, qDB)
	done := f.waitAsync("2", domain.NeedsAnswer)
	f.clock.BlockUntil(1)
	f.clock.Advance(59 * time.Second)
	select {
	case res := <-done:
		t.Fatalf("returned before 60 seconds: %+v", res)
	default:
	}
	f.clock.Advance(time.Second)
	if res := <-done; res.State != domain.NeedsAnswer {
		t.Fatalf("got %+v", res)
	}
}

func TestWaitEndsWhenItsRequestIsCancelled(t *testing.T) {
	f := newFixture(t)
	f.asking(t, qDB)
	cctx, cancel := context.WithCancel(ctx)
	done := make(chan api.WaitResult)
	go func() {
		res, _ := f.core.Wait(cctx, "acme", "2", domain.NeedsAnswer)
		done <- res
	}()
	f.clock.BlockUntil(1)
	cancel()
	if res := <-done; res.State != domain.NeedsAnswer {
		t.Fatalf("got %+v", res)
	}
}

func TestWaitRejections(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	_, err := f.core.Wait(ctx, "acme", "2", "Waiting")
	wantCode(t, err, api.Invalid, "from")
	_, err = f.core.Wait(ctx, "acme", "2", "")
	wantCode(t, err, api.Invalid, "from")
	_, err = f.core.Wait(ctx, "acme", "9", domain.Ready)
	wantCode(t, err, api.NotFound, "")
	_, err = f.core.Wait(ctx, "nope", "9", domain.Ready)
	wantCode(t, err, api.UnknownProject, "")
}
