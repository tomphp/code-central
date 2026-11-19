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
	size:        sqliteSize,
	json:        "TEXT",
	serial:      "INTEGER PRIMARY KEY AUTOINCREMENT",
	rebind:      func(q string) string { return q },
	begin:       "BEGIN IMMEDIATE",
	schemaBegin: "BEGIN",
}

// sqliteSize is the database file and its WAL, as the file system has
// them: a WAL not yet checkpointed holds the latest writes. A WAL that
// isn't there counts for nothing.
func sqliteSize(s *SQLStore) (int64, error) {
	var size int64
	for _, name := range []string{s.path, s.path + "-wal"} {
		info, err := os.Stat(name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return 0, err
		}
		size += info.Size()
	}
	return size, nil
}

// OpenSQLite locks and opens the SQLite Store at path, creating the
// database, its folder and its lock file (<path>.lock) if needed. A
// database at an older format is migrated (sqlite_migrate.go), and one at
// a newer format is refused by name.
func OpenSQLite(path string) (*SQLStore, error) { return openSQLite(path, Format, migrations) }

// openSQLite is OpenSQLite as a build whose format is format and whose
// migration chain is steps: Format and migrations, except in tests.
func openSQLite(path string, format int, steps []MigrationStep) (*SQLStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	lock, err := lockFile(path+".lock", path)
	if err != nil {
		return nil, err
	}
	s := &SQLStore{d: sqlite, path: path, location: Location{Kind: SQLite, Path: path}, format: format, lock: lock}
	err = s.connect()
	var older int
	if err == nil {
		older, err = prepare(context.Background(), s.db, sqlite, format, path)
	}
	if err == nil && older > 0 {
		err = s.migrate(older, format, steps)
	}
	if err == nil {
		// Under the lock no other service is running: a Registration or a
		// Lease here is from one that stopped without deregistering.
		_, err = s.db.Exec(`DELETE FROM registrations; DELETE FROM watcher_leases`)
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
