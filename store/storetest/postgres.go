//go:build !js

// The test Postgres (Spec code-central-1032): CODE_CENTRAL_TEST_POSTGRES_URL
// names a database the suites may fill and empty, which CI runs as a
// service container. Without it the Postgres cases skip, saying so, so
// make test needs no Postgres.

package storetest

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// PostgresURLEnv is the variable that names the test Postgres.
const PostgresURLEnv = "CODE_CENTRAL_TEST_POSTGRES_URL"

// NeedsPostgres is the test Postgres's URL. With none set it skips t,
// naming the variable.
func NeedsPostgres(t testing.TB) string {
	t.Helper()
	u := os.Getenv(PostgresURLEnv)
	if u == "" {
		t.Skipf("%s is not set: no Postgres to run against", PostgresURLEnv)
	}
	return u
}

// FreshPostgres is the URL of a schema of its own, empty, in the test
// Postgres, dropped with everything in it when t ends, so each test
// starts from nothing and leaves nothing. With no test Postgres it skips
// t, as NeedsPostgres does.
func FreshPostgres(t testing.TB) string {
	t.Helper()
	base := NeedsPostgres(t)
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("%s is not a URL: %v", PostgresURLEnv, err)
	}
	db, err := sql.Open("pgx", base)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var b [6]byte
	rand.Read(b[:])
	schema := "test_" + hex.EncodeToString(b[:])
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		if _, err := db.ExecContext(ctx, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("drop schema %s: %v", schema, err)
		}
	})
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String()
}
