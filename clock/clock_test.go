package clock_test

import (
	"testing"
	"time"

	"github.com/tomoram/code-central/clock"
)

var start = time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC)

func TestNextIsTheEarliestWaitingTimersDeadline(t *testing.T) {
	f := clock.NewFake(start)
	if at, ok := f.Next(); ok {
		t.Fatalf("Next %v with no timer waiting, want none", at)
	}
	f.After(3 * time.Second)
	f.After(time.Second)
	f.After(2 * time.Second)
	if at, ok := f.Next(); !ok || !at.Equal(start.Add(time.Second)) {
		t.Fatalf("Next %v, %v, want %v", at, ok, start.Add(time.Second))
	}

	f.Advance(time.Second)
	if at, ok := f.Next(); !ok || !at.Equal(start.Add(2*time.Second)) {
		t.Fatalf("Next %v, %v once the first fired, want %v", at, ok, start.Add(2*time.Second))
	}
	f.Advance(time.Hour)
	if at, ok := f.Next(); ok {
		t.Fatalf("Next %v once every timer fired, want none", at)
	}
}

// One move past several deadlines fires them earliest first, each with
// the time it fell due, as time.After would have, whatever order they
// were made in.
func TestAMovePastSeveralTimersFiresThemInDeadlineOrder(t *testing.T) {
	for name, move := range map[string]func(*clock.Fake){
		"Advance": func(f *clock.Fake) { f.Advance(time.Hour) },
		"Set":     func(f *clock.Fake) { f.Set(start.Add(time.Hour)) },
	} {
		t.Run(name, func(t *testing.T) {
			f := clock.NewFake(start)
			late, early, middle := f.After(3*time.Second), f.After(time.Second), f.After(2*time.Second)
			notYet := f.After(2 * time.Hour)
			move(f)
			for i, c := range []struct {
				ch   <-chan time.Time
				want time.Duration
			}{{early, time.Second}, {middle, 2 * time.Second}, {late, 3 * time.Second}} {
				select {
				case got := <-c.ch:
					if !got.Equal(start.Add(c.want)) {
						t.Errorf("timer %d fired at %v, want %v", i, got, start.Add(c.want))
					}
				default:
					t.Errorf("timer %d didn't fire", i)
				}
			}
			select {
			case got := <-notYet:
				t.Errorf("a timer due after the move fired at %v", got)
			default:
			}
			if now := f.Now(); !now.Equal(start.Add(time.Hour)) {
				t.Errorf("Now %v after the move, want %v", now, start.Add(time.Hour))
			}
		})
	}
}
