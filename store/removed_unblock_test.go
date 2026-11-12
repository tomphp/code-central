package store_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Unblock on push was removed (ADR 0032). A Project file that still holds
// unblock_on_push loads, and rewriting the Project drops the value.
func TestAProjectFileStillHoldingUnblockOnPushLoadsAndDropsIt(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), fmt.Sprintf("format: %d\n", store.Format))
	file := filepath.Join(dir, "projects/acme/project.yaml")
	writeFile(t, file,
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\nci:\n  tracking: true\n  provider: github_actions\n  repository: acme/web\n  unblock_on_push: true\n")

	s := openFile(t, dir)
	err := s.Update(func(w store.Writer) error {
		p, _ := w.Project("acme")
		if !p.CI.Tracking || p.CI.Repository != "acme/web" {
			t.Errorf("ci = %+v, want tracking acme/web", p.CI)
		}
		return w.PutProject(p)
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshot(t, dir)["projects/acme/project.yaml"]; strings.Contains(got, "unblock_on_push") {
		t.Errorf("project.yaml after a rewrite:\n%s\nwant unblock_on_push dropped", got)
	}
}
