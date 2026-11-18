package store_test

import (
	"github.com/tomoram/code-central/domain"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 49 (code-central-905) added a Project's Azure Boards settings.
// The real chain migrates a format-48 store copying every file unchanged:
// each Project reads as Azure Boards off with nothing set, and its GitHub
// Issues settings are as they were. Azure Boards' settings then read back,
// those of the mode not chosen kept.
func TestTheRealChainMigratesAFormat48StoreToTheAzureBoardsFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 48\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n"+
			"github_issues:\n  on: true\n  repositories:\n    - acme/web\n  spec: 4\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	if m := s.Migrated(); m == nil || m.From != 48 || m.To != store.Format {
		t.Fatalf("migrated = %+v, want a migration from format 48", m)
	}
	after := snapshot(t, dir)
	for rel, content := range before {
		if rel != "store.yaml" && after[rel] != content {
			t.Errorf("%s changed:\n%s", rel, after[rel])
		}
	}
	s.View(func(r store.Reader) error {
		p, _ := r.Project("acme")
		if !reflect.DeepEqual(p.AzureBoards, domain.AzureBoards{}) {
			t.Errorf("azure boards after migrating = %+v, want off with nothing set", p.AzureBoards)
		}
		if want := (domain.GitHubIssues{On: true, Repositories: []string{"acme/web"}, Spec: 4}); !reflect.DeepEqual(p.GitHubIssues, want) {
			t.Errorf("github issues after migrating = %+v, want %+v", p.GitHubIssues, want)
		}
		return nil
	})

	want := domain.AzureBoards{On: true, Mode: domain.AzureBoardsWIQL, QueryURL: "https://dev.azure.com/contoso/web/_queries/query/6b4a0c1e-9f1d-4a51-8f43-2a7d3c1b9e10",
		ProjectURL: "https://dev.azure.com/contoso/web", Where: "[System.State] = 'Approved'\nAND [System.AssignedTo] = @Me", PATEnv: "BOARDS_PAT", NewIssuesNeedMe: true, Spec: 5}
	if err := s.Update(func(w store.Writer) error {
		p, _ := w.Project("acme")
		p.AzureBoards = want
		return w.PutProject(p)
	}); err != nil {
		t.Fatal(err)
	}
	s = reopen(t, s)
	s.View(func(r store.Reader) error {
		if p, _ := r.Project("acme"); !reflect.DeepEqual(p.AzureBoards, want) {
			t.Errorf("azure boards read back = %+v, want %+v", p.AzureBoards, want)
		}
		return nil
	})
}

// A project.yaml whose Azure Boards mode is neither saved_query nor wiql
// doesn't load, naming the field.
func TestAnUnknownAzureBoardsModeDoesNotLoad(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 49\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n"+
			"azure_boards:\n  on: true\n  mode: query\n")
	if _, err := store.Open(dir); err == nil || !strings.Contains(err.Error(), "azure_boards.mode") {
		t.Fatalf("open = %v, want a load error naming azure_boards.mode", err)
	}
}
