package core_test

import (
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Accepted but not landed, and sending back a conflict (ticket 72).

func TestATicketAcceptedForMoreThanTenMinutesIsAcceptedNotLanded(t *testing.T) {
	f := newFixture(t)
	f.accepted(t) // at t0+2h
	acceptedAt := t0.Add(2 * time.Hour)

	f.clock.Set(acceptedAt.Add(core.NotLandedAfter))
	wantReason(t, f.get(t, "2"), "")

	f.clock.Advance(time.Second)
	wantReason(t, f.get(t, "2"), core.ReasonAcceptedNotLanded)
	row, ok := longRow(t, f, "acme-2")
	if !ok || row.Reason != core.ReasonAcceptedNotLanded || !row.Since.Equal(acceptedAt) {
		t.Fatalf("Needs Me row %+v, want accepted_not_landed since the accept %s", row, acceptedAt)
	}
}

func TestTheWatchSendsTicketChangedWhenAcceptedWorkTurnsNotLanded(t *testing.T) {
	f := newFixture(t)
	f.accepted(t)

	tick := watch(t, f)

	if got := tick(); len(got) != 0 {
		t.Fatalf("got %v, want none before the threshold", got)
	}
	f.clock.Advance(core.NotLandedAfter + time.Second)
	if got := tick(); len(got) != 1 || got[0].id != "acme-2" {
		t.Fatalf("got %v, want one for acme-2 turning Accepted but not landed", got)
	}
	if got := tick(); len(got) != 0 {
		t.Fatalf("got %v, want none while it stays not landed", got)
	}
}

func TestAnAgentSendsAcceptedWorkBackToReadyWithANote(t *testing.T) {
	f := newFixture(t)
	f.accepted(t)
	f.clock.Set(t0.Add(3 * time.Hour))
	note := "Merging acme-2 into main at 9f8e7d conflicts in core/claim.go."
	v, err := record(f.core.SendBack(ctx, "acme", core.SendBackInput{Ticket: "2", Note: note, Full: true}))
	if err != nil {
		t.Fatal(err)
	}
	m := v.Moves[len(v.Moves)-1]
	if v.State != store.Ready || m.From != store.Accepted || m.Actor != "agent" {
		t.Fatalf("state %s, last move %+v", v.State, m)
	}
	if c := v.Comments[len(v.Comments)-1]; c.Body != note || c.Actor != "agent" {
		t.Fatalf("last comment %+v, want the note", c)
	}
	wantReason(t, v, "")
}

func TestSendingBackFromAcceptedNeedsANote(t *testing.T) {
	f := newFixture(t)
	f.accepted(t)
	for _, actor := range []core.Actor{core.Agent, core.Human} {
		_, err := f.core.SendBack(core.WithActor(ctx, actor), "acme", core.SendBackInput{Ticket: "2", Note: " "})
		wantCode(t, err, core.Invalid, "note")
	}
	if v := f.get(t, "2"); v.State != store.Accepted {
		t.Fatalf("state %s, want still Accepted", v.State)
	}
}

func TestTheHumansSendBackFromFailedIsUnchanged(t *testing.T) {
	f := newFixture(t)
	f.failed(t)
	_, err := f.core.SendBack(ctx, "acme", core.SendBackInput{Ticket: "2", Note: "Try again."})
	wantCode(t, err, core.ActionNotAllowed, "")
	v, err := record(f.core.SendBack(human, "acme", core.SendBackInput{Ticket: "2", Full: true}))
	if err != nil || v.State != store.Ready {
		t.Fatalf("state %s, err %v; the human sends back from Failed with no note", v.State, err)
	}
}
