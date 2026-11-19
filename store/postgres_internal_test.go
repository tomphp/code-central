//go:build !js

package store

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestPostgresPlaceholdersAreNumberedOutsideQuotedText(t *testing.T) {
	got := postgres.rebind(`INSERT INTO meta (key, value) VALUES ('format?', ?) ON CONFLICT (key) DO UPDATE SET value = ?`)
	want := `INSERT INTO meta (key, value) VALUES ('format?', $1) ON CONFLICT (key) DO UPDATE SET value = $2`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

// A serialization failure or a deadlock is worth another attempt; any
// other failure, a unique violation among them, is not.
func TestAPostgresUpdateRetriesSerializationFailuresAndDeadlocksOnly(t *testing.T) {
	for code, want := range map[string]bool{"40001": true, "40P01": true, "23505": false, "42P01": false} {
		err := fmt.Errorf("wrapped: %w", &pgconn.PgError{Code: code})
		if got := postgres.retryable(err); got != want {
			t.Errorf("SQLSTATE %s: retryable %v, want %v", code, got, want)
		}
	}
	if postgres.retryable(errors.New("not from Postgres")) {
		t.Error("an error that isn't Postgres's counts as retryable")
	}
}
