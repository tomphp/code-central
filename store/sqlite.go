//go:build !js

// The SQLite Store Kind: one database file on modernc.org/sqlite (pure
// Go), in WAL mode, with an exclusive lock file beside it, as the File
// Store has. It isn't for a network share: SQLite's locking doesn't hold
// there, and the lock file is beside the database, not on the share's
// terms.

package store

import (
	"context"
	"database/sql"
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
}

// OpenSQLite locks and opens the SQLite Store at path, creating the
// database, its folder and its lock file (<path>.lock) if needed. A
// database at another format is refused by name.
func OpenSQLite(path string) (*SQLStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	lock, err := lockFile(path+".lock", path)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		lock.release()
		return nil, err
	}
	// Every connection has the same page cache; a handful serves the
	// readers while one Update runs.
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	s := &SQLStore{db: db, d: sqlite, path: path, lock: lock}
	s.usage = &sqlUsage{db: db, d: sqlite}
	if err := prepare(context.Background(), db, sqlite, Format, path); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
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
