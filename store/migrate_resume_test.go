package store_test

import (
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 4 (ticket 69) added a Claim's working_dir and resume_command and
// their copies on Failure Reports. The real chain migrates a format-3
// store, whose Claims have neither, leaving its Ticket files unchanged.
func TestTheRealChainMigratesAFormat3StoreToTheResumeFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 3\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n")
	ticket := `title: Form
state: Failed
spec: 1
labels: []
tags: {}
blocked_by: []
replacements: []
created: 2026-01-02T03:04:05Z
updated: 2026-01-02T03:04:05Z
description: ""
claims:
  - number: 1
    id: ABC
    session_id: s1
    started: 2026-01-02T03:04:05Z
    ended: 2026-01-02T03:04:05Z
    ended_by: report_failure
rounds: []
failure_reports:
  - number: 1
    claim: 1
    session_id: s1
    actor: agent
    time: 2026-01-02T03:04:05Z
    what_broke: x
    why: x
    what_to_fix: x
    where_work_stands: x
comments: []
moves: []
edits: []
`
	writeFile(t, filepath.Join(dir, "projects/acme/specs/1.md"),
		"---\nid: 1\ntitle: Login\nlabels: []\ntags: {}\ncreated: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\nedits: []\n---\n")
	writeFile(t, filepath.Join(dir, "projects/acme/tickets/2.yaml"), ticket)

	s := openFile(t, dir)
	if m := s.Migrated(); m == nil || m.From != 3 || m.To != store.Format {
		t.Fatalf("migrated = %+v, want a migration from format 3 to the current format", m)
	}
	if got := readFile(t, filepath.Join(dir, "projects/acme/tickets/2.yaml")); got != ticket {
		t.Fatalf("the Ticket file changed:\n%s", got)
	}
	s.View(func(r store.Reader) error {
		tk, ok := r.Ticket("acme", 2)
		if !ok || tk.Claims[0].SessionID != "s1" || tk.Claims[0].WorkingDir != "" || tk.Claims[0].ResumeCommand != "" ||
			tk.FailureReports[0].WorkingDir != "" || tk.FailureReports[0].ResumeCommand != "" {
			t.Fatalf("ticket 2 after migrating: %+v, %v", tk, ok)
		}
		return nil
	})
}
