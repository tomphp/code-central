package store_test

import (
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

// Format 56 added Azure Boards' Branch format. The real chain migrates a
// format-55 store: Azure Boards stored without it has none.
func TestTheRealChainMigratesAFormat55StoreToTheBranchFormatFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 55\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"), "key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\nazure_boards:\n  on: true\n  query_url: https://dev.azure.com/contoso/web/_queries/query/abc\n  hide_code_central: true\n")

	s := openFile(t, dir)
	s.View(func(r store.Reader) error {
		p, _ := r.Project("acme")
		if !p.AzureBoards.HideCodeCentral || p.AzureBoards.BranchFormat != "" {
			t.Errorf("Azure Boards = %+v, want Hide Code Central kept and no Branch format", p.AzureBoards)
		}
		return nil
	})
}
