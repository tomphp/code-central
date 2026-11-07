package core_test

import (
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

func TestSetSessionUpdatesTheLiveClaimsSessionID(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	f.claim(t, core.ClaimInput{SessionID: "old"})

	v, err := f.core.SetSession(ctx, "acme", core.SetSessionInput{Ticket: "acme-2", ClaimID: "claim-1", SessionID: "new"})
	if err != nil {
		t.Fatal(err)
	}
	if v.Claims[0].SessionID != "new" {
		t.Fatalf("claims %+v, want session new", v.Claims)
	}
	// The session ID is never checked: any value, even none, is taken.
	if v, err = f.core.SetSession(ctx, "acme", core.SetSessionInput{Ticket: "2", ClaimID: "claim-1"}); err != nil || v.Claims[0].SessionID != "" {
		t.Fatalf("got %+v, %v", v.Claims, err)
	}
	// Pings now follow the new session.
	f.core.SetSession(ctx, "acme", core.SetSessionInput{Ticket: "2", ClaimID: "claim-1", SessionID: "new"})
	f.clock.Advance(40 * time.Minute)
	f.core.Ping(ctx, "new")
	wantSilentSince(t, f.get(t, "acme-2"), time.Time{})
}

func TestSetSessionNeedsTheLiveClaimID(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready, store.Ready)
	f.claim(t, core.ClaimInput{Ticket: "2"})
	_, err := f.core.SetSession(ctx, "acme", core.SetSessionInput{Ticket: "2", ClaimID: "claim-9", SessionID: "s"})
	wantCode(t, err, core.StaleClaim, "")
	_, err = f.core.SetSession(ctx, "acme", core.SetSessionInput{Ticket: "3", ClaimID: "claim-1", SessionID: "s"})
	wantCode(t, err, core.StaleClaim, "")
}

func TestTheHumanReleasesAClaimBackToReady(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready, store.Ready)
	f.claim(t, core.ClaimInput{Ticket: "2"})
	f.clock.Advance(time.Hour)
	released := f.clock.Now()

	v, err := f.core.Release(human, "acme", core.ReleaseInput{Ticket: "2", Note: "session died"})
	if err != nil {
		t.Fatal(err)
	}
	if v.State != store.Ready || v.SilentSince != nil {
		t.Fatalf("state %s silent %v, want Ready and not Silent", v.State, v.SilentSince)
	}
	c := v.Claims[0]
	if c.Ended == nil || !c.Ended.Equal(released) || c.EndedBy != "release" {
		t.Fatalf("release should end the Claim: %+v", c)
	}
	want := core.CommentView{Number: 1, Actor: "human", Time: released, Claim: 1, Body: "session died"}
	if len(v.Comments) != 1 || v.Comments[0] != want {
		t.Fatalf("comments %+v, want [%+v]", v.Comments, want)
	}
	// The released session's next write is stale.
	_, err = f.core.Finish(ctx, "acme", core.FinishInput{Ticket: "2", ClaimID: "claim-1", Summary: "S"})
	wantCode(t, err, core.StaleClaim, "")

	// The note is optional.
	f.claim(t, core.ClaimInput{Ticket: "3"})
	if v, err = f.core.Release(human, "acme", core.ReleaseInput{Ticket: "3"}); err != nil || len(v.Comments) != 0 {
		t.Fatalf("got %+v, %v", v.Comments, err)
	}
}

func TestReleaseIsHumanOnlyFromInProgressOrAnswered(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready, store.Ready)
	f.claim(t, core.ClaimInput{Ticket: "2"})
	_, err := f.core.Release(ctx, "acme", core.ReleaseInput{Ticket: "2"})
	wantCode(t, err, core.ActionNotAllowed, "")
	_, err = f.core.Release(human, "acme", core.ReleaseInput{Ticket: "3"}) // Ready
	wantCode(t, err, core.ActionNotAllowed, "")

	f.claim(t, core.ClaimInput{Ticket: "3"})
	f.store.Update(func(w store.Writer) error { // Answered arrives with ticket 29
		tk, _ := w.Ticket("acme", 3)
		tk.State = store.Answered
		return w.PutTicket("acme", tk)
	})
	if v, err := f.core.Release(human, "acme", core.ReleaseInput{Ticket: "3"}); err != nil || v.State != store.Ready {
		t.Fatalf("release from Answered: %+v, %v", v.State, err)
	}
}
