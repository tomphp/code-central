package store_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestSQLiteMakesTheFolderAndTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "code-central.db")
	openSQLite(t, path)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("no database file: %v", err)
	}
}
