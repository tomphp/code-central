package store_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 11 (code-central-256) added a Ticket's pushed commits and a
// Project's CI Runs. The real chain migrates a format-10 store, whose
// Tickets have no pushed commits and whose Projects have no ci-runs.yaml,
// copying every file unchanged.
func TestTheRealChainMigratesAFormat10StoreToTheCIRunsFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 10\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\nci:\n  tracking: true\n  provider: github_actions\n  repository: acme/web\n")
	writeFile(t, filepath.Join(dir, "projects/acme/tickets/2.yaml"),
		"title: Accepted\nstate: Accepted\nspec: 1\ncreated: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\ndescription: \"\"\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	bak := filepath.Join(filepath.Dir(dir), "code-central.format-10.bak")
	if m := s.Migrated(); m == nil || *m != (store.Migration{From: 10, To: store.Format, Backup: bak}) {
		t.Fatalf("migrated = %+v, want a migration from format 10 with its backup", m)
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
		if tk, _ := r.Ticket("acme", 2); len(tk.PushedCommits) != 0 {
			t.Errorf("pushed commits after migrating = %+v, want none", tk.PushedCommits)
		}
		if runs := r.CIRuns("acme"); len(runs) != 0 {
			t.Errorf("ci runs after migrating = %+v, want none", runs)
		}
		return nil
	})
}
