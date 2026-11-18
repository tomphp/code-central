package store_test

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
	"github.com/tomoram/code-central/store/storetest"
)

// These tests are the Migration chain over a Postgres Store (Spec
// code-central-1032, seam 3): the Store is staged as a File Store layout,
// the chain runs over it, and the result is loaded back, all in one
// transaction, and only while no other service is registered on it.

func openPostgresFormat(t *testing.T, url string, format int, steps ...store.MigrationStep) *store.SQLStore {
	t.Helper()
	s, err := store.OpenPostgresFormat(url, format, steps)
	if err != nil {
		t.Fatalf("open as format %d: %v", format, err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// seedPostgresFormat1 is the URL of a format-1 Postgres Store with a
// Project, a Spec and a Ticket.
func seedPostgresFormat1(t *testing.T) string {
	t.Helper()
	url := storetest.FreshPostgres(t)
	s := openPostgresFormat(t, url, 1)
	mustUpdate(t, s, func(w store.Writer) error {
		return w.PutProject(domain.Project{Key: "acme", DisplayName: "acme", Created: t0, Mode: "committed"})
	})
	mustUpdate(t, s, func(w store.Writer) error {
		return w.PutSpec("acme", domain.Spec{Number: 1, Title: "Login", Created: t0, Updated: t0, Body: "# Login\n"})
	})
	mustUpdate(t, s, func(w store.Writer) error {
		return w.PutTicket("acme", domain.Ticket{Number: 2, Title: "Form", State: domain.Ready, Spec: 1, Created: t0, Updated: t0,
			Moves: []domain.Move{{From: domain.Draft, To: domain.Ready, Actor: "agent", Time: t0}}}, store.Moves)
	})
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	return url
}

func TestAnOlderPostgresStoreIsMigratedThroughTheChain(t *testing.T) {
	url := seedPostgresFormat1(t)

	s := openPostgresFormat(t, url, 2, renaming(1))

	if m := s.Migrated(); m == nil || *m != (store.Migration{From: 1, To: 2}) {
		t.Fatalf("Migrated() = %+v, want from 1 to 2 with no backup of its own", m)
	}
	if got := ticketTitle(t, s, 2); got != "Form (migrated)" {
		t.Fatalf("ticket 2's title = %q: the chain did not run", got)
	}
	mustUpdate(t, s, func(w store.Writer) error {
		return w.PutTicket("acme", domain.Ticket{Number: 3, Title: "More", State: domain.Draft, Spec: 1, Created: t0, Updated: t0})
	})
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	again := openPostgresFormat(t, url, 2)
	if m := again.Migrated(); m != nil {
		t.Fatalf("reopening at format 2 migrated again: %+v", m)
	}
	if got := ticketTitle(t, again, 3); got != "More" {
		t.Fatalf("ticket 3 after reopening = %q", got)
	}
}

// The real chain migrates a format-46 Postgres Store to the records the
// same fixture migrates to as a File Store, Usage included.
func TestTheRealChainMigratesAFormat46PostgresStoreAsItDoesTheFileStore(t *testing.T) {
	url := storetest.FreshPostgres(t)
	files := filepath.Join(t.TempDir(), "code-central")
	writeFormat46(t, files)
	fixture := filepath.Join(t.TempDir(), "fixture")
	writeFormat46(t, fixture)
	store.LoadPostgresFixture(t, fixture, url)

	want := openFile(t, files)
	s := openPostgres(t, url)

	if m := s.Migrated(); m == nil || *m != (store.Migration{From: 46, To: store.Format}) {
		t.Fatalf("Migrated() = %+v", m)
	}
	if got, w := dump(t, s), dump(t, want); got != w {
		t.Fatalf("the migrated Postgres Store holds\n%s\nwant, as the File Store migrated\n%s", got, w)
	}
	days := []string{"2026-01-03"}
	got, err := s.Usage().Read("", days)
	if err != nil {
		t.Fatal(err)
	}
	w, err := want.Usage().Read("", days)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Calls) != 2 || len(got.Calls) != len(w.Calls) || got.Calls[1].Error != w.Calls[1].Error {
		t.Fatalf("usage after migrating = %+v, want %+v", got, w)
	}
}

// A Migration refuses while another service is registered on the Store,
// naming it, and leaves the Store as it was; one that stopped without
// deregistering stops counting once its Registration has lapsed.
func TestAPostgresMigrationRefusesWhileAnotherServiceIsRegistered(t *testing.T) {
	url := seedPostgresFormat1(t)
	other := openPostgresFormat(t, url, 1)
	before := dump(t, other)
	reg := store.Registration{ID: "other", Host: "desktop", Version: "v1.0.0", Format: 1, Renewed: time.Now()}
	if err := other.Services().Register(reg); err != nil {
		t.Fatal(err)
	}

	_, err := store.OpenPostgresFormat(url, 2, []store.MigrationStep{renaming(1)})
	if err == nil || !strings.Contains(err.Error(), "desktop (v1.0.0)") || !strings.Contains(err.Error(), "the store is untouched") {
		t.Fatalf("open while another service is registered: %v, want a refusal naming it", err)
	}
	if got := dump(t, other); got != before {
		t.Fatalf("the refused Migration changed the store:\n%s\nwant\n%s", got, before)
	}

	// It stopped without deregistering, and its Registration lapsed.
	reg.Renewed = time.Now().Add(-store.ServiceLife)
	if err := other.Services().Register(reg); err != nil {
		t.Fatal(err)
	}
	if got := ticketTitle(t, openPostgresFormat(t, url, 2, renaming(1)), 2); got != "Form (migrated)" {
		t.Fatalf("ticket 2's title = %q: the chain did not run", got)
	}
}

// Two services at the new format opening an older Store at once migrate
// it once: the second waits for the first and finds it migrated, its
// records all there.
func TestTwoServicesOpeningAnOlderPostgresStoreAtOnceMigrateItOnce(t *testing.T) {
	url := seedPostgresFormat1(t)
	before := dump(t, openPostgresFormat(t, url, 1))

	opened := make([]*store.SQLStore, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range opened {
		wg.Go(func() { opened[i], errs[i] = store.OpenPostgresFormat(url, 2, []store.MigrationStep{renaming(1)}) })
	}
	wg.Wait()

	migrated := 0
	for i, s := range opened {
		if errs[i] != nil {
			t.Fatalf("open %d: %v", i, errs[i])
		}
		t.Cleanup(func() { s.Close() })
		if s.Migrated() != nil {
			migrated++
		}
	}
	if migrated != 1 {
		t.Fatalf("%d of the two opens migrated the store, want one", migrated)
	}
	want := strings.Replace(before, "Title:Form ", "Title:Form (migrated) ", 1)
	if got := dump(t, opened[0]); got != want {
		t.Fatalf("after both opens the store holds\n%s\nwant\n%s", got, want)
	}
}

// A Migration that fails, wherever it fails, even after it has rewritten
// every record, leaves the Store as it was, at its format: it is one
// transaction.
func TestAFailedPostgresMigrationLeavesTheStoreUntouched(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T)
		steps []store.MigrationStep
		want  string
	}{
		{name: "a step fails", steps: []store.MigrationStep{{From: 1, Name: "break tickets", File: func(rel string, data []byte) ([]byte, error) {
			if strings.Contains(rel, "/tickets/") {
				return nil, errors.New("cannot rewrite")
			}
			return data, nil
		}}}, want: `step "break tickets" failed on projects/acme/tickets/2.yaml`},
		{name: "no step from the format", want: "no migration from format 1"},
		{name: "the commit fails", setup: func(t *testing.T) { store.FailPostgresMigrationBeforeCommit(t, errors.New("connection lost")) },
			steps: []store.MigrationStep{renaming(1)}, want: "connection lost"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			url := seedPostgresFormat1(t)
			old := openPostgresFormat(t, url, 1)
			before := dump(t, old)
			old.Close()
			if c.setup != nil {
				c.setup(t)
			}

			_, err := store.OpenPostgresFormat(url, 2, c.steps)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("open: %v, want an error naming %q", err, c.want)
			}
			if got := dump(t, openPostgresFormat(t, url, 1)); got != before {
				t.Fatalf("the store changed:\n%s\nwant\n%s", got, before)
			}
		})
	}
}
