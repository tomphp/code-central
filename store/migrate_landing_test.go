package store_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 7 (code-central-173) added a Project's Landing Method. The real
// chain migrates a format-6 store, whose project.yaml has none, copying
// every file unchanged; the Project reads as Merge, Push off, remote
// origin.
func TestTheRealChainMigratesAFormat6StoreToTheLandingMethodFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 6\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	bak := filepath.Join(filepath.Dir(dir), "code-central.format-6.bak")
	if m := s.Migrated(); m == nil || *m != (store.Migration{From: 6, To: store.Format, Backup: bak}) {
		t.Fatalf("migrated = %+v, want a migration from format 6 with its backup", m)
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
		if want := (store.Landing{Method: store.LandingMerge, Remote: "origin"}); p.Landing != want {
			t.Errorf("landing after migrating = %+v, want %+v", p.Landing, want)
		}
		return nil
	})
}
