package store_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 28 (code-central-432) added a Project's CI responses: the
// human's Land anyway and Re-run request, once per pushed commit. The
// real chain migrates a format-27 store, which has none and whose pushed
// commits may still carry them, copying every file unchanged.
func TestTheRealChainMigratesAFormat27StoreToTheCIResponsesFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 27\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\nci:\n  tracking: true\n  provider: github_actions\n  repository: acme/web\n")
	writeFile(t, filepath.Join(dir, "projects/acme/tickets/2.yaml"),
		"title: Accepted\nstate: Accepted\nspec: 1\ncreated: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\ndescription: \"\"\npushed_commits:\n  - commit: abc123\n    pushed: 2026-01-02T03:04:05Z\n    land_anyway: 2026-01-02T03:05:05Z\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	bak := filepath.Join(filepath.Dir(dir), "code-central.format-27.bak")
	if m := s.Migrated(); m == nil || *m != (store.Migration{From: 27, To: store.Format, Backup: bak}) {
		t.Fatalf("migrated = %+v, want a migration from format 27 with its backup", m)
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
		if p, _ := r.Project("acme"); len(p.CIResponses) != 0 {
			t.Errorf("CI responses after migrating = %+v, want none", p.CIResponses)
		}
		if tk, _ := r.Ticket("acme", 2); len(tk.PushedCommits) != 1 || tk.PushedCommits[0].LandAnyway.IsZero() {
			t.Errorf("pushed commits after migrating = %+v, want abc123 with its Land anyway kept", tk.PushedCommits)
		}
		return nil
	})
}
