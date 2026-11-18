//go:build !js

package store

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
)

// SetSQLiteFormat writes format into the closed SQLite Store at path, as
// a newer or older code-central would have left it.
func SetSQLiteFormat(t testing.TB, path string, format int) {
	t.Helper()
	db, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(context.Background(), `UPDATE meta SET value = ? WHERE key = 'format'`, strconv.Itoa(format)); err != nil {
		t.Fatal(err)
	}
}
