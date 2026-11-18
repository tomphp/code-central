//go:build !js

package store

import (
	"errors"
	"fmt"
)

// OpenAt opens the Store at l, whatever its Kind.
func OpenAt(l Location) (OpenedStore, error) {
	switch l.Kind {
	case Files:
		return Open(l.Path)
	case SQLite:
		return OpenSQLite(l.Path)
	case Postgres:
		if l.url == "" {
			return nil, errors.New("a postgres Location must come from PostgresLocation, which holds its URL")
		}
		return OpenPostgres(l.url)
	default:
		return nil, fmt.Errorf("unknown store kind %q", l.Kind)
	}
}
