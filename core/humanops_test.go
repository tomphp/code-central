package core_test

import (
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

func (f fixture) move(t *testing.T, ref core.Ref, to store.State) core.TicketView {
	t.Helper()
	v, err := f.core.Move(human, "acme", core.MoveInput{Ticket: ref, ToState: to})
	if err != nil {
		t.Fatalf("move %s to %s: %v", ref, to, err)
	}
	return v
}

func TestTheHumanMovesATicketToAnyStateButFailedAndNeedsAnswer(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Draft)
	for _, to := range []store.State{store.Ready, store.InReview, store.Done, store.Draft, store.Cancelled, store.Ready} {
		f.clock.Advance(time.Minute)
		v := f.move(t, "2", to)
		last := v.Moves[len(v.Moves)-1]
		if v.State != to || last.To != to || last.Actor != "human" || !last.Time.Equal(f.clock.Now()) {
			t.Fatalf("move to %s: state %s, last move %+v", to, v.State, last)
		}
	}
	// Failed and Needs Answer are entered only by a Failure Report or a Round.
	for _, to := range []store.State{store.Failed, store.NeedsAnswer} {
		_, err := f.core.Move(human, "acme", core.MoveInput{Ticket: "2", ToState: to})
		wantCode(t, err, core.Invalid, "to_state")
	}
	for _, to := range []store.State{"", "Nope", store.Ready} { // Ready: it is already there
		_, err := f.core.Move(human, "acme", core.MoveInput{Ticket: "2", ToState: to})
		wantCode(t, err, core.Invalid, "to_state")
	}
}

func TestMoveIsTheHumansAlone(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Draft)
	_, err := f.core.Move(ctx, "acme", core.MoveInput{Ticket: "2", ToState: store.Ready})
	wantCode(t, err, core.ActionNotAllowed, "")
	_, err = f.core.Move(human, "acme", core.MoveInput{Ticket: "9", ToState: store.Ready})
	wantCode(t, err, core.NotFound, "")
}

func TestMovingOutOfAClaimStateEndsTheClaim(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	f.claim(t, core.ClaimInput{})
	f.clock.Advance(time.Hour)
	v := f.move(t, "2", store.InReview)
	if c := v.Claims[0]; c.Ended == nil || !c.Ended.Equal(f.clock.Now()) || c.EndedBy != "move" {
		t.Fatalf("claim %+v, want ended by move", c)
	}
	if m := v.Moves[len(v.Moves)-1]; m.Claim != 1 {
		t.Fatalf("the move should record the Claim it ended: %+v", m)
	}
}

func TestMovingWithinTheClaimStatesKeepsTheClaim(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	f.claim(t, core.ClaimInput{})
	v := f.move(t, "2", store.Answered)
	if v.Claims[0].Ended != nil {
		t.Fatalf("claim %+v, want still live", v.Claims[0])
	}
}

func TestMovingOutOfNeedsAnswerDismissesItsOpenQuestions(t *testing.T) {
	f := newFixture(t)
	f.asking(t, qDB)
	v := f.move(t, "2", store.InProgress)
	if v.Rounds[0].Questions[0].Status != store.QuestionDismissed || v.Claims[0].Ended == nil {
		t.Fatalf("questions %+v claims %+v", v.Rounds[0].Questions, v.Claims)
	}
}

func TestRenameProjectChangesTheDisplayNameNeverTheKey(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	p, err := f.core.RenameProject(human, "acme", core.RenameProjectInput{DisplayName: "Acme Web"})
	if err != nil || p.Key != "acme" || p.DisplayName != "Acme Web" || !p.Created.Equal(t0) {
		t.Fatalf("got %+v, %v", p, err)
	}
	if got, _ := f.core.GetProject(ctx, "acme"); got.DisplayName != "Acme Web" {
		t.Fatalf("get_project %+v", got)
	}
	if v := f.get(t, "acme-2"); v.ID != "acme-2" {
		t.Fatalf("Ticket IDs keep the key: %s", v.ID)
	}
	_, err = f.core.RenameProject(human, "acme", core.RenameProjectInput{DisplayName: "  "})
	wantCode(t, err, core.Invalid, "display_name")
	_, err = f.core.RenameProject(human, "nope", core.RenameProjectInput{DisplayName: "X"})
	wantCode(t, err, core.UnknownProject, "")
	_, err = f.core.RenameProject(ctx, "acme", core.RenameProjectInput{DisplayName: "X"})
	wantCode(t, err, core.ActionNotAllowed, "")
}

func TestListProjectsListsEveryProjectWithItsNeedsMeCount(t *testing.T) {
	f := needsMeWorld(t)
	f.project(t, "zed")
	ps, err := f.core.ListProjects(human)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		key string
		n   int
	}{{"acme", 5}, {"web", 2}, {"zed", 0}}
	if len(ps) != len(want) {
		t.Fatalf("projects %+v", ps)
	}
	for i, w := range want {
		if ps[i].Key != w.key || ps[i].NeedsMe != w.n || ps[i].DisplayName != w.key {
			t.Fatalf("project %d = %+v, want %s with %d", i, ps[i], w.key, w.n)
		}
	}
}
