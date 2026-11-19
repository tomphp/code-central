package store_test

import (
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 55 added Azure Boards' Hide Code Central. The real chain migrates
// a format-54 store: Azure Boards stored without it reads as off.
func TestTheRealChainMigratesAFormat54StoreToTheHideCodeCentralFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 54\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"), "key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\nazure_boards:\n  on: true\n  query_url: https://dev.azure.com/contoso/web/_queries/query/abc\n")

	s := openFile(t, dir)
	s.View(func(r store.Reader) error {
		p, _ := r.Project("acme")
		if !p.AzureBoards.On || p.AzureBoards.HideCodeCentral {
			t.Errorf("Azure Boards = %+v, want on with Hide Code Central off", p.AzureBoards)
		}
		return nil
	})
}
