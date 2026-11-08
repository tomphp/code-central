package core_test

import (
	"context"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/store"
)

func wantSilentSince(t *testing.T, v core.TicketView, want time.Time) {
	t.Helper()
	if want.IsZero() {
		if v.SilentSince != nil {
			t.Fatalf("%s silent_since = %v, want not Silent", v.ID, *v.SilentSince)
		}
		return
	}
	if v.SilentSince == nil || !v.SilentSince.Equal(want) {
		t.Fatalf("%s silent_since = %v, want %v", v.ID, v.SilentSince, want)
	}
}

func TestAClaimTurnsSilentAfterThirtyMinutesWithoutActivity(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	claimed := f.clock.Now()
	f.claim(t, core.ClaimInput{SessionID: "s1"})

	f.clock.Advance(30*time.Minute - time.Second)
	wantSilentSince(t, f.get(t, "acme-2"), time.Time{})

	f.clock.Advance(time.Second)
	wantSilentSince(t, f.get(t, "acme-2"), claimed) // quiet since its last activity
}

func TestAnyWriteToTheTicketClearsSilent(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	f.claim(t, core.ClaimInput{})
	f.clock.Advance(45 * time.Minute)

	written := f.clock.Now()
	if _, err := f.core.Comment(ctx, "acme", core.CommentInput{Ticket: "2", Body: "still here"}); err != nil {
		t.Fatal(err)
	}
	wantSilentSince(t, f.get(t, "acme-2"), time.Time{})
	f.clock.Advance(30 * time.Minute)
	wantSilentSince(t, f.get(t, "acme-2"), written)
}

func TestAPingClearsSilentAndIsThrottledToOncePerMinutePerSession(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	f.claim(t, core.ClaimInput{SessionID: "s1"})

	f.clock.Advance(40 * time.Minute)
	pinged := f.clock.Now()
	f.core.Ping(ctx, "s1")
	v := f.get(t, "acme-2")
	wantSilentSince(t, v, time.Time{})
	if !v.Claims[0].LastActivity.Equal(pinged) {
		t.Fatalf("live Claim's last_activity %v, want the ping at %v", v.Claims[0].LastActivity, pinged)
	}

	f.clock.Advance(59 * time.Second)
	f.core.Ping(ctx, "s1") // within 60 seconds of the last one: ignored
	f.clock.Advance(30*time.Minute - 59*time.Second)
	wantSilentSince(t, f.get(t, "acme-2"), pinged)

	pinged = f.clock.Now()
	f.core.Ping(ctx, "s1") // 30 minutes after the last accepted one
	f.clock.Advance(time.Minute)
	f.core.Ping(ctx, "s1") // exactly 60 seconds later: accepted
	f.clock.Advance(30*time.Minute - time.Second)
	wantSilentSince(t, f.get(t, "acme-2"), time.Time{})
	f.clock.Advance(time.Second)
	wantSilentSince(t, f.get(t, "acme-2"), pinged.Add(time.Minute))
}

func TestAPingMarksEveryLiveClaimWithThatSessionInAnyProject(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready, store.Ready) // acme-2, acme-3
	f.project(t, "web")
	f.spec(t, "web", "Site")
	f.ticket(t, "web", core.CreateTicketInput{Spec: "1", Title: "Page", State: store.Ready})
	f.claim(t, core.ClaimInput{Ticket: "2", SessionID: "s1"})
	f.claim(t, core.ClaimInput{Ticket: "3", SessionID: "s2"})
	if _, err := f.core.Claim(ctx, "web", core.ClaimInput{SessionID: "s1"}); err != nil {
		t.Fatal(err)
	}
	claimed := f.clock.Now()

	f.clock.Advance(time.Hour)
	f.core.Ping(ctx, "s1")
	f.core.Ping(ctx, "unknown") // an unknown session is fine
	f.core.Ping(ctx, "")        // so is none

	wantSilentSince(t, f.get(t, "acme-2"), time.Time{})
	wantSilentSince(t, f.get(t, "acme-3"), claimed)
	web, err := f.core.GetTicket(ctx, "web", "2")
	if err != nil {
		t.Fatal(err)
	}
	wantSilentSince(t, web, time.Time{})
}

func TestAClaimWhoseSessionKeepsPostingSessionEventsNeverGoesSilent(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	f.claim(t, core.ClaimInput{SessionID: "s1"})

	for range 12 { // three hours, posting every quarter of an hour
		f.clock.Advance(15 * time.Minute)
		f.core.PostSessionEvents(ctx, "s1", []core.SessionEvent{{Time: f.clock.Now(), Kind: "tool_call", Text: "Bash: go test ./..."}})
		wantSilentSince(t, f.get(t, "acme-2"), time.Time{})
	}
	posted := f.clock.Now()
	f.clock.Advance(30 * time.Minute)
	wantSilentSince(t, f.get(t, "acme-2"), posted) // quiet once the posts stop
}

func TestOnlyInProgressIsEverSilent(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready, store.Ready)
	f.claim(t, core.ClaimInput{Ticket: "2"})
	f.claim(t, core.ClaimInput{Ticket: "3"})
	// Rounds arrive with ticket 29; put the Tickets in Needs Answer and
	// Answered (Claim still live) directly.
	for n, s := range map[int]store.State{2: store.NeedsAnswer, 3: store.Answered} {
		err := f.store.Update(func(w store.Writer) error {
			tk, _ := w.Ticket("acme", n)
			tk.State = s
			return w.PutTicket("acme", tk)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	f.clock.Advance(2 * time.Hour)
	wantSilentSince(t, f.get(t, "acme-2"), time.Time{})
	wantSilentSince(t, f.get(t, "acme-3"), time.Time{})
}

func TestAfterARestartLastActivityCountsFromServiceStart(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	f.claim(t, core.ClaimInput{SessionID: "s1"})
	f.core.Ping(ctx, "s1")

	f.clock.Advance(2 * time.Hour)
	started := f.clock.Now()
	restarted := core.New(f.store, f.clock, &ids.Sequence{})
	f.clock.Advance(29 * time.Minute)
	v, _ := restarted.GetTicket(ctx, "acme", "2")
	wantSilentSince(t, v, time.Time{})
	f.clock.Advance(time.Minute)
	v, _ = restarted.GetTicket(ctx, "acme", "2")
	wantSilentSince(t, v, started)
}

func TestTheWatchSendsTicketChangedWhenAClaimTurnsSilentOrStopsBeingSilent(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready, store.Ready)
	f.claim(t, core.ClaimInput{Ticket: "2", SessionID: "s1"})
	f.clock.Advance(10 * time.Minute)
	f.claim(t, core.ClaimInput{Ticket: "3", SessionID: "s2"})

	ticks := make(chan time.Time)
	type change struct{ project, id string }
	changes := make(chan change, 10)
	watchCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		f.core.WatchTimedReasons(watchCtx, ticks, func(project, id string) { changes <- change{project, id} })
		close(done)
	}()
	defer func() { stop(); <-done }()
	tick := func() []change {
		ticks <- f.clock.Now()
		ticks <- f.clock.Now() // the second send returns once the first tick is handled
		var got []change
		for {
			select {
			case c := <-changes:
				got = append(got, c)
			default:
				return got
			}
		}
	}

	if got := tick(); len(got) != 0 {
		t.Fatalf("nothing is Silent yet, got %v", got)
	}
	f.clock.Advance(20 * time.Minute) // acme-2 quiet for 30 minutes
	if got := tick(); len(got) != 1 || got[0] != (change{"acme", "acme-2"}) {
		t.Fatalf("got %v, want acme-2 turning Silent", got)
	}
	if got := tick(); len(got) != 0 {
		t.Fatalf("once per crossing, got %v", got)
	}
	f.core.Ping(ctx, "s1")
	f.clock.Advance(10 * time.Minute) // acme-3 now Silent too
	got := tick()
	if len(got) != 2 {
		t.Fatalf("got %v, want acme-2 cleared and acme-3 Silent", got)
	}
	f.clock.Advance(30 * time.Minute)
	if got := tick(); len(got) != 1 || got[0] != (change{"acme", "acme-2"}) {
		t.Fatalf("got %v, want acme-2 Silent again", got)
	}
}
