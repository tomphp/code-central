package core_test

import (
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

func (f fixture) move(t *testing.T, ref api.Ref, to domain.State) api.TicketView {
	t.Helper()
	v, err := record(f.core.Move(human, "acme", api.MoveInput{Ticket: ref, ToState: to, Full: true}))
	if err != nil {
		t.Fatalf("move %s to %s: %v", ref, to, err)
	}
	return v
}

func TestTheHumanMovesATicketToAnyStateButFailedAndNeedsAnswer(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Draft)
	for _, to := range []domain.State{domain.Ready, domain.InReview, domain.Done, domain.Draft, domain.Cancelled, domain.Ready} {
		f.clock.Advance(time.Minute)
		v := f.move(t, "2", to)
		last := v.Moves[len(v.Moves)-1]
		if v.State != to || last.To != to || last.Actor != "human" || !last.Time.Equal(f.clock.Now()) {
			t.Fatalf("move to %s: state %s, last move %+v", to, v.State, last)
		}
	}
	// Failed and Needs Answer are entered only by a Failure Report or a Round.
	for _, to := range []domain.State{domain.Failed, domain.NeedsAnswer} {
		_, err := f.core.Move(human, "acme", api.MoveInput{Ticket: "2", ToState: to})
		wantCode(t, err, api.Invalid, "to_state")
	}
	for _, to := range []domain.State{"", "Nope", domain.Ready} { // Ready: it is already there
		_, err := f.core.Move(human, "acme", api.MoveInput{Ticket: "2", ToState: to})
		wantCode(t, err, api.Invalid, "to_state")
	}
}

func TestMoveIsTheHumansAlone(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Draft)
	_, err := f.core.Move(ctx, "acme", api.MoveInput{Ticket: "2", ToState: domain.Ready})
	wantCode(t, err, api.ActionNotAllowed, "")
	_, err = f.core.Move(human, "acme", api.MoveInput{Ticket: "9", ToState: domain.Ready})
	wantCode(t, err, api.NotFound, "")
}

func TestMovingOutOfAClaimStateEndsTheClaim(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	f.claim(t, api.ClaimInput{})
	f.clock.Advance(time.Hour)
	v := f.move(t, "2", domain.InReview)
	if c := v.Claims[0]; c.Ended == nil || !c.Ended.Equal(f.clock.Now()) || c.EndedBy != "move" {
		t.Fatalf("claim %+v, want ended by move", c)
	}
	if m := v.Moves[len(v.Moves)-1]; m.Claim != 1 {
		t.Fatalf("the move should record the Claim it ended: %+v", m)
	}
}

func TestMovingWithinTheClaimStatesKeepsTheClaim(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	f.claim(t, api.ClaimInput{})
	v := f.move(t, "2", domain.Answered)
	if v.Claims[0].Ended != nil {
		t.Fatalf("claim %+v, want still live", v.Claims[0])
	}
}

func TestRenameProjectChangesTheDisplayNameNeverTheKey(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	p, err := f.core.RenameProject(human, "acme", api.RenameProjectInput{DisplayName: "Acme Web"})
	if err != nil || p.Key != "acme" || p.DisplayName != "Acme Web" || !p.Created.Equal(t0) {
		t.Fatalf("got %+v, %v", p, err)
	}
	if got, _ := f.core.GetProject(ctx, "acme"); got.DisplayName != "Acme Web" {
		t.Fatalf("get_project %+v", got)
	}
	if v := f.get(t, "acme-2"); v.ID != "acme-2" {
		t.Fatalf("Ticket IDs keep the key: %s", v.ID)
	}
	_, err = f.core.RenameProject(human, "acme", api.RenameProjectInput{DisplayName: "  "})
	wantCode(t, err, api.Invalid, "display_name")
	_, err = f.core.RenameProject(human, "nope", api.RenameProjectInput{DisplayName: "X"})
	wantCode(t, err, api.UnknownProject, "")
	_, err = f.core.RenameProject(ctx, "acme", api.RenameProjectInput{DisplayName: "X"})
	wantCode(t, err, api.ActionNotAllowed, "")
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
