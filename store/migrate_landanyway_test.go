package store_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 12 (code-central-261) added the human's Land anyway on a pushed
// commit. The real chain migrates a format-11 store, whose pushed commits
// have none, copying every file unchanged.
func TestTheRealChainMigratesAFormat11StoreToTheLandAnywayFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 11\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\nci:\n  tracking: true\n  provider: github_actions\n  repository: acme/web\n")
	writeFile(t, filepath.Join(dir, "projects/acme/tickets/2.yaml"),
		"title: Accepted\nstate: Accepted\nspec: 1\ncreated: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\ndescription: \"\"\npushed_commits:\n  - commit: abc123\n    pushed: 2026-01-02T03:04:05Z\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	bak := filepath.Join(filepath.Dir(dir), "code-central.format-11.bak")
	if m := s.Migrated(); m == nil || *m != (store.Migration{From: 11, To: store.Format, Backup: bak}) {
		t.Fatalf("migrated = %+v, want a migration from format 11 with its backup", m)
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
		if len(tk.PushedCommits) != 1 || tk.PushedCommits[0].Commit != "abc123" || !tk.PushedCommits[0].LandAnyway.IsZero() {
			t.Errorf("pushed commits after migrating = %+v, want abc123 with no Land anyway", tk.PushedCommits)
		}
		return nil
	})
}
