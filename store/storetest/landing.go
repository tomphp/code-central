package storetest

import (
	"fmt"
	"github.com/tomoram/code-central/domain"
	"testing"

	"github.com/tomoram/code-central/store"
)

// runLanding checks a Project's Landing Method: a Project stored without
// one reads as Merge, Push off, remote origin.
func runLanding(t *testing.T, f Factory) {
	t.Run("a project stored without a landing method reads as the default", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		check := func(s store.Store) {
			read(t, s, func(r store.Reader) {
				p, _ := r.Project("acme")
				if want := domain.DefaultLanding(); p.Landing != want {
					t.Fatalf("landing = %+v, want %+v", p.Landing, want)
				}
			})
		}
		check(s)
		if f.Reopen != nil {
			check(f.Reopen(t, s))
		}
	})

	t.Run("a project's landing method reads back after it is written", func(t *testing.T) {
		s := f.New(t)
		want := domain.Landing{Method: domain.LandingBranch, Push: true, Remote: "upstream"}
		put(t, s, func(w store.Writer) error {
			return w.PutProject(domain.Project{Key: "acme", DisplayName: "acme", Created: t0, Mode: "committed", Landing: want})
		})
		check := func(s store.Store) {
			read(t, s, func(r store.Reader) {
				if p, _ := r.Project("acme"); p.Landing != want {
					t.Fatalf("landing = %+v, want %+v", p.Landing, want)
				}
			})
		}
		check(s)
		if f.Reopen != nil {
			check(f.Reopen(t, s))
		}
	})

	for _, on := range []bool{true, false} {
		t.Run(fmt.Sprintf("a project's land per spec %v reads back after it is written", on), func(t *testing.T) {
			s := f.New(t)
			want := domain.Landing{Method: domain.LandingMerge, Push: true, Remote: "upstream", LandPerSpec: on}
			put(t, s, func(w store.Writer) error {
				return w.PutProject(domain.Project{Key: "acme", DisplayName: "acme", Created: t0, Mode: "committed", Landing: want})
			})
			check := func(s store.Store) {
				read(t, s, func(r store.Reader) {
					if p, _ := r.Project("acme"); p.Landing != want {
						t.Fatalf("landing = %+v, want %+v", p.Landing, want)
					}
				})
			}
			check(s)
			if f.Reopen != nil {
				check(f.Reopen(t, s))
			}
		})
	}

	// Rebase is kept whatever the method, and the default Landing with it
	// on is still written.
	for _, want := range []domain.Landing{
		{Method: domain.LandingMerge, Remote: domain.DefaultLandingRemote, LandPerSpec: true, Rebase: true},
		{Method: domain.LandingBranch, Remote: "upstream", Rebase: true},
		{Method: domain.LandingPullRequest, Remote: "upstream", LandPerSpec: true, Rebase: true},
	} {
		t.Run(fmt.Sprintf("a project's rebase reads back under %s", want.Method), func(t *testing.T) {
			s := f.New(t)
			put(t, s, func(w store.Writer) error {
				return w.PutProject(domain.Project{Key: "acme", DisplayName: "acme", Created: t0, Mode: "committed", Landing: want})
			})
			check := func(s store.Store) {
				read(t, s, func(r store.Reader) {
					if p, _ := r.Project("acme"); p.Landing != want {
						t.Fatalf("landing = %+v, want %+v", p.Landing, want)
					}
				})
			}
			check(s)
			if f.Reopen != nil {
				check(f.Reopen(t, s))
			}
		})
	}
}
