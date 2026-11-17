package store_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/store"
)

// homeCI is the Home Repository's stored CI entry in p, named by its Key,
// or the zero entry when it has none.
func homeCI(p store.Project) store.CIRepository {
	if i := slices.IndexFunc(p.CI.Repositories, func(e store.CIRepository) bool { return e.Name == p.Key }); i >= 0 {
		return p.CI.Repositories[i]
	}
	return store.CIRepository{}
}

// Format 50 (code-central-900) moved the Home Repository's CI choice
// from the CI settings' own provider and target into a per-Repository CI
// entry like the others', and added the Factory's report that the Home's
// clone has no Push remote. The real chain migrates a format-49 store
// copying every file unchanged: the old fields load as the Home's entry,
// and the next write puts it in the new place.
func TestTheRealChainMigratesAFormat49StoreToTheHomeCIEntryFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 49\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n"+
			"ci:\n  tracking: true\n  provider: github_actions\n  repository: acme/web\n  push_remote_url: git@github.com:acme/web.git\n"+
			"  repositories:\n    - name: api\n      not_watched: true\n"+
			"repositories:\n  - name: acme\n    home: true\n  - name: api\n    clone_url: git@github.com:acme/api.git\n    base: main\n")
	writeFile(t, filepath.Join(dir, "projects/ops/project.yaml"),
		"key: ops\ndisplay_name: Ops\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n"+
			"ci:\n  tracking: false\n  provider: azure_pipelines\n  repository: acme/stale\n  project_url: https://dev.azure.com/acme/ops\n")
	writeFile(t, filepath.Join(dir, "projects/docs/project.yaml"),
		"key: docs\ndisplay_name: Docs\ncreated: 2026-01-02T03:04:05Z\nmode: committed\n"+
			"ci:\n  tracking: false\n  provider: github_actions\n  push_remote_url: git@github.com:acme/docs.git\n")
	before := snapshot(t, dir)

	s := openFile(t, dir)
	if m := s.Migrated(); m == nil || m.From != 49 || m.To != store.Format {
		t.Fatalf("migrated = %+v, want a migration from format 49", m)
	}
	after := snapshot(t, dir)
	for rel, content := range before {
		if rel != "store.yaml" && after[rel] != content {
			t.Errorf("%s changed:\n%s", rel, after[rel])
		}
	}
	s.View(func(r store.Reader) error {
		acme, _ := r.Project("acme")
		want := []store.CIRepository{{Name: "acme", Provider: store.CIGitHubActions, Repository: "acme/web"}, {Name: "api", NotWatched: true}}
		if !acme.CI.Tracking || !slices.Equal(acme.CI.Repositories, want) || acme.CI.NoPushRemote {
			t.Errorf("acme's CI = %+v, want tracking on with the Home's entry %+v first", acme.CI, want)
		}
		ops, _ := r.Project("ops")
		if got, want := homeCI(ops), (store.CIRepository{Name: "ops", Provider: store.CIAzurePipelines, Repository: "acme/stale", ProjectURL: "https://dev.azure.com/acme/ops"}); got != want {
			t.Errorf("ops's Home entry = %+v, want %+v, each provider's own kept", got, want)
		}
		// GitHub Actions naming no repository watched nothing: the Home's
		// entry is left to be filled in from its Clone URL.
		docs, _ := r.Project("docs")
		if len(docs.CI.Repositories) != 0 {
			t.Errorf("docs's CI entries = %+v, want none", docs.CI.Repositories)
		}
		return nil
	})

	if err := s.Update(func(w store.Writer) error {
		p, _ := w.Project("acme")
		p.DisplayName = "Acme 2"
		return w.PutProject(p)
	}); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "projects/acme/project.yaml"))
	want := "ci:\n  tracking: true\n  push_remote_url: git@github.com:acme/web.git\n  repositories:\n" +
		"    - name: acme\n      provider: github_actions\n      repository: acme/web\n    - name: api\n      not_watched: true\n"
	if !strings.Contains(got, want) {
		t.Fatalf("project.yaml =\n%s\nwant its ci written in the new place:\n%s", got, want)
	}
	s = reopen(t, s)
	s.View(func(r store.Reader) error {
		p, _ := r.Project("acme")
		if got := homeCI(p); got != (store.CIRepository{Name: "acme", Provider: store.CIGitHubActions, Repository: "acme/web"}) {
			t.Errorf("the Home's entry after a write = %+v, want it kept", got)
		}
		return nil
	})
}

// An unknown CI Provider in the older Home fields still fails to load.
func TestAnUnknownOlderHomeCIProviderFailsToLoad(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 49\n")
	writeFile(t, filepath.Join(dir, "projects/acme/project.yaml"),
		"key: acme\ndisplay_name: Acme\ncreated: 2026-01-02T03:04:05Z\nmode: committed\nci:\n  tracking: false\n  provider: jenkins\n")
	if _, err := store.Open(dir); err == nil || !strings.Contains(err.Error(), "ci.provider") {
		t.Fatalf("open = %v, want a load error on ci.provider", err)
	}
}
