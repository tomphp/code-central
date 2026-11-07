// Package codecentraltest starts a real Code Central for tests: the real
// core, the in-memory store and a fake clock behind the real httpapi, on
// an httptest port. Go clients of the HTTP API test against it rather
// than a hand-written fake.
package codecentraltest

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/httpapi"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/store"
	"github.com/tomoram/code-central/web"
)

// Start is the fake clock's initial time.
var Start = time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)

// Server is a running test service.
type Server struct {
	URL    string
	Client *client.Client
	Clock  *clock.Fake
	Core   *core.Core
	Store  store.Store
}

// NewServer starts a test service, stopped when t ends.
func NewServer(t testing.TB) *Server {
	t.Helper()
	s := store.NewMemory()
	clk := clock.NewFake(Start)
	c := core.New(s, clk, &ids.Sequence{})
	hs := httptest.NewServer(httpapi.NewHandler(c, web.Handler()))
	t.Cleanup(hs.Close)
	return &Server{URL: hs.URL, Client: client.New(hs.URL), Clock: clk, Core: c, Store: s}
}
