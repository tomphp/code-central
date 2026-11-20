package server_test

import (
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/store"
)

// A write whose attempts on a shared Store kept conflicting is refused as
// conflict, saying to try again (Spec code-central-1032).

// conflictingStore fails every Update with store.ErrConflict while on.
type conflictingStore struct {
	store.Store
	on *atomic.Bool
}

func (s conflictingStore) Update(fn func(store.Writer) error) error {
	if s.on.Load() {
		return store.ErrConflict
	}
	return s.Store.Update(fn)
}

// conflictingServer is newServer over a store whose writes conflict once
// the returned switch is on.
func conflictingServer(t *testing.T) (*codecentraltest.Server, *atomic.Bool) {
	t.Helper()
	on := new(atomic.Bool)
	srv := newServerWith(t, []codecentraltest.Option{codecentraltest.WithStore(func(s store.Store) store.Store {
		return conflictingStore{s, on}
	})})
	on.Store(true)
	return srv, on
}

func TestAConflictIsRefusedOverHTTPWith409(t *testing.T) {
	srv, _ := conflictingServer(t)
	status, b := httpDo(t, srv, httpCall{"POST", "/api/p/acme/claim", `{"ticket":"acme-3","session_id":"sess-1"}`})
	e := errorJSON(t, b, api.Conflict)
	if status != http.StatusConflict {
		t.Fatalf("status %d, want 409", status)
	}
	if !strings.Contains(e.Message, "try again") {
		t.Fatalf("message %q, want it to say to try again", e.Message)
	}
}

func TestAConflictIsAnMCPErrorThatSaysToTryAgain(t *testing.T) {
	srv, _ := conflictingServer(t)
	s := connect(t, srv, "acme")
	e := mcpRejected(t, s, "claim", map[string]any{"ticket": "acme-3", "session_id": "sess-1"}, api.Conflict)
	if !strings.Contains(e.Message, "try again") {
		t.Fatalf("message %q, want it to say to try again", e.Message)
	}
}
