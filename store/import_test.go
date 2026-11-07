package store_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/store"
	"github.com/tomoram/code-central/store/storetest"
)

// importedFiles are the five files storetest.ImportBatch writes.
var importedFiles = []string{
	"projects/acme/specs/2.md", "projects/acme/specs/5.md",
	"projects/acme/tickets/3.yaml", "projects/acme/tickets/4.yaml", "projects/acme/tickets/6.yaml",
}

func seedAcme(t *testing.T, dir string) *store.FileStore {
	t.Helper()
	s := openFile(t, dir)
	mustUpdate(t, s, func(w store.Writer) error {
		return w.PutProject(store.Project{Key: "acme", DisplayName: "acme", Created: t0, Mode: "committed"})
	})
	return s
}

// reopen closes s and opens its directory again, which runs recovery.
func reopen(t *testing.T, s *store.FileStore) *store.FileStore {
	t.Helper()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	return openFile(t, s.Dir())
}

// counts is how many Specs and Tickets acme holds.
func counts(t *testing.T, s store.Store) (specs, tickets int) {
	t.Helper()
	s.View(func(r store.Reader) error {
		specs, tickets = len(r.Specs("acme")), len(r.Tickets("acme"))
		return nil
	})
	return
}

func present(dir string) []string {
	var out []string
	for _, f := range importedFiles {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			out = append(out, f)
		}
	}
	return out
}

func noStaging(t *testing.T, dir string) {
	t.Helper()
	entries, _ := os.ReadDir(filepath.Join(dir, "staging"))
	if len(entries) != 0 {
		t.Fatalf("staging still holds %d entries after recovery", len(entries))
	}
}

func TestAnImportStoppedAtAnyStepRecoversToAllOrNothing(t *testing.T) {
	const files = 5
	// stage 5 files, write the manifest, 5 renames, delete the manifest
	const steps = 2*files + 2
	for k := 1; k <= steps+1; k++ {
		t.Run(fmt.Sprintf("stopped after step %d", k), func(t *testing.T) {
			t.Parallel() // fsync-bound
			stopAt(t, k, files, steps)
		})
	}
}

// stopAt runs a real Import that stops after step k, then startup
// recovery, and checks the result is all of the Import or none of it. A
// k past the last step must let the Import finish.
func stopAt(t *testing.T, k, files, steps int) {
	dir := t.TempDir()
	s := seedAcme(t, dir)
	store.StopImportAfter(s, k)
	err := s.Update(func(w store.Writer) error { return w.PutImport("acme", storetest.ImportBatch()) })
	if k > steps {
		if err != nil {
			t.Fatalf("the import did not finish: %v", err)
		}
		return
	}
	if !errors.Is(err, store.ErrImportStopped) {
		t.Fatalf("step %d: %v", k, err)
	}
	if sp, tk := counts(t, s); sp != 0 || tk != 0 {
		t.Fatalf("step %d: memory changed before the import finished (%d specs, %d tickets)", k, sp, tk)
	}
	s = reopen(t, s)
	sp, tk := counts(t, s)
	committed := k > files // the manifest is written at step files+1
	switch {
	case committed && (sp != 2 || tk != 3):
		t.Fatalf("step %d: after recovery %d specs, %d tickets; want all of 2 and 3", k, sp, tk)
	case !committed && (sp != 0 || tk != 0):
		t.Fatalf("step %d: after recovery %d specs, %d tickets; want none", k, sp, tk)
	}
	if got := present(dir); committed != (len(got) == files) || (!committed && len(got) != 0) {
		t.Fatalf("step %d: files in place after recovery: %v", k, got)
	}
	noStaging(t, dir)
}

func TestAnOrphanStagingFolderIsDeletedAtStartup(t *testing.T) {
	dir := t.TempDir()
	s := seedAcme(t, dir)
	s.Close()
	orphan := filepath.Join(dir, "staging", "import-123")
	writeFile(t, filepath.Join(orphan, "0"), "title: T\n")
	s = openFile(t, dir)
	if _, err := os.Stat(orphan); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("orphan staging folder survived startup: %v", err)
	}
	if sp, tk := counts(t, s); sp != 0 || tk != 0 {
		t.Fatalf("an orphan's files were loaded: %d specs, %d tickets", sp, tk)
	}
}

func TestAPartlyRenamedManifestIsRolledForward(t *testing.T) {
	dir := t.TempDir()
	s := seedAcme(t, dir)
	store.StopImportAfter(s, 5+1+2) // the manifest and two renames are done
	if err := s.Update(func(w store.Writer) error { return w.PutImport("acme", storetest.ImportBatch()) }); !errors.Is(err, store.ErrImportStopped) {
		t.Fatalf("err = %v", err)
	}
	if got := present(dir); len(got) != 2 {
		t.Fatalf("before recovery %v are in place, want two", got)
	}
	manifests, _ := filepath.Glob(filepath.Join(dir, "staging", "*", "manifest.yaml"))
	if len(manifests) != 1 {
		t.Fatalf("manifests: %v", manifests)
	}
	s = reopen(t, s)
	if got := present(dir); !slices.Equal(got, importedFiles) {
		t.Fatalf("after recovery %v are in place", got)
	}
	if sp, tk := counts(t, s); sp != 2 || tk != 3 {
		t.Fatalf("after recovery %d specs, %d tickets", sp, tk)
	}
	noStaging(t, dir)
}

func TestAnImportedTicketsMoveHasNoFrom(t *testing.T) {
	dir := t.TempDir()
	s := seedAcme(t, dir)
	mustUpdate(t, s, func(w store.Writer) error { return w.PutImport("acme", storetest.ImportBatch()) })
	got, err := os.ReadFile(filepath.Join(dir, "projects/acme/tickets/3.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	want := "moves:\n  - to: Done\n    actor: agent\n    time: 2026-11-07T09:30:00Z\n    via: import\n"
	if !strings.Contains(string(got), want) {
		t.Fatalf("ticket file:\n%s\nwant it to contain:\n%s", got, want)
	}
}
