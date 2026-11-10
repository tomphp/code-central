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
	"github.com/tomoram/code-central/usage"
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
	// UsageDir is the folder the usage recorder keeps its files under,
	// as the store's directory holds them in the real service.
	UsageDir string
}

// Version is the service version a test server reports unless
// WithVersion says otherwise; it matches an unstamped CLI's.
const Version = "dev"

// Option configures NewServer.
type Option func(*options)

type options struct {
	version     string
	log         io.Writer
	usageDir    string
	location    *time.Location
	start       time.Time
	ciCheck     core.CICheck
	issuesCheck core.IssuesCheck
}

// WithVersion makes the server report version in its version header.
func WithVersion(version string) Option { return func(o *options) { o.version = version } }

// WithLog makes the server write its log, as serve's --log holds it, to
// log.
func WithLog(log io.Writer) Option { return func(o *options) { o.log = log } }

// WithUsageDir keeps Usage under dir instead of a new temporary folder,
// so a second server started on it reads the first one's records, as a
// restarted service would.
func WithUsageDir(dir string) Option { return func(o *options) { o.usageDir = dir } }

// WithLocation makes the server's local time zone loc, which dates Usage
// by day; it is UTC unless given.
func WithLocation(loc *time.Location) Option { return func(o *options) { o.location = loc } }

// WithStart starts the fake clock at t instead of Start, so a server
// can start on a later day, as a restarted service would.
func WithStart(t time.Time) Option { return func(o *options) { o.start = t } }

// WithCICheck makes check the server's CI Watcher check mode, run when
// CI Tracking is turned on; without it, nothing is checked.
func WithCICheck(check core.CICheck) Option { return func(o *options) { o.ciCheck = check } }

// WithIssuesCheck makes check the server's Issue Watcher check mode, run
// when GitHub Issues is turned on; without it, nothing is checked.
func WithIssuesCheck(check core.IssuesCheck) Option {
	return func(o *options) { o.issuesCheck = check }
}

// NewServer starts a test service, stopped when t ends.
func NewServer(t testing.TB, opts ...Option) *Server {
	t.Helper()
	o := options{version: Version, location: time.UTC, start: Start}
	for _, opt := range opts {
		opt(&o)
	}
	if o.usageDir == "" {
		o.usageDir = t.TempDir()
	}
	s := store.NewMemory()
	clk := clock.NewFake(o.start)
	c := core.New(s, clk, &ids.Sequence{})
	c.SetUsage(usage.New(o.usageDir, clk, o.location, o.log))
	c.SetCICheck(o.ciCheck)
	c.SetIssuesCheck(o.issuesCheck)
	hs := httptest.NewServer(frontdoor.Versioned(frontdoor.Handler(c, web.Handler(), o.log), o.version))
	t.Cleanup(hs.Close)
	return &Server{URL: hs.URL, Client: client.New(hs.URL), Clock: clk, Core: c, Store: s, UsageDir: o.usageDir}
}
