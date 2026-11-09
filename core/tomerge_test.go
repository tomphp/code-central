package core_test

import (
	"context"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/store"
)

// To merge: an Accepted Ticket under the Branch Landing Method waits on
// the human at once (code-central-175).

func (f fixture) setLanding(t *testing.T, m store.LandingMethod) {
	t.Helper()
	if _, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: m}); err != nil {
		t.Fatalf("set_landing_method %s: %v", m, err)
	}
}

func TestAnAcceptedTicketUnderBranchIsToMergeAtOnce(t *testing.T) {
	f := newFixture(t)
	f.accepted(t) // at t0+2h
	f.setLanding(t, store.LandingBranch)
	acceptedAt := t0.Add(2 * time.Hour)

	wantReason(t, f.get(t, "2"), core.ReasonToMerge)
	row, ok := longRow(t, f, "acme-2")
	if !ok || row.Reason != core.ReasonToMerge || !row.Since.Equal(acceptedAt) || row.Branch != "acme-2" {
		t.Fatalf("Needs Me row %+v, want to_merge on branch acme-2 since the accept %s", row, acceptedAt)
	}

	f.clock.Advance(core.NotLandedAfter + time.Hour)
	wantReason(t, f.get(t, "2"), core.ReasonToMerge)
}

func TestToMergeComesStraightAfterInReview(t *testing.T) {
	f := newFixture(t)
	f.accepted(t)
	f.setLanding(t, store.LandingBranch)
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Tc", State: store.Ready}) // acme-3
	if _, err := f.core.Move(human, "acme", core.MoveInput{Ticket: "3", ToState: store.InReview}); err != nil {
		t.Fatalf("move: %v", err)
	}
	v, err := f.core.NeedsMe(human)
	if err != nil {
		t.Fatal(err)
	}
	var reasons []core.NeedsMeReason
	for _, g := range v.Groups {
		reasons = append(reasons, g.Reason)
	}
	if len(reasons) != 2 || reasons[0] != core.ReasonInReview || reasons[1] != core.ReasonToMerge {
		t.Fatalf("groups %v, want in_review then to_merge", reasons)
	}
}

func TestSwitchingTheLandingMethodSwitchesTheReason(t *testing.T) {
	f := newFixture(t)
	f.accepted(t)
	f.clock.Advance(core.NotLandedAfter + time.Second)
	wantReason(t, f.get(t, "2"), core.ReasonAcceptedNotLanded)

	f.setLanding(t, store.LandingBranch)
	wantReason(t, f.get(t, "2"), core.ReasonToMerge)

	f.setLanding(t, store.LandingMerge)
	wantReason(t, f.get(t, "2"), core.ReasonAcceptedNotLanded)
}

func TestSwitchingTheLandingMethodSendsANoticeForEachAcceptedTicket(t *testing.T) {
	f := newFixture(t)
	f.accepted(t)
	notices, stop := f.core.Events().Subscribe()
	defer stop()

	f.setLanding(t, store.LandingBranch)
	got := map[events.Event]bool{}
	for len(notices) > 0 {
		got[<-notices] = true
	}
	if !got[events.Event{Type: events.TicketChanged, Project: "acme", ID: "acme-2"}] {
		t.Fatalf("notices %v, want ticket.changed for the Accepted acme-2", got)
	}
	if !got[events.Event{Type: events.ProjectChanged, Project: "acme"}] {
		t.Fatalf("notices %v, want project.changed too", got)
	}

	f.setLanding(t, store.LandingBranch) // the same method: nothing changes
	if n := len(notices); n != 0 {
		t.Fatalf("%d notices for setting the same method, want none", n)
	}
}

func TestTheWatchNeverRaisesAcceptedNotLandedUnderBranch(t *testing.T) {
	f := newFixture(t)
	f.accepted(t)
	f.setLanding(t, store.LandingBranch)

	ticks := make(chan time.Time)
	changes := make(chan string, 10)
	watchCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		f.core.WatchTimedReasons(watchCtx, ticks, func(_, id string) { changes <- id })
		close(done)
	}()
	defer func() { stop(); <-done }()
	tick := func() int {
		ticks <- f.clock.Now()
		ticks <- f.clock.Now() // the second send returns once the first tick is handled
		return len(changes)
	}

	tick()
	f.clock.Advance(core.NotLandedAfter + time.Second)
	if n := tick(); n != 0 {
		t.Fatalf("%d notices, want none: a Branch Project's Accepted work is never not landed", n)
	}
	wantReason(t, f.get(t, "2"), core.ReasonToMerge)
}
