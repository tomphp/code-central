package store_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 14 (code-central-296) added a Project's Unblock on push. The
// real chain migrates a format-13 store, whose CI settings have none,
// copying every file unchanged; Unblock on push reads as off.
func TestTheRealChainMigratesAFormat13StoreToTheUnblockOnPushFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 13\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\nci:\n  tracking: true\n  provider: github_actions\n  repository: acme/web\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	bak := filepath.Join(filepath.Dir(dir), "code-central.format-13.bak")
	if m := s.Migrated(); m == nil || *m != (store.Migration{From: 13, To: store.Format, Backup: bak}) {
		t.Fatalf("migrated = %+v, want a migration from format 13 with its backup", m)
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
		if p, _ := r.Project("acme"); !p.CI.Tracking || p.CI.Repository != "acme/web" || p.CI.UnblockOnPush {
			t.Errorf("ci after migrating = %+v, want tracking acme/web with Unblock on push off", p.CI)
		}
		return nil
	})
}
