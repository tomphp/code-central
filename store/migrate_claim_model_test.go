package store_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 19 (code-central-309) added the model and effort a Claim's
// latest turn ran with. The real chain migrates a format-18 store, whose
// Claims have neither, copying every file unchanged; a Claim from before
// reads as having run on Claude Code's default.
func TestTheRealChainMigratesAFormat18StoreToTheClaimModelFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 18\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n")
	writeFile(t, filepath.Join(dir, "projects/acme/tickets/2.yaml"),
		"title: Form\nstate: In Progress\nspec: 1\ntags:\n  model: opus\nclaims:\n  - number: 1\n    id: ABC\n    session_id: s1\n    started: 2026-01-02T03:04:05Z\ncreated: 2026-01-02T03:04:05Z\nupdated: 2026-01-02T03:04:05Z\ndescription: \"\"\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	bak := filepath.Join(filepath.Dir(dir), "code-central.format-18.bak")
	if m := s.Migrated(); m == nil || *m != (store.Migration{From: 18, To: store.Format, Backup: bak}) {
		t.Fatalf("migrated = %+v, want a migration from format 18 with its backup", m)
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
		if len(tk.Claims) != 1 || tk.Claims[0].SessionID != "s1" || tk.Claims[0].Model != "" || tk.Claims[0].Effort != "" || tk.Tags["model"] != "opus" {
			t.Errorf("ticket after migrating = %+v, want its Claim and Tags kept, with no model or effort recorded", tk)
		}
		return nil
	})

	// A model and effort recorded after migrating read back.
	if err := s.Update(func(w store.Writer) error {
		tk, _ := w.Ticket("acme", 2)
		tk.Claims[0].Model, tk.Claims[0].Effort = "opus", "high"
		return w.PutTicket("acme", tk)
	}); err != nil {
		t.Fatal(err)
	}
	s = reopen(t, s)
	s.View(func(r store.Reader) error {
		if tk, _ := r.Ticket("acme", 2); tk.Claims[0].Model != "opus" || tk.Claims[0].Effort != "high" {
			t.Errorf("claim = %+v, want opus at high", tk.Claims[0])
		}
		return nil
	})
}
