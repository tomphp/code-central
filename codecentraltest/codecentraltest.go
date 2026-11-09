// Package codecentraltest starts a real Code Central for tests: the real
// core, the in-memory store and a fake clock behind the real front doors
// (HTTP API and MCP, behind the shared middleware), on
// an httptest port. Go clients of the HTTP API test against it rather
// than a hand-written fake.
package codecentraltest

import (
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/frontdoor"
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

// Version is the service version a test server reports unless
// WithVersion says otherwise; it matches an unstamped CLI's.
const Version = "dev"

// Option configures NewServer.
type Option func(*options)

type options struct {
	version string
	log     io.Writer
}

// WithVersion makes the server report version in its version header.
func WithVersion(version string) Option { return func(o *options) { o.version = version } }

// WithLog makes the server write its log, as serve's --log holds it, to
// log.
func WithLog(log io.Writer) Option { return func(o *options) { o.log = log } }

// NewServer starts a test service, stopped when t ends.
func NewServer(t testing.TB, opts ...Option) *Server {
	t.Helper()
	o := options{version: Version}
	for _, opt := range opts {
		opt(&o)
	}
	s := store.NewMemory()
	clk := clock.NewFake(Start)
	c := core.New(s, clk, &ids.Sequence{})
	hs := httptest.NewServer(frontdoor.Versioned(frontdoor.Handler(c, web.Handler(), o.log), o.version))
	t.Cleanup(hs.Close)
	return &Server{URL: hs.URL, Client: client.New(hs.URL), Clock: clk, Core: c, Store: s}
}
