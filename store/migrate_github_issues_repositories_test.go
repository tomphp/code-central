package store_test

import (
	"github.com/tomoram/code-central/domain"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 47 (code-central-858) replaced the GitHub Issues setting
// `repository` with a list, `repositories`. The real chain migrates a
// format-46 store moving each Project's repository into the list, and
// leaves a Project with none, or without GitHub Issues, as it was.
func TestTheRealChainMigratesAFormat46StoreToTheGitHubIssuesRepositoriesFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 46\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n"+
			"github_issues:\n  on: true\n  repository: acme/web\n  label: good first issue\n  new_issues_need_me: true\n  spec: 4\n")
	writeFile(t, filepath.Join(dir, "projects/off/project.yaml"),
		"key: off\ndisplay_name: Off\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n"+
			"github_issues:\n  on: false\n  label: bug\n")
	writeFile(t, filepath.Join(dir, "projects/plain/project.yaml"),
		"key: plain\ndisplay_name: Plain\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	if m := s.Migrated(); m == nil || m.From != 46 || m.To != store.Format {
		t.Fatalf("migrated = %+v, want a migration from format 46", m)
	}
	after := snapshot(t, dir)
	if after["projects/plain/project.yaml"] != before["projects/plain/project.yaml"] {
		t.Errorf("a Project without GitHub Issues changed:\n%s", after["projects/plain/project.yaml"])
	}
	if got := after["projects/acme/project.yaml"]; strings.Contains(got, "  repository:") || !strings.Contains(got, "repositories:") {
		t.Errorf("acme's project.yaml = %s, want repository moved into repositories", got)
	}
	s.View(func(r store.Reader) error {
		p, _ := r.Project("acme")
		want := domain.GitHubIssues{On: true, Repositories: []string{"acme/web"}, Label: "good first issue", NewIssuesNeedMe: true, Spec: 4}
		if !reflect.DeepEqual(p.GitHubIssues, want) {
			t.Errorf("acme's github issues after migrating = %+v, want %+v", p.GitHubIssues, want)
		}
		o, _ := r.Project("off")
		if len(o.GitHubIssues.Repositories) != 0 || o.GitHubIssues.Label != "bug" || o.GitHubIssues.On {
			t.Errorf("off's github issues after migrating = %+v, want it as it was", o.GitHubIssues)
		}
		return nil
	})

	// A list of several reads back in order.
	if err := s.Update(func(w store.Writer) error {
		p, _ := w.Project("acme")
		p.GitHubIssues.Repositories = []string{"acme/web", "acme/api"}
		return w.PutProject(p)
	}); err != nil {
		t.Fatal(err)
	}
	s = reopen(t, s)
	s.View(func(r store.Reader) error {
		if p, _ := r.Project("acme"); !reflect.DeepEqual(p.GitHubIssues.Repositories, []string{"acme/web", "acme/api"}) {
			t.Errorf("repositories read back = %v", p.GitHubIssues.Repositories)
		}
		return nil
	})
}
