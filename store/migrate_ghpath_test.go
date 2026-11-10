package store_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 10 (code-central-252) added the gh path Option. The real chain
// migrates a format-9 store, whose options.yaml has none, copying every
// file unchanged; gh path reads as empty, so gh is found on PATH.
func TestTheRealChainMigratesAFormat9StoreToTheGHPathFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 9\n")
	writeFile(t, filepath.Join(dir, "options.yaml"), "focus_clear: all_clear\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	bak := filepath.Join(filepath.Dir(dir), "code-central.format-9.bak")
	if m := s.Migrated(); m == nil || *m != (store.Migration{From: 9, To: store.Format, Backup: bak}) {
		t.Fatalf("migrated = %+v, want a migration from format 9 with its backup", m)
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
		if got, want := r.Options(), (store.Options{FocusClear: store.FocusAllClear}); got != want {
			t.Errorf("options after migrating = %+v, want %+v", got, want)
		}
		return nil
	})
}
