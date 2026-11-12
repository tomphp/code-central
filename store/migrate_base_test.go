package store_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 26 (code-central-434) added a Project's base to its Landing. The
// real chain migrates a format-25 store copying every file unchanged; a
// Project from before reads its base as the fallback.
func TestTheRealChainMigratesAFormat25StoreToTheBaseFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 25\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\nci:\n  provider: github_actions\n  push_remote_base: release\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	bak := filepath.Join(filepath.Dir(dir), "code-central.format-25.bak")
	if m := s.Migrated(); m == nil || *m != (store.Migration{From: 25, To: store.Format, Backup: bak}) {
		t.Fatalf("migrated = %+v, want a migration from format 25 with its backup", m)
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
		if p.Landing.Base != "" || p.Base() != "release" {
			t.Errorf("base = %q stored, %q resolved, want none stored and release resolved", p.Landing.Base, p.Base())
		}
		return nil
	})
}
