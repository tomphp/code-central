package app_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/app"
	"github.com/tomoram/code-central/store"
	"github.com/tomoram/code-central/store/storetest"
)

// A service on a Shared Store (ADR 0068) registers itself, as its host and
// version, while it runs, and deletes its Registration as it stops.

func postgresAt(t *testing.T, url string) store.Location {
	t.Helper()
	l, err := store.PostgresLocation(url)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// registrations is every live Registration on the Postgres Store at url.
func registrations(t *testing.T, url string) []store.Registration {
	t.Helper()
	s, err := store.OpenPostgres(url)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	regs, err := s.Services().Registrations(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return regs
}

func TestAServiceIsRegisteredOnItsPostgresStoreWhileItRuns(t *testing.T) {
	url := storetest.FreshPostgres(t)
	r, err := start(t, app.Options{Store: postgresAt(t, url), Version: "v9.9.9"})
	if err != nil {
		t.Fatal(err)
	}
	host, _ := os.Hostname()
	if regs := registrations(t, url); len(regs) != 1 || regs[0].Host != host || regs[0].Version != "v9.9.9" || regs[0].Format != store.Format {
		t.Fatalf("registrations while it runs: %+v, want this service's, on %s", regs, host)
	}
	r.stop(t)
	if regs := registrations(t, url); len(regs) != 0 {
		t.Fatalf("registrations once it stopped: %+v, want none", regs)
	}
}

// A running service whose Store another service migrates past its format
// stops at its next renewal, naming both formats; started again, it
// refuses the Store.
func TestAServiceStopsOnceAnotherMigratesItsPostgresStorePastItsFormat(t *testing.T) {
	url := storetest.FreshPostgres(t)
	app.RenewEvery(t, 20*time.Millisecond)
	r, err := start(t, app.Options{Store: postgresAt(t, url)})
	if err != nil {
		t.Fatal(err)
	}
	setFormat(t, url, store.Format+1)

	select {
	case err = <-r.done:
		r.cancel()
		r.cancel = nil
	case <-time.After(10 * time.Second):
		t.Fatal("the service went on running on a store migrated past its format")
	}
	var changed *store.FormatChangedError
	if !errors.As(err, &changed) {
		t.Fatalf("the service stopped with %v, want the store's format changed", err)
	}
	for _, f := range []int{store.Format, store.Format + 1} {
		if !strings.Contains(err.Error(), fmt.Sprint(f)) {
			t.Fatalf("%q doesn't name format %d", err, f)
		}
	}

	_, err = start(t, app.Options{Store: postgresAt(t, url)})
	if err == nil || !strings.Contains(err.Error(), "unknown format") {
		t.Fatalf("starting on a store at a newer format: %v, want a refusal", err)
	}
}

// setFormat writes format into the Postgres Store at url, as a newer
// code-central migrating it would.
func setFormat(t *testing.T, url string, format int) {
	t.Helper()
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(context.Background(), `UPDATE meta SET value = $1 WHERE key = 'format'`, strconv.Itoa(format)); err != nil {
		t.Fatal(err)
	}
}
