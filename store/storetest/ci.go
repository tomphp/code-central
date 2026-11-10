package storetest

import (
	"testing"
	"time"

	"github.com/tomoram/code-central/store"
)

// runCI checks a Project's CI settings: a Project stored without them
// reads as CI Tracking off, on GitHub Actions.
func runCI(t *testing.T, f Factory) {
	t.Run("a project stored without ci settings reads as ci tracking off", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		check := func(s store.Store) {
			read(t, s, func(r store.Reader) {
				p, _ := r.Project("acme")
				if want := (store.CI{Provider: store.CIGitHubActions}); !p.CI.Equal(want) {
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
		want := store.CI{Tracking: true, Provider: store.CIGitHubActions, Repository: "acme/web",
			IgnoredWorkflows: []string{"nightly.yml", "preview.yaml"}, PushRemoteURL: "git@github.com:acme/web.git", UnblockOnPush: true,
			Rules: []store.CIRule{
				{Spec: 4, Workflow: "ci.yml", Branch: "main", State: store.Ready, Created: t0},
				{Spec: 7, Workflow: store.AnyWorkflow, Branch: "release/2.x", State: store.Draft, Created: t0.Add(time.Hour)},
			}}
		put(t, s, func(w store.Writer) error {
			return w.PutProject(store.Project{Key: "acme", DisplayName: "acme", Created: t0, Mode: "committed", CI: want})
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
