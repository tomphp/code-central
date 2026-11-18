//go:build !js

package store

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
	"time"
)

// SetSQLiteFormat writes format into the closed SQLite Store at path, as
// a newer or older code-central would have left it.
func SetSQLiteFormat(t testing.TB, path string, format int) {
	t.Helper()
	setFormat(t, "sqlite", sqliteDSN(path), sqlite, format)
}

// SetPostgresFormat writes format into the Postgres Store at url, as a
// newer or older code-central would have left it.
func SetPostgresFormat(t testing.TB, url string, format int) {
	t.Helper()
	setFormat(t, "pgx", url, postgres, format)
}

// setFormat writes format into the meta row of the database at dsn,
// opened with driver, in dialect d's placeholders.
func setFormat(t testing.TB, driver, dsn string, d dialect, format int) {
	t.Helper()
	db, err := sql.Open(driver, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(context.Background(), d.rebind(`UPDATE meta SET value = ? WHERE key = 'format'`), strconv.Itoa(format)); err != nil {
		t.Fatal(err)
	}
}

// PostgresURLOf is the URL s was opened with, for a test to open the same
// schema again.
func PostgresURLOf(s *SQLStore) string { return s.url }

// DropPostgresChanges ends the connection s listens for Changes on, from
// the database's side, as a restart or a network failure would.
func DropPostgresChanges(t testing.TB, s *SQLStore) {
	t.Helper()
	db, err := sql.Open("pgx", s.url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var dropped int
	q := `SELECT count(pg_terminate_backend(pid)) FROM pg_stat_activity WHERE application_name = $1`
	if err := db.QueryRowContext(context.Background(), q, changesApplication(s.service)).Scan(&dropped); err != nil {
		t.Fatal(err)
	}
	if dropped == 0 {
		t.Fatal("no connection listening for Changes")
	}
}

// SetListenCheck makes a LISTEN connection ping the database after d
// without a notice, until t ends.
func SetListenCheck(t testing.TB, d time.Duration) {
	was := listenCheck
	listenCheck = d
	t.Cleanup(func() { listenCheck = was })
}
