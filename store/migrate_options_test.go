package store_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 6 (code-central-168) added the global Options file. The real
// chain migrates a format-5 store, which has none, copying every file
// unchanged and keeping the old store as a backup; the Options read as
// their defaults.
func TestTheRealChainMigratesAFormat5StoreToTheOptionsFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 5\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n")
	writeFile(t, filepath.Join(dir, "projects/acme/specs/1.md"),
		"---\nid: 1\ntitle: Login\nlabels: []\ntags: {}\ncreated: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\nedits: []\n---\n# Login\n")
	writeFile(t, filepath.Join(dir, "projects/acme/tickets/2.yaml"), `title: Form
state: Accepted
spec: 1
labels: []
tags: {}
blocked_by: []
replacements: []
created: 2026-01-02T03:04:05Z
updated: 2026-01-02T03:04:05Z
description: ""
branch: acme-2
commit: abc123
claims: []
rounds: []
failure_reports: []
comments: []
moves: []
edits: []
`)
	before := snapshot(t, dir)

	s := openFile(t, dir)
	bak := filepath.Join(filepath.Dir(dir), "code-central.format-5.bak")
	if m := s.Migrated(); m == nil || *m != (store.Migration{From: 5, To: store.Format, Backup: bak}) {
		t.Fatalf("migrated = %+v, want a migration from format 5 with its backup", m)
	}
	after := snapshot(t, dir)
	for rel, content := range before {
		if rel == "store.yaml" {
			continue
		}
		if after[rel] != content {
			t.Errorf("%s changed:\n%s", rel, after[rel])
		}
	}
	if len(after) != len(before) {
		t.Errorf("files after migrating: %v", after)
	}
	if got := after["store.yaml"]; got != fmt.Sprintf("format: %d\n", store.Format) {
		t.Errorf("store.yaml = %q", got)
	}
	if got := snapshot(t, bak); fmt.Sprint(got) != fmt.Sprint(before) {
		t.Errorf("the backup is not the format-5 store:\n%v", got)
	}
	s.View(func(r store.Reader) error {
		if o := r.Options(); o.FocusClear != store.FocusActivityDashboard {
			t.Errorf("options after migrating = %+v, want the defaults", o)
		}
		return nil
	})
}
