package store_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/store"
)

// Format 44 (code-central-857) added each Repository's CI entry and the
// Repository Name of CI Runs and CI responses. The real chain migrates a
// format-43 store copying every file unchanged: a Project from before has
// no entries, and its runs and responses name no Repository, so all read
// as the Home Repository's.
func TestTheRealChainMigratesAFormat43StoreToTheCIRepositoriesFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 43\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n"+
			"ci:\n  tracking: true\n  provider: github_actions\n  repository: acme/web\n"+
			"ci_responses:\n  - commit: abc123\n    land_anyway: 2026-01-02T03:04:05Z\n")
	writeFile(t, filepath.Join(dir, "projects/acme/ci-runs.yaml"),
		"runs:\n  - id: 7\n    attempt: 1\n    workflow: ci.yml\n    name: CI\n    branch: main\n    commit: abc123\n    status: queued\n    url: https://example.com/7\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	if m := s.Migrated(); m == nil || m.From != 43 || m.To != store.Format {
		t.Fatalf("migrated = %+v, want a migration from format 43", m)
	}
	after := snapshot(t, dir)
	for rel, content := range before {
		if rel != "store.yaml" && after[rel] != content {
			t.Errorf("%s changed:\n%s", rel, after[rel])
		}
	}
	s.View(func(r store.Reader) error {
		p, _ := r.Project("acme")
		if p.CI.Repositories != nil || p.CIResponses[0].Repository != "" {
			t.Errorf("CI entries %+v and response %+v, want no entries and no Repository Name", p.CI.Repositories, p.CIResponses[0])
		}
		if runs := r.CIRuns("acme"); runs[0].Repository != "" {
			t.Errorf("run %+v, want no Repository Name", runs[0])
		}
		return nil
	})

	// CI entries and Repository Names written after migrating read back
	// as written.
	at := time.Date(2026, 1, 3, 4, 5, 6, 0, time.UTC)
	entries := []store.CIRepository{
		{Name: "api", Provider: store.CIGitHubActions, Repository: "acme/api"},
		{Name: "docs", NotWatched: true},
		{Name: "infra", Provider: store.CIAzurePipelines, ProjectURL: "https://dev.azure.com/acme/infra"},
	}
	if err := s.Update(func(w store.Writer) error {
		p, _ := w.Project("acme")
		p.CI.Repositories = entries
		p.CIResponses = append(p.CIResponses, store.CIResponse{Repository: "api", Commit: "fed789", Rerun: at})
		return w.PutProject(p)
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(func(w store.Writer) error {
		runs := w.CIRuns("acme")
		runs = append(runs, store.CIRun{Repository: "api", ID: 7, Attempt: 1, Workflow: "ci.yml", Name: "CI", Commit: "fed789", Status: store.CIQueued, URL: "https://example.com/api/7"})
		return w.PutCIRuns("acme", runs)
	}); err != nil {
		t.Fatal(err)
	}
	s = reopen(t, s)
	s.View(func(r store.Reader) error {
		p, _ := r.Project("acme")
		if !reflect.DeepEqual(p.CI.Repositories, entries) {
			t.Errorf("CI entries %+v, want %+v", p.CI.Repositories, entries)
		}
		if p.CIResponses[1].Repository != "api" {
			t.Errorf("response %+v, want it in api", p.CIResponses[1])
		}
		if runs := r.CIRuns("acme"); len(runs) != 2 || runs[1].Repository != "api" {
			t.Errorf("runs %+v, want the second in api", runs)
		}
		return nil
	})
}

// A CI entry naming a Repository twice, by a name that breaks the
// Repository Name rules, or with no CI Provider while watched, doesn't
// load.
func TestCIEntriesAreCheckedOnLoad(t *testing.T) {
	for name, list := range map[string]string{
		"twice":        "    - name: api\n      provider: github_actions\n    - name: api\n      not_watched: true\n",
		"bad name":     "    - name: Api!\n      provider: github_actions\n",
		"no provider":  "    - name: api\n      repository: acme/api\n",
		"bad provider": "    - name: api\n      provider: jenkins\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "code-central")
			writeFile(t, filepath.Join(dir, "store.yaml"), "format: 44\n")
			writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
				"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n"+
					"ci:\n  tracking: false\n  provider: github_actions\n  repositories:\n"+list)
			if _, err := store.Open(dir); err == nil || !strings.Contains(err.Error(), "ci.repositories[") {
				t.Fatalf("open: %v, want a load error naming the ci.repositories field", err)
			}
		})
	}
}
