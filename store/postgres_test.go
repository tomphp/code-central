package store_test

import (
	"fmt"
	"strings"
	"testing"

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

// A Postgres Store has no Migrations yet (Ticket code-central-1049): one
// at an older format is refused by name, and left as it is.
func TestPostgresRefusesAnOlderFormatUntilItMigrates(t *testing.T) {
	url := storetest.FreshPostgres(t)
	s := openPostgres(t, url)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	store.SetPostgresFormat(t, url, store.Format-1)
	_, err := store.OpenPostgres(url)
	if err == nil || !strings.Contains(err.Error(), "cannot yet migrate") || !strings.Contains(err.Error(), s.Path()) {
		t.Fatalf("open at an older format: %v", err)
	}
	store.SetPostgresFormat(t, url, store.Format)
	openPostgres(t, url) // the refusal changed nothing
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
	for url, want := range map[string]string{
		"postgres://alice:s3cret@db.example.com:5433/central?sslmode=require": "db.example.com:5433/central",
		"postgres://alice:s3cret@db.example.com/central":                      "db.example.com:5432/central",
	} {
		l, err := store.PostgresLocation(url)
		if err != nil {
			t.Fatalf("%s: %v", url, err)
		}
		if l.Kind != store.Postgres || l.Path != want {
			t.Fatalf("%s: kind %s at %q, want postgres at %q", url, l.Kind, l.Path, want)
		}
		if got := l.String(); got != "postgres at "+want {
			t.Fatalf("shown as %q", got)
		}
		for _, shown := range []string{fmt.Sprint(l), fmt.Sprintf("%v", l), fmt.Sprintf("%+v", l), fmt.Sprintf("%#v", l)} {
			if strings.Contains(shown, "s3cret") || strings.Contains(shown, "alice") {
				t.Fatalf("the credential shows in %q", shown)
			}
		}
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
