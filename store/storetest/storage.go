package storetest

import (
	"testing"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// The Storage tab's figures: the Store's size, and each Project's count
// of Specs beside its State counts.
func runStorage(t *testing.T, f Factory) {
	t.Run("spec counts count each project's specs", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		put(t, s, func(w store.Writer) error {
			return w.PutProject(domain.Project{Key: "beta", DisplayName: "beta", Created: t0, Mode: "committed"})
		})
		read(t, s, func(r store.Reader) {
			if got := r.SpecCount("acme"); got != 0 {
				t.Errorf("a project with no Specs counts %d", got)
			}
		})
		for _, sp := range []struct {
			project string
			n       int
		}{{"acme", 1}, {"acme", 2}, {"beta", 3}, {"acme", 4}} {
			put(t, s, func(w store.Writer) error {
				return w.PutSpec(sp.project, domain.Spec{Number: sp.n, Title: "S", Created: t0, Updated: t0})
			})
		}
		// Writing a Spec again counts it once.
		put(t, s, func(w store.Writer) error {
			return w.PutSpec("acme", domain.Spec{Number: 2, Title: "S again", Created: t0, Updated: t0})
		})
		read(t, s, func(r store.Reader) {
			for project, want := range map[string]int{"acme": 3, "beta": 1, "other": 0} {
				if got := r.SpecCount(project); got != want {
					t.Errorf("%s counts %d Specs, want %d", project, got, want)
				}
			}
		})
	})

	t.Run("a store with records has a size", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		put(t, s, func(w store.Writer) error {
			return w.PutSpec("acme", domain.Spec{Number: 1, Title: "S", Body: "A body.", Created: t0, Updated: t0})
		})
		put(t, s, func(w store.Writer) error {
			return w.PutTicket("acme", domain.Ticket{Number: 2, Title: "T", State: domain.Ready, Spec: 1, Created: t0, Updated: t0})
		})
		size, err := s.Size()
		if err != nil {
			t.Fatalf("size: %v", err)
		}
		if size <= 0 {
			t.Errorf("size %d, want above zero", size)
		}
	})
}
