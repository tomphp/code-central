package store_test

import (
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 32 (code-central-532) added the Focus Pop-up Option. The real
// chain migrates a format-31 store, whose options.yaml has none, copying
// every file unchanged; Focus Pop-up reads as off.
func TestTheRealChainMigratesAFormat31StoreToTheFocusPopupFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 31\n")
	writeFile(t, filepath.Join(dir, "options.yaml"), "focus_clear: all_clear\n")

	s := openFile(t, dir)
	if m := s.Migrated(); m == nil || m.From != 31 || m.To != store.Format {
		t.Fatalf("migrated = %+v, want a migration from format 31", m)
	}
	s.View(func(r store.Reader) error {
		want := store.Options{FocusClear: store.FocusAllClear, FocusPopup: store.FocusPopupOff}
		if got := r.Options(); got != want {
			t.Errorf("options after migrating = %+v, want %+v", got, want)
		}
		return nil
	})
}
