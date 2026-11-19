package store

import "fmt"

// Kind is a Store Kind: how a Store keeps its records (ADR 0067).
type Kind string

// The Store Kinds this code-central opens.
const (
	// Files is the File Store: YAML files under one folder, the default.
	Files Kind = "files"
	// SQLite is one SQLite database file, on this machine's disk.
	SQLite Kind = "sqlite"
	// Postgres is a Postgres database, which several services may share
	// (ADR 0068).
	Postgres Kind = "postgres"
)

// Memory is the in-memory fake's Kind, which tests and the UI's
// WebAssembly fake run on (ADR 0060). It keeps nothing, and no setting
// opens it.
const Memory Kind = "memory"

// Location names a Store to open: its Kind and where it is. It shows no
// credential, so it is safe to print.
type Location struct {
	Kind Kind
	// Path is the File Store's folder, the SQLite database file, or the
	// Postgres Store's address (host:port/database).
	Path string
	// Host, Port and Database are the Postgres Store's address in parts,
	// as its URL names them: zero for the other Kinds.
	Host     string
	Port     int
	Database string
	// url is the Postgres Store's URL, with its credential, which only
	// OpenAt reads: PostgresLocation sets it, and nothing shows it.
	url string
}

// String is the Location as the startup line shows it: "files at /path".
func (l Location) String() string { return fmt.Sprintf("%s at %s", l.Kind, l.Path) }

// GoString is the same, so %#v shows no more than %v does.
func (l Location) GoString() string { return l.String() }

// OpenedStore is a Store some Open has opened and holds until Close:
// what serve runs on, whatever its Kind.
type OpenedStore interface {
	Store
	// Close lets the Store go, with its lock.
	Close() error
	// Migrated is the Migration the open ran, or nil for none.
	Migrated() *Migration
}
