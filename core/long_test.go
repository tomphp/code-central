package core_test

import (
	"context"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// keepActive pings the Claim's session every 20 minutes for d, so it
// never turns Silent.
func (f fixture) keepActive(d time.Duration) {
	for step := 20 * time.Minute; d > 0; d -= step {
		f.clock.Advance(min(step, d))
		f.core.Ping(ctx, "s1")
	}
}

func longRow(t *testing.T, f fixture, id string) (core.NeedsMeRow, bool) {
	t.Helper()
	v, err := f.core.NeedsMe(human)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range v.Groups {
		for _, r := range g.Tickets {
			if r.ID == id {
				return r, true
			}
		}
	}
	return core.NeedsMeRow{}, false
}

func TestAnActiveClaimTurnsLongAfterSixtyMinutesWithoutAMove(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	claimed := f.clock.Now()
	f.claim(t, core.ClaimInput{SessionID: "s1"})

	f.keepActive(core.LongAfter)
	wantReason(t, f.get(t, "2"), "")

	f.clock.Advance(time.Second)
	wantReason(t, f.get(t, "2"), core.ReasonLongClaim)
	row, ok := longRow(t, f, "acme-2")
	if !ok || row.Reason != core.ReasonLongClaim || !row.Since.Equal(claimed) {
		t.Fatalf("Needs Me row %+v, want long_claim since its last move %s", row, claimed)
	}
}

func TestOnlyAMoveRestartsALongClaimsClock(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	f.claim(t, core.ClaimInput{SessionID: "s1"})
	f.keepActive(40 * time.Minute)
	if _, err := f.core.Comment(ctx, "acme", core.CommentInput{Ticket: "2", Body: "still going"}); err != nil {
		t.Fatal(err)
	}
	f.keepActive(20*time.Minute + time.Second)
	wantReason(t, f.get(t, "2"), core.ReasonLongClaim) // a comment is activity, not a move
}

func TestPickingUpAnswersRestartsALongClaimsClock(t *testing.T) {
	f := newFixture(t)
	f.asking(t, qDB) // claimed at t0 by sess-1, Round posted an hour later
	if _, err := f.core.Answer(human, "acme", core.AnswerInput{Ticket: "2", Question: 1, Text: "SQLite"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.PickUpAnswers(ctx, "acme", core.PickUpAnswersInput{Ticket: "2", ClaimID: "claim-1"}); err != nil {
		t.Fatal(err)
	}
	picked := f.clock.Now()
	for range 3 {
		f.clock.Advance(20 * time.Minute)
		f.core.Ping(ctx, "sess-1")
	}
	wantReason(t, f.get(t, "2"), "")
	f.clock.Advance(time.Second)
	wantReason(t, f.get(t, "2"), core.ReasonLongClaim)
	if row, _ := longRow(t, f, "acme-2"); !row.Since.Equal(picked) {
		t.Fatalf("since %s, want the pick-up %s", row.Since, picked)
	}
}

func TestTheWatchSendsTicketChangedWhenAClaimTurnsLongOrSilentTurnsLong(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	f.claim(t, core.ClaimInput{SessionID: "s1"})

	ticks := make(chan time.Time)
	changes := make(chan string, 10)
	watchCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		f.core.WatchSilent(watchCtx, ticks, func(_, id string) { changes <- id })
		close(done)
	}()
	defer func() { stop(); <-done }()
	tick := func() int {
		ticks <- f.clock.Now()
		ticks <- f.clock.Now() // the second send returns once the first tick is handled
		return len(changes)
	}

	f.keepActive(core.LongAfter + time.Second)
	if n := tick(); n != 1 {
		t.Fatalf("%d notices, want one for acme-2 turning Long", n)
	}
	<-changes
	if n := tick(); n != 0 {
		t.Fatalf("%d notices, want none while it stays Long", n)
	}
	f.clock.Advance(core.SilentAfter)
	tick() // Long to Silent
	<-changes
	f.core.Ping(ctx, "s1")
	if n := tick(); n != 1 {
		t.Fatalf("%d notices, want one for acme-2 going from Silent back to Long", n)
	}
}

func TestSilentTakesPrecedenceOverLong(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	f.claim(t, core.ClaimInput{SessionID: "s1"})
	f.clock.Advance(core.LongAfter + time.Minute) // quiet the whole time
	wantReason(t, f.get(t, "2"), core.ReasonSilentClaim)

	f.core.Ping(ctx, "s1") // active again, and still unmoved
	wantReason(t, f.get(t, "2"), core.ReasonLongClaim)
}
