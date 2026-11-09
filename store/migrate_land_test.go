package store_test

import (
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 5 (ticket 71) added the Accepted State, a Ticket's finish branch
// and commit, and a land's commit on its Move. The real chain migrates a
// format-4 store, which has none of them, leaving its Ticket files
// unchanged.
func TestTheRealChainMigratesAFormat4StoreToTheLandFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 4\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n")
	ticket := `title: Form
state: Done
spec: 1
labels: []
tags: {}
blocked_by: []
replacements: []
created: 2026-01-02T03:04:05Z
updated: 2026-01-02T03:04:05Z
description: ""
claims: []
rounds: []
failure_reports: []
comments: []
moves:
  - from: In Review
    to: Done
    actor: human
    time: 2026-01-02T03:04:05Z
edits: []
`
	writeFile(t, filepath.Join(dir, "projects/acme/specs/1.md"),
		"---\nid: 1\ntitle: Login\nlabels: []\ntags: {}\ncreated: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\nedits: []\n---\n")
	writeFile(t, filepath.Join(dir, "projects/acme/tickets/2.yaml"), ticket)

	s := openFile(t, dir)
	if m := s.Migrated(); m == nil || m.From != 4 || m.To != store.Format {
		t.Fatalf("migrated = %+v, want a migration from format 4 to the current format", m)
	}
	if got := readFile(t, filepath.Join(dir, "projects/acme/tickets/2.yaml")); got != ticket {
		t.Fatalf("the Ticket file changed:\n%s", got)
	}
	s.View(func(r store.Reader) error {
		tk, ok := r.Ticket("acme", 2)
		if !ok || tk.State != store.Done || tk.Branch != "" || tk.Commit != "" || tk.Moves[0].Commit != "" {
			t.Fatalf("ticket 2 after migrating: %+v, %v", tk, ok)
		}
		return nil
	})
}
