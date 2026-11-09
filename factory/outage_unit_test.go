package factory

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/tomoram/code-central/clock"
)

// pausedWorld is a factory with --parallel 2 on a fake clock, and the
// clock.
func pausedWorld() (*factory, *clock.Fake) {
	clk := clock.NewFake(time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC))
	f := newFactory(Env{Stdout: io.Discard, Now: clk.Now, Backoff: time.Minute}, options{parallel: 2})
	return f, clk
}

func TestAPausedFactoryStartsNoTurnInAFreeSlot(t *testing.T) {
	f, _ := pausedWorld()
	ctx := context.Background()
	if !f.slotFree(ctx) {
		t.Fatal("no slot free in an idle Factory")
	}
	// A turn this poll started was cut off and gave its slot back.
	f.cutOff(claim{Ticket: "acme-2", ClaimID: "c2"}, "overloaded_error")
	if f.slotFree(ctx) {
		t.Error("a paused Factory may start a turn in its free slot; want none started until the outage is over")
	}
}

func TestTheBackoffDoublesWithEachOutageInARowUpToItsCap(t *testing.T) {
	f, clk := pausedWorld()
	c := claim{Ticket: "acme-2", ClaimID: "c2"}
	want := time.Minute
	for range 8 {
		f.cutOff(c, "rate_limit")
		if got := f.pause.backoff; got != want {
			t.Fatalf("backoff %s, want %s", got, want)
		}
		clk.Advance(want - time.Second)
		if due := f.duePausedResumes(1); len(due) != 0 {
			t.Fatalf("resumed %v a second before the %s backoff passed", due, want)
		}
		clk.Advance(time.Second)
		if due := f.duePausedResumes(1); len(due) != 1 || due[0].ClaimID != c.ClaimID {
			t.Fatalf("resumes %v once the %s backoff passed; want %s", due, want, c.Ticket)
		}
		want = min(2*want, maxBackoff)
	}
	if f.pause.backoff != maxBackoff {
		t.Errorf("backoff %s after eight outages in a row, want the %s cap", f.pause.backoff, maxBackoff)
	}
}
