package store_test

import (
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

func TestTheRealChainMigratesAFormat56StoreToTheWorkItemStatesFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 56\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"), "key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\nazure_boards:\n  on: true\n  query_url: https://dev.azure.com/contoso/web/_queries/query/abc\n  branch_format: feat/{id}\n")

	s := openFile(t, dir)
	s.View(func(r store.Reader) error {
		p, _ := r.Project("acme")
		if p.AzureBoards.BranchFormat != "feat/{id}" || p.AzureBoards.WorkItemStates != (domain.WorkItemStates{}) {
			t.Errorf("Azure Boards = %+v, want Branch format kept and no Work Item States", p.AzureBoards)
		}
		return nil
	})
}
