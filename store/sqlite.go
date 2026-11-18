//go:build !js

// The SQLite Store Kind: one database file on modernc.org/sqlite (pure
// Go), in WAL mode, with an exclusive lock file beside it, as the File
// Store has. It isn't for a network share: neither SQLite's locking nor
// the lock file's holds there.

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// sqlite is SQLite's dialect: documents are TEXT, and the placeholders
// are the ? the statements are written with.
var sqlite = dialect{
	json:   "TEXT",
	serial: "INTEGER PRIMARY KEY AUTOINCREMENT",
	rebind: func(q string) string { return q },
	begin:  "BEGIN IMMEDIATE",
}

// OpenSQLite locks and opens the SQLite Store at path, creating the
// database, its folder and its lock file (<path>.lock) if needed. A
// database at an older format is migrated (migrateSQLite), and one at a
// newer format is refused by name.
func OpenSQLite(path string) (*SQLStore, error) { return openSQLite(path, Format, migrations) }

func openSQLite(path string, format int, steps []MigrationStep) (*SQLStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	lock, err := lockFile(path+".lock", path)
	if err != nil {
		return nil, err
	}
	s := &SQLStore{d: sqlite, path: path, lock: lock}
	err = s.connect()
	var older int
	if err == nil {
		older, err = prepare(context.Background(), s.db, sqlite, format, path)
	}
	if err == nil && older > 0 {
		err = s.migrate(older, format, steps)
	}
	if err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// connect opens the database at s.path.
func (s *SQLStore) connect() error {
	db, err := sql.Open("sqlite", sqliteDSN(s.path))
	if err != nil {
		return err
	}
	// Every connection has the same page cache; a handful serves the
	// readers while one Update runs.
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	s.db = db
	s.usage = &sqlUsage{db: db, d: sqlite}
	return nil
}

// Migrating a SQLite Store (Spec code-central-1032) runs the File Store's
// chain of steps: the database is exported as a File Store layout into a
// temporary folder, the File Store migrates that, and the result is
// imported into a new database file beside the old one. Only then is the
// old database copied to <path>.format-<n>.bak by VACUUM INTO and the new
// one renamed over it. A failure before the rename deletes the new file
// and the backup, and leaves the original untouched. Backups are never
// deleted after.

// failSQLiteMigrationBeforeSwap, set only by tests, fails a Migration
// once its backup is written, just before the swap.
var failSQLiteMigrationBeforeSwap error

// sqliteBackupFor is where the database at path is kept after migrating
// from format n.
func sqliteBackupFor(path string, n int) string { return fmt.Sprintf("%s.format-%d.bak", path, n) }

// migrate brings the open database from format from up to format, and
// leaves s open on the migrated database.
func (s *SQLStore) migrate(from, format int, steps []MigrationStep) error {
	ctx := context.Background()
	if _, err := migrationChain(from, format, steps); err != nil {
		return fmt.Errorf("%s: %w", s.path, err)
	}
	bak := sqliteBackupFor(s.path, from)
	if _, err := os.Lstat(bak); !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("cannot migrate %s from format %d: the backup %s already exists; move it aside and start again (the store is untouched)", s.path, from, bak)
	}
	next := filepath.Join(filepath.Dir(s.path), "."+filepath.Base(s.path)+".migrating")
	if err := removeSQLite(next); err != nil { // left by a Migration that never finished
		return err
	}
	stage, err := os.MkdirTemp("", "code-central-migrating-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	swapped, backedUp := false, false
	defer func() {
		if swapped {
			return
		}
		removeSQLite(next)
		if backedUp {
			os.Remove(bak)
		}
	}()

	files := filepath.Join(stage, "store")
	if err := exportFiles(ctx, s.db, s.d, files, from); err != nil {
		return fmt.Errorf("migrating %s from format %d: staging it as files: %w (the store is untouched)", s.path, from, err)
	}
	staged, err := open(files, format, steps)
	if err != nil {
		return fmt.Errorf("%s: %w", s.path, err)
	}
	err = buildSQLite(ctx, next, format, staged)
	staged.Close()
	if err != nil {
		return fmt.Errorf("migrating %s from format %d: loading the migrated records: %w (the store is untouched)", s.path, from, err)
	}

	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, bak); err != nil {
		return fmt.Errorf("migrating %s from format %d: backing it up to %s: %w (the store is untouched)", s.path, from, bak, err)
	}
	backedUp = true
	if failSQLiteMigrationBeforeSwap != nil {
		return failSQLiteMigrationBeforeSwap
	}
	// Closed, the old database has checkpointed its WAL into the file, so
	// none of it can be mistaken for the new database's.
	if err := s.db.Close(); err != nil {
		return err
	}
	s.db = nil
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
// record and Usage record of from, and closes it.
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
	return db.Close()
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

// sqliteDSN is the file: URI that opens path in WAL mode, waiting up to
// five seconds for a busy database (a Usage append during an Update) and
// flushing every commit to the disk, as the File Store's writes are.
func sqliteDSN(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	p := filepath.ToSlash(abs)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p // a Windows drive path, as file:///C:/...
	}
	q := url.Values{}
	for _, pragma := range []string{"busy_timeout(5000)", "journal_mode(WAL)", "synchronous(FULL)"} {
		q.Add("_pragma", pragma)
	}
	u := url.URL{Scheme: "file", Path: p, RawQuery: q.Encode()}
	return u.String()
}
