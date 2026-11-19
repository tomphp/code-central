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
	execAll(t, "sqlite", sqliteDSN(path), setFormat(format))
}

// SetPostgresFormat writes format into the Postgres Store at url, as a
// newer or older code-central would have left it.
func SetPostgresFormat(t testing.TB, url string, format int) {
	t.Helper()
	execAll(t, "pgx", url, setFormat(format))
}

// DropSQLiteServiceTables drops the services' tables from the closed
// SQLite Store at path, as a Store made before them would be.
func DropSQLiteServiceTables(t testing.TB, path string) {
	t.Helper()
	execAll(t, "sqlite", sqliteDSN(path), dropServiceTables...)
}

// DropPostgresServiceTables drops the services' tables from the Postgres
// Store at url, as a Store made before them would be.
func DropPostgresServiceTables(t testing.TB, url string) {
	t.Helper()
	execAll(t, "pgx", url, dropServiceTables...)
}

// setFormat is the statement writing format into the meta row.
func setFormat(format int) string {
	return `UPDATE meta SET value = '` + strconv.Itoa(format) + `' WHERE key = 'format'`
}

var dropServiceTables = []string{`DROP TABLE watcher_leases`, `DROP TABLE registrations`}

// execAll runs each statement on the database at dsn, opened with driver.
func execAll(t testing.TB, driver, dsn string, stmts ...string) {
	t.Helper()
	db, err := sql.Open(driver, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range stmts {
		if _, err := db.ExecContext(context.Background(), stmt); err != nil {
			t.Fatal(err)
		}
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

// OpenPostgresFormat opens url as a build whose format is format and
// whose migration chain is steps, so tests can migrate to a test-only
// format.
func OpenPostgresFormat(url string, format int, steps []MigrationStep) (*SQLStore, error) {
	return openPostgres(url, format, steps)
}

// FailPostgresMigrationBeforeCommit makes the next Postgres Migrations
// fail with err once they have rewritten the Store, just before they
// commit.
func FailPostgresMigrationBeforeCommit(t interface{ Cleanup(func()) }, err error) {
	failPostgresMigrationBeforeCommit = err
	t.Cleanup(func() { failPostgresMigrationBeforeCommit = nil })
}
