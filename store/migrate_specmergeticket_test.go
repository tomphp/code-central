package store_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 33 (code-central-542) added a Ticket's Spec merge mark. The real
// chain migrates a format-32 store, whose Tickets have none, copying
// every file unchanged; a Spec merge Ticket the Factory opened before
// reads as unmarked, so it stays manual.
func TestTheRealChainMigratesAFormat32StoreToTheSpecMergeMarkFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 32\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n")
	writeFile(t, filepath.Join(dir, "projects/acme/tickets/2.yaml"),
		"title: Merge the base into the Spec Branch of acme-1\nstate: Ready\nspec: 1\ncreated: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\ndescription: \"\"\n")
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
		tk, _ := r.Ticket("acme", 2)
		if tk.SpecMergeTicket || tk.Title != "Merge the base into the Spec Branch of acme-1" {
			t.Errorf("ticket after migrating = %+v, want it kept and unmarked", tk)
		}
		return nil
	})
}
