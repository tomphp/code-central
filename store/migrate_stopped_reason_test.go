package store_test

import (
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

func TestTheRealChainMigratesAFormat59StoreAndKeepsItsFocusKindsOff(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 59\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\nfocus_kinds_off:\n  - answered\n  - silent_claim\n")

	s := openFile(t, dir)
	s.View(func(r store.Reader) error {
		p, _ := r.Project("acme")
		if len(p.FocusKindsOff) != 2 || p.FocusKindsOff[0] != "answered" || p.FocusKindsOff[1] != "silent_claim" {
			t.Errorf("focus_kinds_off after migrating = %v, want answered and silent_claim", p.FocusKindsOff)
		}
		return nil
	})
}
