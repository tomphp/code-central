package storetest

import (
	"github.com/tomoram/code-central/domain"
	"testing"
	"time"

	"github.com/tomoram/code-central/store"
)

// runCI checks a Project's CI settings: a Project stored without them
// reads as CI Tracking off, with nothing named.
func runCI(t *testing.T, f Factory) {
	t.Run("a project stored without ci settings reads as ci tracking off", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		check := func(s store.Store) {
			read(t, s, func(r store.Reader) {
				p, _ := r.Project("acme")
				if want := (domain.CI{}); !p.CI.Equal(want) {
					t.Fatalf("ci = %+v, want %+v", p.CI, want)
				}
			})
		}
		check(s)
		if f.Reopen != nil {
			check(f.Reopen(t, s))
		}
	})

	t.Run("a project's ci settings read back after they are written", func(t *testing.T) {
		s := f.New(t)
		want := domain.CI{Tracking: true, Repositories: []domain.CIRepository{{Name: "acme", Provider: domain.CIGitHubActions, Repository: "acme/web"}},
			IgnoredWorkflows: []string{"nightly.yml", "preview.yaml"}, PushRemoteURL: "git@github.com:acme/web.git",
			Rules: []domain.CIRule{
				{Spec: 4, Workflow: "ci.yml", Branch: "main", State: domain.Ready, Created: t0},
				{Spec: 7, Workflow: domain.AnyWorkflow, Branch: "release/2.x", State: domain.Draft, Created: t0.Add(time.Hour)},
			}}
		put(t, s, func(w store.Writer) error {
			return w.PutProject(domain.Project{Key: "acme", DisplayName: "acme", Created: t0, Mode: "committed", CI: want})
		})
		check := func(s store.Store) {
			read(t, s, func(r store.Reader) {
				if p, _ := r.Project("acme"); !p.CI.Equal(want) {
					t.Fatalf("ci = %+v, want %+v", p.CI, want)
				}
			})
		}
		check(s)
		if f.Reopen != nil {
			check(f.Reopen(t, s))
		}
	})

	t.Run("a home reported with no push remote reads back so, not watched", func(t *testing.T) {
		s := f.New(t)
		want := domain.CI{NoPushRemote: true, Repositories: []domain.CIRepository{{Name: "acme", NotWatched: true}}}
		put(t, s, func(w store.Writer) error {
			return w.PutProject(domain.Project{Key: "acme", DisplayName: "acme", Created: t0, Mode: "committed", CI: want})
		})
		check := func(s store.Store) {
			read(t, s, func(r store.Reader) {
				if p, _ := r.Project("acme"); !p.CI.Equal(want) {
					t.Fatalf("ci = %+v, want %+v", p.CI, want)
				}
			})
		}
		check(s)
		if f.Reopen != nil {
			check(f.Reopen(t, s))
		}
	})
}
