package factorytest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
)

// fakeT is a testing.TB that records what the test logs and runs its
// cleanups when finish is called.
type fakeT struct {
	testing.TB
	mu       sync.Mutex
	lines    []string
	cleanups []func()
}

func (f *fakeT) Helper() {}
func (f *fakeT) Logf(format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lines = append(f.lines, fmt.Sprintf(format, args...))
}
func (f *fakeT) Cleanup(fn func()) { f.cleanups = append(f.cleanups, fn) }
func (f *fakeT) logged() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.lines, "\n")
}
func (f *fakeT) finish() {
	for i := len(f.cleanups) - 1; i >= 0; i-- {
		f.cleanups[i]()
	}
}

// proxyTo starts a harness proxy, logging to a fake test, in front of
// upstream.
func proxyTo(t *testing.T, upstream http.Handler) (*fakeT, string) {
	up := httptest.NewServer(upstream)
	t.Cleanup(up.Close)
	ft := &fakeT{TB: t}
	h := &Harness{t: ft, Server: &codecentraltest.Server{URL: up.URL}, refused: map[string]int64{}}
	return ft, h.startProxy()
}

// closesMidBody answers with a body shorter than it promised and drops the
// connection.
var closesMidBody = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Length", "100")
	w.Write([]byte("short"))
	w.(http.Flusher).Flush()
	conn, _, _ := w.(http.Hijacker).Hijack()
	conn.Close()
})

func TestProxyLogsUpstreamErrorNamingRequest(t *testing.T) {
	ft, url := proxyTo(t, closesMidBody)
	if resp, err := http.Get(url + "/p/acme/tickets"); err == nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	if got := ft.logged(); !strings.Contains(got, "GET /p/acme/tickets") {
		t.Fatalf("logged %q, want a line naming GET /p/acme/tickets", got)
	}
}

func TestProxyLogsNothingWhenPassing(t *testing.T) {
	ft, url := proxyTo(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") }))
	resp, err := http.Get(url + "/ok")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got := ft.logged(); got != "" {
		t.Fatalf("logged %q, want nothing", got)
	}
}

func TestProxyLogsNothingForACancelledRequest(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	ft, url := proxyTo(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
	}))
	defer close(release)
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", url+"/slow", nil)
	done := make(chan struct{})
	go func() { defer close(done); http.DefaultClient.Do(req) }()
	<-started
	cancel()
	<-done
	if got := ft.logged(); got != "" {
		t.Fatalf("logged %q, want nothing", got)
	}
}

func TestProxyDropsLinesAfterTheTest(t *testing.T) {
	ft, url := proxyTo(t, closesMidBody)
	ft.finish()
	if resp, err := http.Get(url + "/late"); err == nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	if got := ft.logged(); got != "" {
		t.Fatalf("logged %q after the test finished, want nothing", got)
	}
}
