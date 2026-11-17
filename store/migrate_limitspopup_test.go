package store_test

import (
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 41 (code-central-867) added the Show Limits popup Option. The
// real chain migrates a format-40 store, whose options.yaml has none,
// copying every file unchanged; Show Limits popup reads as on.
func TestTheRealChainMigratesAFormat40StoreToTheLimitsPopupFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 40\n")
	writeFile(t, filepath.Join(dir, "options.yaml"), "focus_clear: all_clear\n")

	s := openFile(t, dir)
	if m := s.Migrated(); m == nil || m.From != 40 || m.To != store.Format {
		t.Fatalf("migrated = %+v, want a migration from format 40", m)
	}
	s.View(func(r store.Reader) error {
		want := store.Options{FocusClear: store.FocusAllClear, FocusPopup: store.FocusPopupOff, LimitsPopup: store.LimitsPopupOn}
		if got := r.Options(); got != want {
			t.Errorf("options after migrating = %+v, want %+v", got, want)
		}
		return nil
	})
}
