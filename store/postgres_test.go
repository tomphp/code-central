package store_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/store"
	"github.com/tomoram/code-central/store/storetest"
)

func openPostgres(t *testing.T, url string) *store.SQLStore {
	t.Helper()
	s, err := store.OpenPostgres(url)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// The contract on Postgres: each test in a schema of its own, reopened
// over the same schema, and a second handle on it as another service.
func TestPostgresStoreContract(t *testing.T) {
	storetest.NeedsPostgres(t)
	storetest.Run(t, storetest.Factory{
		New: func(t *testing.T) store.Store { return openPostgres(t, storetest.FreshPostgres(t)) },
		Reopen: func(t *testing.T, s store.Store) store.Store {
			sx := s.(*store.SQLStore)
			url := store.PostgresURLOf(sx)
			if err := sx.Close(); err != nil {
				t.Fatalf("close: %v", err)
			}
			return openPostgres(t, url)
		},
		Second: func(t *testing.T, s store.Store) store.Store {
			return openPostgres(t, store.PostgresURLOf(s.(*store.SQLStore)))
		},
	})
}

func TestPostgresRefusesAnUnknownFormat(t *testing.T) {
	url := storetest.FreshPostgres(t)
	s := openPostgres(t, url)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	store.SetPostgresFormat(t, url, store.Format+1)
	_, err := store.OpenPostgres(url)
	if err == nil || !strings.Contains(err.Error(), "unknown format") || !strings.Contains(err.Error(), s.Path()) {
		t.Fatalf("open at a newer format: %v", err)
	}
}

// A Postgres Store made before the services' tables gains them when it is
// opened, so its services can register.
func TestAPostgresStoreMadeBeforeTheServicesTablesGainsThem(t *testing.T) {
	url := storetest.FreshPostgres(t)
	if err := openPostgres(t, url).Close(); err != nil {
		t.Fatal(err)
	}
	store.DropPostgresServiceTables(t, url)

	mustRegisterAndHoldALease(t, openPostgres(t, url))
}

// A running service whose Store another service migrates past its
// format hears of it on its next renewal, naming both formats, and every
// Update it tries is refused before it writes, so it never writes records
// of a format it doesn't know into the Store.
func TestARunningPostgresServiceStopsWritingOnceAnotherMigratesPastIt(t *testing.T) {
	url := storetest.FreshPostgres(t)
	s := openPostgres(t, url)
	mustUpdate(t, s, func(w store.Writer) error {
		return w.PutProject(domain.Project{Key: "acme", DisplayName: "Acme", Created: t0, Mode: "committed"})
	})
	store.SetPostgresFormat(t, url, store.Format+1)

	err := s.Services().Register(store.Registration{ID: "a", Host: "laptop", Version: "v1", Format: store.Format, Renewed: t0})
	var changed *store.FormatChangedError
	if !errors.As(err, &changed) || changed.Stored != store.Format+1 || changed.Own != store.Format {
		t.Fatalf("renewing: %v", err)
	}
	for _, f := range []int{store.Format, store.Format + 1} {
		if !strings.Contains(err.Error(), fmt.Sprint(f)) {
			t.Fatalf("%q doesn't name format %d", err, f)
		}
	}

	err = s.Update(func(w store.Writer) error {
		return w.PutProject(domain.Project{Key: "acme", DisplayName: "Renamed", Created: t0, Mode: "committed"})
	})
	if !errors.As(err, &changed) {
		t.Fatalf("an update after the migration: %v", err)
	}
	store.SetPostgresFormat(t, url, store.Format)
	if err := s.View(func(r store.Reader) error {
		if p, _ := r.Project("acme"); p.DisplayName != "Acme" {
			t.Fatalf("the refused update wrote: %+v", p)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// Two services opening a fresh database at the same moment create its
// schema in turn, behind the advisory lock: both open, and the second
// finds the first's format row rather than inserting its own.
func TestTwoServicesOpeningAFreshPostgresStoreTakeTurns(t *testing.T) {
	url := storetest.FreshPostgres(t)
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			s, err := store.OpenPostgres(url)
			if err == nil {
				err = s.Close()
			}
			errs <- err
		}()
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("open: %v", err)
		}
	}
	openPostgres(t, url) // and it is at this build's format
}

// Where a Postgres Store is shows as its host, port and database, never
// the credential its URL holds.
func TestAPostgresLocationShowsNoCredential(t *testing.T) {
	type address struct {
		path, host string
		port       int
		database   string
	}
	for url, want := range map[string]address{
		"postgres://alice:s3cret@db.example.com:5433/central?sslmode=require": {"db.example.com:5433/central", "db.example.com", 5433, "central"},
		"postgres://alice:s3cret@db.example.com/central":                      {"db.example.com:5432/central", "db.example.com", 5432, "central"},
		// With no database named, Postgres's is the user's.
		"postgres://central:s3cret@db.example.com": {"db.example.com:5432/central", "db.example.com", 5432, "central"},
	} {
		l, err := store.PostgresLocation(url)
		if err != nil {
			t.Fatalf("%s: %v", url, err)
		}
		if got := (address{l.Path, l.Host, l.Port, l.Database}); l.Kind != store.Postgres || got != want {
			t.Fatalf("%s: kind %s at %+v, want postgres at %+v", url, l.Kind, got, want)
		}
		if got := l.String(); got != "postgres at "+want.path {
			t.Fatalf("shown as %q", got)
		}
		for _, shown := range []string{fmt.Sprint(l), fmt.Sprintf("%v", l), fmt.Sprintf("%+v", l), fmt.Sprintf("%#v", l)} {
			if strings.Contains(shown, "s3cret") || strings.Contains(shown, "alice") {
				t.Fatalf("the credential shows in %q", shown)
			}
		}
	}
}

// An open Postgres Store is where its URL names, and its Location keeps
// the URL's credential unshown, as PostgresLocation's does.
func TestAnOpenPostgresStoreIsAtItsURLsAddress(t *testing.T) {
	url := storetest.FreshPostgres(t)
	want, err := store.PostgresLocation(url)
	if err != nil {
		t.Fatal(err)
	}
	s := openPostgres(t, url)
	if got := s.Location(); got != want {
		t.Fatalf("location %v (%s:%d/%s), want %v", got, got.Host, got.Port, got.Database, want)
	}
}

func TestAPostgresLocationRefusesWhatIsNotAURLWithoutRepeatingThePassword(t *testing.T) {
	_, err := store.PostgresLocation("postgres://alice:s3cret@db.example.com:notaport/central")
	if err == nil {
		t.Fatal("a URL with a bad port was accepted")
	}
	if strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("the error repeats the password: %v", err)
	}
}

// A database that can't be reached fails the open, naming the address and
// nothing of the credential; port 1 is never listening.
func TestOpeningAnUnreachablePostgresFailsNamingTheAddressOnly(t *testing.T) {
	_, err := store.OpenPostgres("postgres://alice:s3cret@127.0.0.1:1/central?connect_timeout=3&sslmode=disable")
	if err == nil {
		t.Fatal("opened a store at a port nothing listens on")
	}
	if !strings.Contains(err.Error(), "127.0.0.1:1/central") {
		t.Fatalf("the error doesn't name the address: %v", err)
	}
	if strings.Contains(err.Error(), "s3cret") || strings.Contains(err.Error(), "alice") {
		t.Fatalf("the error shows the credential: %v", err)
	}
}

// A Location made by hand holds no URL to open: only PostgresLocation's
// does.
func TestOpenAtRefusesAPostgresLocationWithoutItsURL(t *testing.T) {
	_, err := store.OpenAt(store.Location{Kind: store.Postgres, Path: "db.example.com:5432/central"})
	if err == nil || !strings.Contains(err.Error(), "PostgresLocation") {
		t.Fatalf("open: %v, want it to say the Location must come from PostgresLocation", err)
	}
}

// A service whose LISTEN connection drops may have missed Changes while
// it was down: once it listens again it hears one project-wide Change
// per Project, which wakes the Factories, so every waiter refetches. Then
// it hears the other services' Changes again.
func TestAPostgresStoreHearsEveryProjectChangedAfterItsListenConnectionDrops(t *testing.T) {
	url := storetest.FreshPostgres(t)
	a, b := openPostgres(t, url), openPostgres(t, url)
	for _, key := range []string{"acme", "beta"} {
		if err := a.Update(func(w store.Writer) error {
			return w.PutProject(domain.Project{Key: key, DisplayName: key, Mode: "committed"})
		}); err != nil {
			t.Fatal(err)
		}
	}
	heard := storetest.Hear(b)
	store.DropPostgresChanges(t, b)
	got := map[string]store.Change{}
	for range 2 {
		c := storetest.NextHeard(t, heard)
		got[c.Project] = c
	}
	for _, key := range []string{"acme", "beta"} {
		if want := (store.Change{Project: key, Type: events.ProjectChanged, Wakes: true}); got[key] != want {
			t.Fatalf("after the drop, heard %+v for %s, want %+v", got[key], key, want)
		}
	}
	announce(t, a, store.Change{Project: "acme", Type: events.ProjectChanged})
	if c, want := storetest.NextHeard(t, heard), (store.Change{Project: "acme", Type: events.ProjectChanged}); c != want {
		t.Fatalf("after reconnecting, heard %+v, want %+v", c, want)
	}
}

// A LISTEN connection that hears nothing for a while pings the database
// rather than taking the silence for a drop: it hears no project-wide
// Changes, and still hears the other services'.
func TestAQuietPostgresListenConnectionIsKeptAndStillHears(t *testing.T) {
	store.SetListenCheck(t, 50*time.Millisecond)
	url := storetest.FreshPostgres(t)
	a, b := openPostgres(t, url), openPostgres(t, url)
	if err := a.Update(func(w store.Writer) error {
		return w.PutProject(domain.Project{Key: "acme", DisplayName: "acme", Mode: "committed"})
	}); err != nil {
		t.Fatal(err)
	}
	heard := storetest.Hear(b)
	time.Sleep(500 * time.Millisecond) // several checks' worth of silence
	storetest.NothingHeard(t, heard)
	want := store.Change{Project: "acme", Type: events.TicketChanged, ID: "acme-1"}
	announce(t, a, want)
	if c := storetest.NextHeard(t, heard); c != want {
		t.Fatalf("after the silence, heard %+v, want %+v", c, want)
	}
}

// announce commits an Update on s that announces ch and writes nothing.
func announce(t *testing.T, s store.Store, ch store.Change) {
	t.Helper()
	if err := s.Update(func(w store.Writer) error {
		w.Announce(ch)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
