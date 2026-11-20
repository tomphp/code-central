package store_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
	"github.com/tomoram/code-central/store/storetest"
)

func openSQLite(t *testing.T, path string) *store.SQLStore {
	t.Helper()
	s, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSQLiteStoreContract(t *testing.T) {
	storetest.Run(t, storetest.Factory{
		New: func(t *testing.T) store.Store { return openSQLite(t, filepath.Join(t.TempDir(), "code-central.db")) },
		Reopen: func(t *testing.T, s store.Store) store.Store {
			sx := s.(*store.SQLStore)
			if err := sx.Close(); err != nil {
				t.Fatalf("close: %v", err)
			}
			return openSQLite(t, sx.Path())
		},
	})
}

func TestSQLiteIsLockedWhileOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "code-central.db")
	openSQLite(t, path)
	_, err := store.OpenSQLite(path)
	if !errors.Is(err, store.ErrLocked) {
		t.Fatalf("second open: %v, want ErrLocked", err)
	}
}

func TestSQLiteRefusesAnUnknownFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "code-central.db")
	s := openSQLite(t, path)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	store.SetSQLiteFormat(t, path, store.Format+1)
	_, err := store.OpenSQLite(path)
	if err == nil || !strings.Contains(err.Error(), "unknown format") || !strings.Contains(err.Error(), "code-central.db") {
		t.Fatalf("open at a newer format: %v", err)
	}
}

// A SQLite Store made before the services' tables gains them when it is
// opened, so its service can register and keep its runtime state.
func TestASQLiteStoreMadeBeforeTheServicesTablesGainsThem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "code-central.db")
	if err := openSQLite(t, path).Close(); err != nil {
		t.Fatal(err)
	}
	store.DropSQLiteServiceTables(t, path)

	mustUseTheServicesTables(t, openSQLite(t, path))
}

// mustUseTheServicesTables registers a service on s, has it hold acme's
// Watcher Lease and writes and reads each kind of runtime state.
func mustUseTheServicesTables(t *testing.T, s store.Store) {
	t.Helper()
	v := s.Services()
	if err := v.Register(store.Registration{ID: "a", Host: "laptop", Version: "v1", Format: store.Format, Renewed: t0}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if l, err := v.HoldLease("acme", "a", t0); err != nil || l.Holder != "a" {
		t.Fatalf("hold acme's Watcher Lease: %+v, %v", l, err)
	}
	if err := v.PutHeartbeat(store.Heartbeat{Project: "acme", Factory: "factory", FactoryHost: "laptop", Doc: []byte(`{}`), Service: "a", ServiceHost: "laptop", Expires: t0}); err != nil {
		t.Fatalf("put a heartbeat: %v", err)
	}
	if hs, err := v.Heartbeats(); err != nil || len(hs) != 1 {
		t.Fatalf("heartbeats: %+v, %v", hs, err)
	}
	if err := v.PutFactoryReports(store.FactoryReports{Project: "acme", Doc: []byte(`{}`)}); err != nil {
		t.Fatalf("put factory reports: %v", err)
	}
	if rs, err := v.FactoryReports(); err != nil || len(rs) != 1 {
		t.Fatalf("factory reports: %+v, %v", rs, err)
	}
	if err := v.PutPing(store.Ping{Session: "s1", At: t0, Service: "a", ServiceHost: "laptop"}); err != nil {
		t.Fatalf("put a ping: %v", err)
	}
	if ps, err := v.Pings(); err != nil || len(ps) != 1 {
		t.Fatalf("pings: %+v, %v", ps, err)
	}
	if err := v.PutTranscriptHolder(store.TranscriptHolder{Project: "acme", Ticket: 2, Claim: 1, Service: "a", ServiceHost: "laptop", At: t0}); err != nil {
		t.Fatalf("put a transcript holder: %v", err)
	}
	if _, ok, err := v.TranscriptHolder("acme", 2, 1); err != nil || !ok {
		t.Fatalf("transcript holder: %v, %v", ok, err)
	}
}

// A SQLite Store is its database file, and its size is the file and its
// WAL, which hold every write.
func TestASQLiteStoreIsItsFileAndItsSizeTheFileAndItsWAL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "code-central.db")
	s := openSQLite(t, path)
	if got, want := s.Location(), (store.Location{Kind: store.SQLite, Path: path}); got != want {
		t.Fatalf("location %v, want %v", got, want)
	}
	mustUpdate(t, s, func(w store.Writer) error {
		return w.PutProject(domain.Project{Key: "acme", DisplayName: "Acme", Created: t0, Mode: "committed"})
	})
	var want int64
	for _, name := range []string{path, path + "-wal"} {
		info, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		want += info.Size()
	}
	if got, err := s.Size(); err != nil || got != want {
		t.Fatalf("size %d, %v; want the file and its WAL, %d", got, err, want)
	}
}

func TestSQLiteMakesTheFolderAndTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "code-central.db")
	openSQLite(t, path)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("no database file: %v", err)
	}
}

// A record a Store couldn't give back, such as a Ticket whose Link has
// no external state, is refused at the write, not found at the read.
func TestSQLiteRefusesARecordItCouldNotReadBack(t *testing.T) {
	s := openSQLite(t, filepath.Join(t.TempDir(), "code-central.db"))
	if err := s.Update(func(w store.Writer) error { return w.PutProject(domain.Project{Key: "acme"}) }); err != nil {
		t.Fatal(err)
	}
	err := s.Update(func(w store.Writer) error {
		return w.PutTicket("acme", domain.Ticket{Number: 2, Title: "T", State: domain.Draft, Spec: 1, Link: domain.Link{Integration: "github", ID: "1"}})
	})
	if err == nil || !strings.Contains(err.Error(), "link.external_state") {
		t.Fatalf("write: %v, want it refused naming link.external_state", err)
	}
	if err := s.View(func(r store.Reader) error {
		if _, ok := r.Ticket("acme", 2); ok {
			t.Error("the Ticket was written")
		}
		return nil
	}); err != nil {
		t.Fatalf("a later read fails: %v", err)
	}
}
