package store_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 13 (code-central-265) added the human's Re-run request on a
// pushed commit. The real chain migrates a format-12 store, whose pushed commits
// have none, copying every file unchanged.
func TestTheRealChainMigratesAFormat12StoreToTheRerunFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 12\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\nci:\n  tracking: true\n  provider: github_actions\n  repository: acme/web\n")
	writeFile(t, filepath.Join(dir, "projects/acme/tickets/2.yaml"),
		"title: Accepted\nstate: Accepted\nspec: 1\ncreated: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\ndescription: \"\"\npushed_commits:\n  - commit: abc123\n    pushed: 2026-01-02T03:04:05Z\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	bak := filepath.Join(filepath.Dir(dir), "code-central.format-12.bak")
	if m := s.Migrated(); m == nil || *m != (store.Migration{From: 12, To: store.Format, Backup: bak}) {
		t.Fatalf("migrated = %+v, want a migration from format 12 with its backup", m)
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
		if len(tk.PushedCommits) != 1 || tk.PushedCommits[0].Commit != "abc123" {
			t.Errorf("pushed commits after migrating = %+v, want abc123", tk.PushedCommits)
		}
		if p, _ := r.Project("acme"); len(p.CIResponses) != 0 {
			t.Errorf("CI responses after migrating = %+v, want no Re-run request", p.CIResponses)
		}
		return nil
	})
}
