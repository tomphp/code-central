package store_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 9 (code-central-254) added a Project's CI Rules. The real chain
// migrates a format-8 store, whose ci mapping has none, copying every
// file unchanged; the Project reads with no CI Rules.
func TestTheRealChainMigratesAFormat8StoreToTheCIRulesFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 8\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\nci:\n  tracking: true\n  provider: github_actions\n  repository: acme/web\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	bak := filepath.Join(filepath.Dir(dir), "code-central.format-8.bak")
	if m := s.Migrated(); m == nil || *m != (store.Migration{From: 8, To: store.Format, Backup: bak}) {
		t.Fatalf("migrated = %+v, want a migration from format 8 with its backup", m)
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
		if want := (store.CI{Tracking: true, Provider: store.CIGitHubActions, Repository: "acme/web"}); !p.CI.Equal(want) {
			t.Errorf("ci after migrating = %+v, want %+v", p.CI, want)
		}
		return nil
	})
}
