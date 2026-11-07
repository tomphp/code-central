package store_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomoram/code-central/store"
	"github.com/tomoram/code-central/store/storetest"
)

// These tests play a build whose format is 2 (or 3), with test-only
// steps, against a store written by the real format-1 code.

// seedFormat1 writes a format-1 store with a Project, a Spec and a
// Ticket, and closes it.
func seedFormat1(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "code-central")
	s := seedAcme(t, dir)
	mustUpdate(t, s, func(w store.Writer) error {
		return w.PutSpec("acme", store.Spec{Number: 1, Title: "Login", Created: t0, Updated: t0, Body: "# Login\n"})
	})
	mustUpdate(t, s, func(w store.Writer) error {
		return w.PutTicket("acme", store.Ticket{Number: 2, Title: "Form", State: store.Ready, Spec: 1, Created: t0, Updated: t0,
			Moves: []store.Move{{From: store.Draft, To: store.Ready, Actor: "agent", Time: t0}}})
	})
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	return dir
}

// snapshot is every file under dir (the lock aside) and its contents.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, f := range listFiles(t, dir) {
		if f == "lock" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			t.Fatal(err)
		}
		out[f] = string(data)
	}
	return out
}

func openFormat(t *testing.T, dir string, format int, steps ...store.MigrationStep) *store.FileStore {
	t.Helper()
	s, err := store.OpenFormat(dir, format, steps)
	if err != nil {
		t.Fatalf("open as format %d: %v", format, err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// marking appends a YAML comment to each Ticket file, so a test can see
// which files a step rewrote.
func marking(from int, mark string) store.MigrationStep {
	return store.MigrationStep{From: from, Name: "mark tickets " + mark, File: func(rel string, data []byte) ([]byte, error) {
		if strings.Contains(rel, "/tickets/") {
			return append(data, "# "+mark+"\n"...), nil
		}
		return data, nil
	}}
}

// siblings are the entries next to the store folder.
func siblings(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestAnOlderFormatIsMigratedThroughStagingAndBackedUp(t *testing.T) {
	dir := seedFormat1(t)
	before := snapshot(t, dir)

	s := openFormat(t, dir, 2, marking(1, "migrated"))

	if got := readFile(t, filepath.Join(dir, "store.yaml")); got != "format: 2\n" {
		t.Fatalf("store.yaml = %q", got)
	}
	if got := readFile(t, filepath.Join(dir, "projects/acme/tickets/2.yaml")); !strings.HasSuffix(got, "# migrated\n") {
		t.Fatalf("the step did not rewrite the ticket:\n%s", got)
	}
	if got := readFile(t, filepath.Join(dir, "projects/acme/specs/1.md")); got != before["projects/acme/specs/1.md"] {
		t.Fatalf("an unchanged file changed:\n%s", got)
	}
	bak := filepath.Join(filepath.Dir(dir), "code-central.format-1.bak")
	if m := s.Migrated(); m == nil || *m != (store.Migration{From: 1, To: 2, Backup: bak}) {
		t.Fatalf("Migrated() = %+v", m)
	}
	if got := snapshot(t, bak); fmt.Sprint(got) != fmt.Sprint(before) {
		t.Fatalf("the backup is not the old store:\n%v\nwant\n%v", got, before)
	}
	if got := siblings(t, dir); fmt.Sprint(got) != "[code-central code-central.format-1.bak]" {
		t.Fatalf("next to the store: %v (want no staging folder left)", got)
	}

	// The migrated store is loaded, locked and writable.
	s.View(func(r store.Reader) error {
		if tk, ok := r.Ticket("acme", 2); !ok || tk.Title != "Form" {
			t.Fatalf("ticket 2 after migration: %+v %v", tk, ok)
		}
		return nil
	})
	if _, err := store.OpenFormat(dir, 2, nil); !errors.Is(err, store.ErrLocked) {
		t.Fatalf("second open of the migrated store: %v, want ErrLocked", err)
	}
	mustUpdate(t, s, func(w store.Writer) error {
		return w.PutTicket("acme", store.Ticket{Number: 3, Title: "More", State: store.Draft, Spec: 1, Created: t0, Updated: t0})
	})
	if _, err := os.Stat(filepath.Join(bak, "projects/acme/tickets/3.yaml")); err == nil {
		t.Fatal("a write after the migration reached the backup")
	}

	// Opening again as format 2 migrates nothing.
	s.Close()
	if again := openFormat(t, dir, 2, marking(1, "again")); again.Migrated() != nil {
		t.Fatalf("Migrated() = %+v on a store already at format 2", again.Migrated())
	}
	if got := readFile(t, filepath.Join(dir, "projects/acme/tickets/2.yaml")); strings.Contains(got, "again") {
		t.Fatal("a store already at the build's format was migrated again")
	}
}

func TestAnAdditiveStepOnlyRewritesTheNumber(t *testing.T) {
	dir := seedFormat1(t)
	before := snapshot(t, dir)
	openFormat(t, dir, 2, store.MigrationStep{From: 1, Name: "add an optional field"})
	after := snapshot(t, dir)
	if after["store.yaml"] != "format: 2\n" {
		t.Fatalf("store.yaml = %q", after["store.yaml"])
	}
	delete(before, "store.yaml")
	delete(after, "store.yaml")
	if fmt.Sprint(after) != fmt.Sprint(before) {
		t.Fatalf("an additive step changed files:\n%v\nwant\n%v", after, before)
	}
}

func TestStepsAreChainedAndOneBackupIsKeptPerFormat(t *testing.T) {
	dir := seedFormat1(t)
	s := openFormat(t, dir, 3, marking(1, "one"), marking(2, "two"))
	if got := readFile(t, filepath.Join(dir, "projects/acme/tickets/2.yaml")); !strings.HasSuffix(got, "# one\n# two\n") {
		t.Fatalf("steps did not run in order:\n%s", got)
	}
	s.Close()
	if got := siblings(t, dir); fmt.Sprint(got) != "[code-central code-central.format-1.bak]" {
		t.Fatalf("a chained migration from format 1 keeps one backup: %v", got)
	}

	// A later upgrade from format 3 adds a backup and keeps the old one.
	openFormat(t, dir, 4, marking(1, "one"), marking(2, "two"), marking(3, "three"))
	if got := siblings(t, dir); fmt.Sprint(got) != "[code-central code-central.format-1.bak code-central.format-3.bak]" {
		t.Fatalf("after a second upgrade: %v", got)
	}
	if got := readFile(t, filepath.Join(dir, "projects/acme/tickets/2.yaml")); !strings.HasSuffix(got, "# one\n# two\n# three\n") {
		t.Fatalf("the second upgrade ran the wrong steps:\n%s", got)
	}
}

func TestAFailedStepLeavesTheStoreUntouched(t *testing.T) {
	dir := seedFormat1(t)
	before := snapshot(t, dir)
	failing := store.MigrationStep{From: 1, Name: "split the description", File: func(rel string, data []byte) ([]byte, error) {
		if rel == "projects/acme/tickets/2.yaml" {
			return nil, errors.New("no description to split")
		}
		return data, nil
	}}
	_, err := store.OpenFormat(dir, 2, []store.MigrationStep{failing})
	if err == nil {
		t.Fatal("a failing step let the store open")
	}
	for _, want := range []string{"format 1 to 2", "split the description", "projects/acme/tickets/2.yaml", "no description to split", "untouched"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not say %q", err, want)
		}
	}
	if got := snapshot(t, dir); fmt.Sprint(got) != fmt.Sprint(before) {
		t.Fatalf("a failed migration changed the store:\n%v", got)
	}
	if got := siblings(t, dir); fmt.Sprint(got) != "[code-central]" {
		t.Fatalf("a failed migration left %v", got)
	}
	// The lock was released and the old build still opens it.
	openFile(t, dir).Close()
}

func TestAMissingStepRefusesToStart(t *testing.T) {
	dir := seedFormat1(t)
	_, err := store.OpenFormat(dir, 3, []store.MigrationStep{marking(1, "one")})
	if err == nil || !strings.Contains(err.Error(), "no migration from format 2") {
		t.Fatalf("err = %v", err)
	}
	if got := readFile(t, filepath.Join(dir, "store.yaml")); got != "format: 1\n" {
		t.Fatalf("store.yaml = %q", got)
	}
}

func TestANewerFormatIsStillRefusedByName(t *testing.T) {
	dir := seedFormat1(t)
	writeFile(t, filepath.Join(dir, "store.yaml"), "format: 3\n")
	_, err := store.OpenFormat(dir, 2, []store.MigrationStep{marking(1, "one")})
	if err == nil || !strings.Contains(err.Error(), "unknown format 3") || !strings.Contains(err.Error(), "store.yaml") {
		t.Fatalf("err = %v", err)
	}
}

func TestAnExistingBackupIsNeverOverwritten(t *testing.T) {
	dir := seedFormat1(t)
	bak := filepath.Join(filepath.Dir(dir), "code-central.format-1.bak")
	writeFile(t, filepath.Join(bak, "store.yaml"), "format: 1\n")
	before := snapshot(t, dir)
	_, err := store.OpenFormat(dir, 2, []store.MigrationStep{marking(1, "one")})
	if err == nil || !strings.Contains(err.Error(), bak) {
		t.Fatalf("err = %v, want it to name %s", err, bak)
	}
	if got := snapshot(t, dir); fmt.Sprint(got) != fmt.Sprint(before) {
		t.Fatal("the store changed")
	}
	if got := snapshot(t, bak); fmt.Sprint(got) != "map[store.yaml:format: 1\n]" {
		t.Fatalf("the existing backup changed: %v", got)
	}
}

func TestAMigrationStoppedBetweenTheRenamesFinishesAtStartup(t *testing.T) {
	dir := seedFormat1(t)
	store.StopMigrationBetweenRenames(t)
	if _, err := store.OpenFormat(dir, 2, []store.MigrationStep{marking(1, "migrated")}); !errors.Is(err, store.ErrMigrationStopped) {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the hook did not stop between the renames: %v", err)
	}
	store.StopMigrationBetweenRenames(nil)

	s := openFormat(t, dir, 2, marking(1, "migrated"))
	if got := readFile(t, filepath.Join(dir, "projects/acme/tickets/2.yaml")); !strings.HasSuffix(got, "# migrated\n") {
		t.Fatalf("startup did not put the staged store in place:\n%s", got)
	}
	if got := siblings(t, dir); fmt.Sprint(got) != "[code-central code-central.format-1.bak]" {
		t.Fatalf("next to the store: %v", got)
	}
	s.View(func(r store.Reader) error {
		if _, ok := r.Ticket("acme", 2); !ok {
			t.Fatal("ticket 2 not loaded")
		}
		return nil
	})
}

func TestAPendingImportIsRolledForwardBeforeTheMigration(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "code-central")
	s := seedAcme(t, dir)
	store.StopImportAfter(s, len(importedFiles)+1) // the manifest is written, nothing renamed
	if err := s.Update(func(w store.Writer) error { return w.PutImport("acme", storetest.ImportBatch()) }); !errors.Is(err, store.ErrImportStopped) {
		t.Fatal(err)
	}
	s.Close()

	s = openFormat(t, dir, 2, marking(1, "migrated"))
	if sp, tk := counts(t, s); sp != 2 || tk != 3 {
		t.Fatalf("after recovery and migration: %d specs, %d tickets", sp, tk)
	}
	if got := readFile(t, filepath.Join(dir, "projects/acme/tickets/3.yaml")); !strings.HasSuffix(got, "# migrated\n") {
		t.Fatalf("the imported ticket was not migrated:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "code-central.format-1.bak", "projects/acme/tickets/3.yaml")); err != nil {
		t.Fatalf("the backup lacks the rolled-forward import: %v", err)
	}
}
