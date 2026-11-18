// Change notices (Spec code-central-1032): an Update's Announce reaches
// the other services sharing the Store once the Update commits, and
// never the service that made it. A Kind one service holds alone has no
// one to tell, and never delivers.

package storetest

import (
	"errors"
	"testing"
	"time"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// quiet is how long a case waits to be sure a Change isn't coming.
const quiet = 200 * time.Millisecond

func runChanges(t *testing.T, f Factory) {
	ticketChanged := func(n int, wakes bool) store.Change {
		return store.Change{Project: "acme", Type: "ticket.changed", ID: "acme-" + string(rune('0'+n)), Wakes: wakes}
	}
	// announcing writes Ticket n and announces it.
	announcing := func(n int, wakes bool) func(store.Writer) error {
		return func(w store.Writer) error {
			if err := w.PutTicket("acme", domain.Ticket{Number: n, Title: "T", State: domain.Ready, Spec: 1, Created: t0, Updated: t0}); err != nil {
				return err
			}
			w.Announce(ticketChanged(n, wakes))
			return nil
		}
	}

	t.Run("a store never hears its own changes", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		heard := Hear(s)
		put(t, s, announcing(1, true))
		NothingHeard(t, heard)
	})

	if f.Second == nil {
		return
	}

	t.Run("another service hears a change once it commits, and not before", func(t *testing.T) {
		a := f.New(t)
		seedProject(t, a)
		b := f.Second(t, a)
		heard := Hear(b)
		put(t, a, func(w store.Writer) error {
			if err := announcing(1, true)(w); err != nil {
				return err
			}
			NothingHeard(t, heard)
			return nil
		})
		if got, want := NextHeard(t, heard), ticketChanged(1, true); got != want {
			t.Fatalf("heard %+v, want %+v", got, want)
		}
	})

	t.Run("a service hears the others' changes but not its own", func(t *testing.T) {
		a := f.New(t)
		seedProject(t, a)
		b := f.Second(t, a)
		heardByA, heardByB := Hear(a), Hear(b)
		put(t, a, announcing(1, true))
		if got, want := NextHeard(t, heardByB), ticketChanged(1, true); got != want {
			t.Fatalf("b heard %+v, want %+v", got, want)
		}
		put(t, b, announcing(2, false))
		// A Store hears Changes in the order they commit, so a's own would
		// come first.
		if got, want := NextHeard(t, heardByA), ticketChanged(2, false); got != want {
			t.Fatalf("a heard %+v first, want b's %+v", got, want)
		}
	})

	t.Run("another service hears nothing of an update that fails", func(t *testing.T) {
		a := f.New(t)
		seedProject(t, a)
		b := f.Second(t, a)
		heard := Hear(b)
		failed := errors.New("failed after announcing")
		err := a.Update(func(w store.Writer) error {
			if err := announcing(1, true)(w); err != nil {
				return err
			}
			return failed
		})
		if !errors.Is(err, failed) {
			t.Fatalf("update: %v", err)
		}
		NothingHeard(t, heard)
	})
}

// Hear is the Changes s delivers, as they come.
func Hear(s store.Store) <-chan store.Change {
	heard := make(chan store.Change, 64)
	s.Changes(func(c store.Change) { heard <- c })
	return heard
}

// NextHeard is the next Change heard, failing t when none comes within
// a few seconds.
func NextHeard(t *testing.T, heard <-chan store.Change) store.Change {
	t.Helper()
	select {
	case c := <-heard:
		return c
	case <-time.After(10 * time.Second):
		t.Fatal("no Change heard")
		return store.Change{}
	}
}

// NothingHeard fails t if a Change is heard within a moment.
func NothingHeard(t *testing.T, heard <-chan store.Change) {
	t.Helper()
	select {
	case c := <-heard:
		t.Fatalf("heard %+v", c)
	case <-time.After(quiet):
	}
}
