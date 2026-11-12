package store_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 29 (code-central-433) added a Project's CI Fixes Spec and a CI
// response's fix Ticket. The real chain migrates a format-28 store, which
// has neither, copying every file unchanged.
func TestTheRealChainMigratesAFormat28StoreToTheCIFixesFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 28\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\nci_responses:\n  - commit: abc123\n    land_anyway: 2026-01-02T03:05:05Z\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	bak := filepath.Join(filepath.Dir(dir), "code-central.format-28.bak")
	if m := s.Migrated(); m == nil || *m != (store.Migration{From: 28, To: store.Format, Backup: bak}) {
		t.Fatalf("migrated = %+v, want a migration from format 28 with its backup", m)
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
		if p.CIFixesSpec != 0 || len(p.CIResponses) != 1 || p.CIResponses[0].FixTicket != 0 {
			t.Errorf("project after migrating = %+v, want no CI Fixes Spec or fix Ticket", p)
		}
		return nil
	})
}
