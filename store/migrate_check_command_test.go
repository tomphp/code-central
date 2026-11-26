package store_test

import (
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

func TestTheRealChainMigratesAFormat57StoreToTheCheckCommandFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 57\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"), "key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\nrepositories:\n  - name: acme\n    home: true\n    push_remote: upstream\n  - name: web\n    clone_url: git@github.com:acme/web.git\n    base: main\n")

	s := openFile(t, dir)
	s.View(func(r store.Reader) error {
		p, _ := r.Project("acme")
		if p.CheckCommand != "" {
			t.Errorf("Project Check Command = %q, want none", p.CheckCommand)
		}
		for _, rp := range p.RepositoryList() {
			if rp.CheckCommand != "" {
				t.Errorf("%s's Check Command = %q, want none", rp.Name, rp.CheckCommand)
			}
		}
		if home := p.RepositoryList()[0]; home.PushRemote != "upstream" {
			t.Errorf("Home Push remote = %q, want upstream kept", home.PushRemote)
		}
		return nil
	})
}
