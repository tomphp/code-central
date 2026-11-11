package store_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 20 (code-central-364) added a Project's Azure DevOps project URL
// and PAT environment variable, and the partially succeeded CI
// conclusion. The real chain migrates a format-19 store, whose CI
// settings have neither, copying every file unchanged.
func TestTheRealChainMigratesAFormat19StoreToTheAzurePipelinesFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 19\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\nci:\n  tracking: true\n  provider: github_actions\n  repository: acme/web\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	bak := filepath.Join(filepath.Dir(dir), "code-central.format-19.bak")
	if m := s.Migrated(); m == nil || *m != (store.Migration{From: 19, To: store.Format, Backup: bak}) {
		t.Fatalf("migrated = %+v, want a migration from format 19 with its backup", m)
	}
	after := snapshot(t, dir)
	for rel, content := range before {
		if rel != "store.yaml" && after[rel] != content {
			t.Errorf("%s changed:\n%s", rel, after[rel])
		}
	}
	if got := after["store.yaml"]; got != fmt.Sprintf("format: %d\n", store.Format) {
		t.Errorf("store.yaml = %q", got)
	}
	s.View(func(r store.Reader) error {
		p, _ := r.Project("acme")
		if !p.CI.Tracking || p.CI.Repository != "acme/web" || p.CI.ProjectURL != "" || p.CI.PATEnv != "" {
			t.Errorf("ci after migrating = %+v, want acme/web with no project URL or PAT environment variable", p.CI)
		}
		return nil
	})

	// The new settings and conclusion read back, each provider's own kept.
	if err := s.Update(func(w store.Writer) error {
		p, _ := w.Project("acme")
		p.CI.Provider, p.CI.ProjectURL, p.CI.PATEnv = store.CIAzurePipelines, "https://dev.azure.com/acme/web", "ACME_PAT"
		return w.PutProject(p)
	}); err != nil {
		t.Fatal(err)
	}
	s = reopen(t, s)
	s.View(func(r store.Reader) error {
		p, _ := r.Project("acme")
		if p.CI.Provider != store.CIAzurePipelines || p.CI.ProjectURL != "https://dev.azure.com/acme/web" || p.CI.PATEnv != "ACME_PAT" || p.CI.Repository != "acme/web" {
			t.Errorf("ci = %+v, want Azure Pipelines settings with the repository kept", p.CI)
		}
		return nil
	})
	if !store.CIPartiallySucceeded.Valid() || store.CIPartiallySucceeded.Failed() || store.CIPartiallySucceeded.Cancelled() {
		t.Error("partially_succeeded must be a valid conclusion that is neither failed nor cancelled")
	}
}
