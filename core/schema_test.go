package core_test

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/store"
)

var writeSchema = flag.Bool("write-store-schema", false,
	"write the store schema snapshot for a newly bumped store.Format (never overwrites one)")

// snapshotPath is the checked-in schema of one store format. Every past
// format keeps its snapshot, unchanged, for good.
func snapshotPath(format int) string {
	return filepath.Join("testdata", "store-schema", fmt.Sprintf("format-%d.txt", format))
}

// howToBump is what to do when the schema changed: files are decoded
// strictly, so even an additive change must bump the format, or an older
// binary fails on "unknown field" instead of the clear "unknown format".
func howToBump(old int) string {
	next := old + 1
	return fmt.Sprintf(`the store schema changed without a format bump. Every change to a store file struct
(store/file.go), its yaml tags or the checked value sets bumps the format, even a purely
additive one. To ship it:
  1. In store/format_current.go, raise currentFormat from %[1]d to %[2]d.
  2. In store/migrate.go, add a step to migrations:
       {From: %[1]d, Name: "<what changed>"}
     An additive change (a new optional field) needs no File func: the migration only
     rewrites the number in store.yaml. A change to existing data needs a File func
     that rewrites each affected file.
  3. Run: go test ./core -run TestTheStoreSchemaMatchesItsSnapshot -write-store-schema
     It writes core/testdata/store-schema/format-%[2]d.txt. Never edit format-%[1]d.txt.
  4. Add a store test that migrates a format-%[1]d store to format %[2]d, and update the
     e2e synthetic store in e2e/migration_test.go if the files it writes changed.
Diff the current schema against %[3]s to see what changed`, old, next, snapshotPath(old))
}

func TestTheStoreSchemaMatchesItsSnapshot(t *testing.T) {
	got := store.Schema()
	path := snapshotPath(store.Format)
	want, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist) && *writeSchema:
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", path)
		return
	case errors.Is(err, fs.ErrNotExist):
		t.Fatalf("store.Format is %d but there is no %s. If you just bumped the format, run\n"+
			"  go test ./core -run TestTheStoreSchemaMatchesItsSnapshot -write-store-schema", store.Format, path)
	case err != nil:
		t.Fatal(err)
	}
	if string(want) != got {
		t.Fatalf("%s\n\ncurrent schema:\n%s", howToBump(store.Format), got)
	}
}

func TestEveryPastStoreFormatKeepsItsSnapshot(t *testing.T) {
	for n := 1; n <= store.Format; n++ {
		if _, err := os.Stat(snapshotPath(n)); err != nil {
			t.Errorf("%s is missing: past snapshots stay checked in, unchanged, for good", snapshotPath(n))
		}
	}
	if _, err := os.Stat(snapshotPath(store.Format + 1)); err == nil {
		t.Errorf("%s exists but store.Format is %d: raise currentFormat in store/format_current.go",
			snapshotPath(store.Format+1), store.Format)
	}
}
