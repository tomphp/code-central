package store_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 16 (code-central-287) added a Ticket's awaiting triage. The real
// chain migrates a format-15 store, whose Tickets have none, copying
// every file unchanged; an Incoming Issue from before reads as not
// awaiting triage, so it never shows as a New issue.
func TestTheRealChainMigratesAFormat15StoreToTheAwaitingTriageFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 15\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\ngithub_issues:\n  on: true\n  repository: acme/web\n  new_issues_need_me: true\n  spec: 1\n")
	writeFile(t, filepath.Join(dir, "projects/acme/tickets/2.yaml"),
		"title: Login is broken\nstate: Draft\nspec: 1\nlink:\n  integration: github\n  id: acme/web#7\n  external_state: open\ncreated: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\ndescription: \"\"\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	bak := filepath.Join(filepath.Dir(dir), "code-central.format-15.bak")
	if m := s.Migrated(); m == nil || *m != (store.Migration{From: 15, To: store.Format, Backup: bak}) {
		t.Fatalf("migrated = %+v, want a migration from format 15 with its backup", m)
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
		if tk.AwaitingTriage || tk.Link.ID != "acme/web#7" {
			t.Errorf("ticket after migrating = %+v, want its Link kept and not awaiting triage", tk)
		}
		return nil
	})
}
