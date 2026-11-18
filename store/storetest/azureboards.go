package storetest

import (
	"github.com/tomoram/code-central/domain"
	"testing"

	"github.com/tomoram/code-central/store"
)

// runAzureBoards checks a Project's Azure Boards settings: a Project
// stored without them reads as Azure Boards off, and every setting reads
// back, those of the Mode not chosen included.
func runAzureBoards(t *testing.T, f Factory) {
	t.Run("a project stored without azure boards settings reads as azure boards off", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		check := func(s store.Store) {
			read(t, s, func(r store.Reader) {
				if p, _ := r.Project("acme"); p.AzureBoards != (domain.AzureBoards{}) {
					t.Fatalf("azure boards = %+v, want off with nothing set", p.AzureBoards)
				}
			})
		}
		check(s)
		if f.Reopen != nil {
			check(f.Reopen(t, s))
		}
	})

	t.Run("a project's azure boards settings read back after they are written", func(t *testing.T) {
		s := f.New(t)
		want := domain.AzureBoards{On: true, Mode: domain.AzureBoardsSavedQuery, QueryURL: "https://dev.azure.com/contoso/web/_queries/query/6b4a0c1e-9f1d-4a51-8f43-2a7d3c1b9e10",
			ProjectURL: "https://dev.azure.com/contoso/api", Where: "[System.State] = 'Approved'", PATEnv: "BOARDS_PAT", NewIssuesNeedMe: true, Spec: 4}
		put(t, s, func(w store.Writer) error {
			return w.PutProject(domain.Project{Key: "acme", DisplayName: "acme", Created: t0, Mode: "committed", AzureBoards: want})
		})
		check := func(s store.Store) {
			read(t, s, func(r store.Reader) {
				if p, _ := r.Project("acme"); p.AzureBoards != want {
					t.Fatalf("azure boards = %+v, want %+v", p.AzureBoards, want)
				}
			})
		}
		check(s)
		if f.Reopen != nil {
			check(f.Reopen(t, s))
		}
	})
}
