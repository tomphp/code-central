package store_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 33 (code-central-555) added a Project's Focus Kinds. The real
// chain migrates a format-32 store copying every file unchanged; a Project
// from before reads every Needs Me reason as on.
func TestTheRealChainMigratesAFormat32StoreToTheFocusKindsFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 32\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	bak := filepath.Join(filepath.Dir(dir), "code-central.format-32.bak")
	if m := s.Migrated(); m == nil || *m != (store.Migration{From: 32, To: store.Format, Backup: bak}) {
		t.Fatalf("migrated = %+v, want a migration from format 32 with its backup", m)
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
		if len(p.FocusKindsOff) != 0 {
			t.Errorf("focus kinds off = %v, want every reason on", p.FocusKindsOff)
		}
		return nil
	})
}
