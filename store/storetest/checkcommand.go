package storetest

import (
	"slices"
	"testing"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// runCheckCommand checks the Check Command of a Project and of each of
// its Repositories, the Home's included, reads back after it is written.
func runCheckCommand(t *testing.T, f Factory) {
	t.Run("a project's and its repositories' check commands read back after they are written", func(t *testing.T) {
		s := f.New(t)
		repositories := []domain.Repository{
			{Name: "acme", Home: true, CheckCommand: "make typecheck"},
			{Name: "web", CloneURL: "git@github.com:acme/web.git", Base: "main", CheckCommand: "npm run check"},
			{Name: "api", CloneURL: "git@github.com:acme/api.git", Base: "main"},
		}
		put(t, s, func(w store.Writer) error {
			return w.PutProject(domain.Project{Key: "acme", DisplayName: "acme", Created: t0, Mode: "committed", CheckCommand: "make check", Repositories: repositories})
		})
		check := func(s store.Store) {
			read(t, s, func(r store.Reader) {
				p, _ := r.Project("acme")
				if p.CheckCommand != "make check" {
					t.Errorf("Project Check Command = %q, want make check", p.CheckCommand)
				}
				if !slices.Equal(p.Repositories, repositories) {
					t.Errorf("repositories = %+v, want %+v", p.Repositories, repositories)
				}
			})
		}
		check(s)
		if f.Reopen != nil {
			check(f.Reopen(t, s))
		}
	})
}
