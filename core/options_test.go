package core_test

import (
	"strings"
	"testing"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/store"
)

// countingStore counts the writes that reach the store.
type countingStore struct {
	store.Store
	writes *int
}

func (s countingStore) Update(fn func(store.Writer) error) error {
	return s.Store.Update(func(w store.Writer) error { return fn(countingWriter{w, s.writes}) })
}

type countingWriter struct {
	store.Writer
	writes *int
}

func (w countingWriter) PutOptions(o store.Options) error {
	*w.writes++
	return w.Writer.PutOptions(o)
}

// countingCore is a Core over a memory store that counts its Options
// writes, with a subscription to its notices.
func countingCore(t *testing.T) (*core.Core, *int, <-chan events.Event) {
	t.Helper()
	writes := new(int)
	c := core.New(countingStore{store.NewMemory(), writes}, clock.NewFake(t0), &ids.Sequence{})
	notices, stop := c.Events().Subscribe()
	t.Cleanup(stop)
	return c, writes, notices
}

func focus(f store.FocusClear) *store.FocusClear { return &f }

// noNotice fails if a notice is waiting.
func noNotice(t *testing.T, notices <-chan events.Event) {
	t.Helper()
	select {
	case e := <-notices:
		t.Fatalf("unexpected notice %+v", e)
	default:
	}
}

func TestGetOptionsIsEveryActorsAndStartsAtTheDefaults(t *testing.T) {
	f := newFixture(t)
	for _, a := range []core.Actor{core.Human, core.Agent, core.Integration} {
		o, err := f.core.GetOptions(core.WithActor(ctx, a))
		if err != nil || o.FocusClear != store.FocusAllClear {
			t.Fatalf("get_options as %s: %+v, %v", a, o, err)
		}
	}
}

func TestSetOptionsIsTheHumans(t *testing.T) {
	f := newFixture(t)
	for _, a := range []core.Actor{core.Agent, core.Integration} {
		_, err := f.core.SetOptions(core.WithActor(ctx, a), core.SetOptionsInput{FocusClear: focus(store.FocusActivityDashboard)})
		wantCode(t, err, core.ActionNotAllowed, "")
	}
	if o, _ := f.core.GetOptions(ctx); o.FocusClear != store.FocusAllClear {
		t.Fatalf("a refused set_options changed the Options: %+v", o)
	}
}

func TestSetOptionsChangesAndPublishesOneNotice(t *testing.T) {
	c, writes, notices := countingCore(t)

	o, err := c.SetOptions(human, core.SetOptionsInput{FocusClear: focus(store.FocusActivityDashboard)})
	if err != nil || o.FocusClear != store.FocusActivityDashboard {
		t.Fatalf("set_options: %+v, %v", o, err)
	}
	if got := <-notices; got != (events.Event{Type: events.OptionsChanged}) {
		t.Fatalf("got %+v, want a bare options.changed", got)
	}
	noNotice(t, notices)
	if *writes != 1 {
		t.Fatalf("%d writes, want 1", *writes)
	}
	if got, _ := c.GetOptions(ctx); got != o {
		t.Fatalf("get_options after set: %+v, want %+v", got, o)
	}

	// The same value again, or nothing at all, changes nothing.
	for _, in := range []core.SetOptionsInput{{FocusClear: focus(store.FocusActivityDashboard)}, {}} {
		if got, err := c.SetOptions(human, in); err != nil || got != o {
			t.Fatalf("a no-op set_options: %+v, %v", got, err)
		}
	}
	noNotice(t, notices)
	if *writes != 1 {
		t.Fatalf("a no-op set_options wrote: %d writes", *writes)
	}
}

func TestSetOptionsValidatesBeforeWriting(t *testing.T) {
	c, writes, notices := countingCore(t)

	_, err := c.SetOptions(human, core.SetOptionsInput{FocusClear: focus("fireworks")})
	wantCode(t, err, core.Invalid, "focus_clear")
	for _, want := range []string{"focus_clear", "all_clear", "activity_dashboard"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal should name %q: %v", want, err)
		}
	}
	noNotice(t, notices)
	if *writes != 0 {
		t.Fatalf("a refused set_options wrote: %d writes", *writes)
	}
}

func TestSetOptionsRecordsNothingOnAnyTicket(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	before, _ := f.core.GetTicket(ctx, "acme", "2")
	if _, err := f.core.SetOptions(human, core.SetOptionsInput{FocusClear: focus(store.FocusActivityDashboard)}); err != nil {
		t.Fatal(err)
	}
	after, _ := f.core.GetTicket(ctx, "acme", "2")
	if len(after.Moves) != len(before.Moves) || len(after.Edits) != len(before.Edits) || len(after.Comments) != len(before.Comments) {
		t.Fatalf("set_options touched a Ticket: %+v", after)
	}
}
