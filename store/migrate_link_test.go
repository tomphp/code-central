package store_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 2 (ticket 42) added the typed link. The real chain migrates a
// format-1 store, whose items have no link, without changing them.
func TestTheRealChainMigratesAFormat1StoreToTheLinkFormat(t *testing.T) {
	dir := seedFormat1(t)
	s := openFile(t, dir)
	if m := s.Migrated(); m == nil || m.From != 1 {
		t.Fatalf("migrated = %+v, want a migration from format 1", m)
	}
	if got := readFile(t, filepath.Join(dir, "store.yaml")); got != fmt.Sprintf("format: %d\n", store.Format) {
		t.Fatalf("store.yaml = %q", got)
	}
	s.View(func(r store.Reader) error {
		tk, ok := r.Ticket("acme", 2)
		if !ok || tk.Title != "Form" || tk.Link.Linked() {
			t.Fatalf("ticket 2 after migrating: %+v, %v", tk, ok)
		}
		return nil
	})
}
