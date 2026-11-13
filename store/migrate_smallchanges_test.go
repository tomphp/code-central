package store_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 31 (code-central-479) added a Project's Small Changes Spec. The real chain migrates a format-30 store, which
// has none, copying every file unchanged.
func TestTheRealChainMigratesAFormat30StoreToTheSmallChangesFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 30\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\nci_responses:\n  - commit: abc123\n    land_anyway: 2026-01-02T03:05:05Z\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	bak := filepath.Join(filepath.Dir(dir), "code-central.format-30.bak")
	if m := s.Migrated(); m == nil || *m != (store.Migration{From: 30, To: store.Format, Backup: bak}) {
		t.Fatalf("migrated = %+v, want a migration from format 30 with its backup", m)
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
		if p.SmallChangesSpec != 0 {
			t.Errorf("project after migrating = %+v, want no Small Changes Spec", p)
		}
		return nil
	})
}
