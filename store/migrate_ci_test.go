package store_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 8 (code-central-251) added a Project's CI settings. The real
// chain migrates a format-7 store, whose project.yaml has none, copying
// every file unchanged; the Project reads as CI Tracking off.
func TestTheRealChainMigratesAFormat7StoreToTheCIFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 7\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\nlanding:\n  method: merge\n  push: true\n  remote: origin\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	bak := filepath.Join(filepath.Dir(dir), "code-central.format-7.bak")
	if m := s.Migrated(); m == nil || *m != (store.Migration{From: 7, To: store.Format, Backup: bak}) {
		t.Fatalf("migrated = %+v, want a migration from format 7 with its backup", m)
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
		if !p.CI.Equal(store.DefaultCI()) {
			t.Errorf("ci after migrating = %+v, want %+v", p.CI, store.DefaultCI())
		}
		return nil
	})
}
