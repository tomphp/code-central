package store_test

import (
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 39 (code-central-667) added a Project's Retros Spec. The real
// chain migrates a format-38 store, which has none, copying every file
// unchanged.
func TestTheRealChainMigratesAFormat38StoreToTheRetrosSpecFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 38\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\nsmall_changes_spec: 4\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	if m := s.Migrated(); m == nil || m.From != 38 || m.To != store.Format {
		t.Fatalf("migrated = %+v, want a migration from format 38", m)
	}
	after := snapshot(t, dir)
	for rel, content := range before {
		if rel != "store.yaml" && after[rel] != content {
			t.Errorf("%s changed:\n%s", rel, after[rel])
		}
	}
	s.View(func(r store.Reader) error {
		p, _ := r.Project("acme")
		if p.RetrosSpec != 0 || p.SmallChangesSpec != 4 {
			t.Errorf("project after migrating = %+v, want no Retros Spec", p)
		}
		return nil
	})
}
