package storetest

import (
	"testing"

	"github.com/tomoram/code-central/store"
)

// runOptions checks the human's global Options: the store's one record
// that belongs to no Project.
func runOptions(t *testing.T, f Factory) {
	t.Run("options read as their defaults before any are written", func(t *testing.T) {
		s := f.New(t)
		read(t, s, func(r store.Reader) {
			if got := r.Options(); got != (store.Options{FocusClear: store.FocusActivityDashboard}) {
				t.Fatalf("options = %+v, want the defaults", got)
			}
		})
	})

	t.Run("options read back after they are written, with no Project", func(t *testing.T) {
		s := f.New(t)
		want := store.Options{FocusClear: store.FocusAllClear, GHPath: "/opt/tools/gh"}
		put(t, s, func(w store.Writer) error { return w.PutOptions(want) })
		check := func(s store.Store) {
			read(t, s, func(r store.Reader) {
				if got := r.Options(); got != want {
					t.Fatalf("options = %+v, want %+v", got, want)
				}
				if n := len(r.Projects()); n != 0 {
					t.Fatalf("writing options made %d Projects", n)
				}
			})
		}
		check(s)
		if f.Reopen != nil {
			check(f.Reopen(t, s))
		}
	})

	t.Run("an options write is the update's one write", func(t *testing.T) {
		s := f.New(t)
		err := s.Update(func(w store.Writer) error {
			if err := w.PutOptions(store.Options{FocusClear: store.FocusAllClear}); err != nil {
				return err
			}
			return w.PutProject(store.Project{Key: "acme", DisplayName: "acme", Created: t0, Mode: "committed"})
		})
		if err == nil {
			t.Fatal("a second write after an options write was allowed")
		}
	})

	t.Run("an option outside its set is refused and changes nothing", func(t *testing.T) {
		s := f.New(t)
		err := s.Update(func(w store.Writer) error { return w.PutOptions(store.Options{FocusClear: "everything"}) })
		if err == nil {
			t.Fatal("an unknown focus_clear was written")
		}
		read(t, s, func(r store.Reader) {
			if got := r.Options(); got.FocusClear != store.FocusActivityDashboard {
				t.Fatalf("options = %+v after a refused write", got)
			}
		})
	})
}
