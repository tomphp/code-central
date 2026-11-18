//go:build !js

// Migrating a SQLite Store (Spec code-central-1032) runs the File Store's
// chain of steps, so one chain serves every Kind. The database is
// exported as a File Store layout into a staging folder beside it
// (.<db>.staging), the File Store migrates that, and the result is
// imported into a new database file beside the old one (.<db>.migrating).
// Only then is the old database copied to <db>.format-<n>.bak by VACUUM
// INTO, closed, and the new one renamed over it. A failure while the old
// database is still open deletes the new file and the backup, and leaves
// the original untouched; once it is closed for the swap, the backup is
// kept whatever happens. Backups are never deleted after.

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// failSQLiteMigrationBeforeSwap, set only by tests, fails a Migration
// once its backup is written, just before the swap.
var failSQLiteMigrationBeforeSwap error

// sqliteBackupFor is where the database at path is kept after migrating
// from format n.
func sqliteBackupFor(path string, n int) string { return fmt.Sprintf("%s.format-%d.bak", path, n) }

// sqliteBeside is the hidden file or folder beside the database at path
// that a Migration names suffix.
func sqliteBeside(path, suffix string) string {
	return filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+suffix)
}

// migrate brings the open database from format from up to format, and
// leaves s open on the migrated database.
func (s *SQLStore) migrate(from, format int, steps []MigrationStep) error {
	ctx := context.Background()
	// The staged File Store works out its own chain; checked here first, a
	// missing step names the database rather than the staged store.yaml.
	if _, err := migrationChain(from, format, steps); err != nil {
		return fmt.Errorf("%s: %w", s.path, err)
	}
	bak := sqliteBackupFor(s.path, from)
	if _, err := os.Lstat(bak); !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("cannot migrate %s from format %d: the backup %s already exists; move it aside and start again (the store is untouched)", s.path, from, bak)
	}
	failed := func(what string, err error) error {
		return fmt.Errorf("migrating %s from format %d: %s: %w (the store is untouched)", s.path, from, what, err)
	}
	stage, next := sqliteBeside(s.path, ".staging"), sqliteBeside(s.path, ".migrating")
	// Either may be left by a Migration that never finished.
	if err := os.RemoveAll(stage); err != nil {
		return err
	}
	if err := removeSQLite(next); err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	oldOpen, swapped := true, false
	defer func() {
		if swapped {
			return
		}
		removeSQLite(next)
		if oldOpen { // the backup is a copy of a database still whole
			os.Remove(bak)
		}
	}()

	files := filepath.Join(stage, "store")
	if err := exportFiles(ctx, s.db, s.d, files, from); err != nil {
		return failed("staging it as files", err)
	}
	staged, err := open(files, format, steps)
	if err != nil {
		return fmt.Errorf("%s: %w", s.path, err)
	}
	err = buildSQLite(ctx, next, format, staged)
	staged.Close()
	if err != nil {
		return failed("loading the migrated records", err)
	}
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, bak); err != nil {
		return failed("backing it up to "+bak, err)
	}
	if failSQLiteMigrationBeforeSwap != nil {
		return failSQLiteMigrationBeforeSwap
	}

	// Closed, the old database has checkpointed its WAL into the file, so
	// none of it can be mistaken for the new database's.
	oldOpen = false
	err = s.db.Close()
	s.db = nil
	if err != nil {
		return err
	}
	if err := removeSQLiteSidecars(s.path); err != nil {
		return err
	}
	if err := os.Rename(next, s.path); err != nil {
		return err
	}
	swapped = true
	if err := syncDir(filepath.Dir(s.path)); err != nil {
		return err
	}
	if err := s.connect(); err != nil {
		return err
	}
	if _, err := prepare(ctx, s.db, s.d, format, s.path); err != nil {
		return err
	}
	s.migrated = &Migration{From: from, To: format, Backup: bak}
	return nil
}

// buildSQLite writes a new database at path, at format, holding every
// record and Usage record of from, and closes it, its WAL checkpointed
// into the file, ready to be renamed.
func buildSQLite(ctx context.Context, path string, format int, from *FileStore) error {
	db, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := prepare(ctx, db, sqlite, format, path); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := importFiles(ctx, tx, sqlite, from); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if err := db.Close(); err != nil {
		return err
	}
	if fi, err := os.Stat(path + "-wal"); err == nil && fi.Size() > 0 {
		return errors.New("the new database's write-ahead log was not checkpointed into it")
	}
	return nil
}

// removeSQLite deletes the database at path and its sidecar files, any
// that exist.
func removeSQLite(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return removeSQLiteSidecars(path)
}

// removeSQLiteSidecars deletes the WAL, shared-memory and journal files
// of the database at path, any that exist.
func removeSQLiteSidecars(path string) error {
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if err := os.Remove(path + suffix); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}
