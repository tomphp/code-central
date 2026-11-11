package store_test

import (
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 21 added a Project's Land per Spec. The real chain migrates a
// format-20 store: a Project with no stored value reads as on, with a
// stored Landing that lacks it too, and a stored false stays off.
func TestTheRealChainMigratesAFormat20StoreToTheLandPerSpecFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 20\n")
	head := "created: 2026-01-02T03:04:05Z\nmode: committed\n"
	writeFile(t, filepath.Join(dir, "projects/none/project.yaml"), "key: none\ndisplay_name: None\n"+head)
	writeFile(t, filepath.Join(dir, "projects/absent/project.yaml"), "key: absent\ndisplay_name: Absent\n"+head+"landing:\n  method: merge\n  push: true\n  remote: origin\n")
	writeFile(t, filepath.Join(dir, "projects/off/project.yaml"), "key: off\ndisplay_name: Off\n"+head+"landing:\n  method: merge\n  push: false\n  remote: origin\n  land_per_spec: false\n")
	writeFile(t, filepath.Join(dir, "projects/on/project.yaml"), "key: on\ndisplay_name: On\n"+head+"landing:\n  method: branch\n  push: false\n  remote: origin\n  land_per_spec: true\n")

	s := openFile(t, dir)
	s.View(func(r store.Reader) error {
		for key, want := range map[string]bool{"none": true, "absent": true, "off": false, "on": true} {
			if p, _ := r.Project(key); p.Landing.LandPerSpec != want {
				t.Errorf("%s: land per spec = %v, want %v", key, p.Landing.LandPerSpec, want)
			}
		}
		return nil
	})
}
