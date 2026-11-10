package storetest

import (
	"testing"

	"github.com/tomoram/code-central/store"
)

// runGitHubIssues checks a Project's GitHub Issues settings: a Project
// stored without them reads as GitHub Issues off.
func runGitHubIssues(t *testing.T, f Factory) {
	t.Run("a project stored without github issues settings reads as github issues off", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		check := func(s store.Store) {
			read(t, s, func(r store.Reader) {
				if p, _ := r.Project("acme"); p.GitHubIssues != (store.GitHubIssues{}) {
					t.Fatalf("github issues = %+v, want off with nothing set", p.GitHubIssues)
				}
			})
		}
		check(s)
		if f.Reopen != nil {
			check(f.Reopen(t, s))
		}
	})

	t.Run("a project's github issues settings read back after they are written", func(t *testing.T) {
		s := f.New(t)
		want := store.GitHubIssues{On: true, Repository: "acme/web", Label: "good first issue", NewIssuesNeedMe: true, Spec: 4}
		put(t, s, func(w store.Writer) error {
			return w.PutProject(store.Project{Key: "acme", DisplayName: "acme", Created: t0, Mode: "committed", GitHubIssues: want})
		})
		check := func(s store.Store) {
			read(t, s, func(r store.Reader) {
				if p, _ := r.Project("acme"); p.GitHubIssues != want {
					t.Fatalf("github issues = %+v, want %+v", p.GitHubIssues, want)
				}
			})
		}
		check(s)
		if f.Reopen != nil {
			check(f.Reopen(t, s))
		}
	})
}
