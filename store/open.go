//go:build !js

package store

import "fmt"

// OpenAt opens the Store at l, whatever its Kind.
func OpenAt(l Location) (OpenedStore, error) {
	switch l.Kind {
	case Files:
		return Open(l.Path)
	case SQLite:
		return OpenSQLite(l.Path)
	default:
		return nil, fmt.Errorf("unknown store kind %q", l.Kind)
	}
}
