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
)

// Location names a Store to open: its Kind and where it is. It names no
// credential, so it is safe to print.
type Location struct {
	Kind Kind
	// Path is the File Store's folder, or the SQLite database file.
	Path string
}

// String is the Location as the startup line shows it: "files at /path".
func (l Location) String() string { return fmt.Sprintf("%s at %s", l.Kind, l.Path) }

// OpenedStore is a Store some Open has opened and holds until Close:
// what serve runs on, whatever its Kind.
type OpenedStore interface {
	Store
	// Close lets the Store go, with its lock.
	Close() error
	// Migrated is the Migration the open ran, or nil for none.
	Migrated() *Migration
}
