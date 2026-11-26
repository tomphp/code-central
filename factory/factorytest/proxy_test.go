package factorytest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/factory/git"

	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/codecentraltest"
)

// fakeT is a testing.TB that records what the test logs and what fails it
// with Errorf, from any goroutine, and runs its cleanups when finish is
// called.
type fakeT struct {
	testing.TB
	mu       sync.Mutex
	lines    []string
	errors   []string
	cleanups []func()
}

func (f *fakeT) Helper() {}
func (f *fakeT) Logf(format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lines = append(f.lines, fmt.Sprintf(format, args...))
}
func (f *fakeT) Errorf(format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errors = append(f.errors, fmt.Sprintf(format, args...))
}
func (f *fakeT) failed() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.errors, "\n")
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

// proxyTo starts a rig proxy, logging to a fake test, in front of
// upstream.
func proxyTo(t *testing.T, upstream http.Handler) (*fakeT, string) {
	up := httptest.NewServer(upstream)
	t.Cleanup(up.Close)
	ft := &fakeT{TB: t}
	h := &Rig{t: ft, Server: &codecentraltest.Server{URL: up.URL}, refused: map[string]int64{}}
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

// claimThroughProxy claims ticket through the rig's URL and returns the
// client's error.
func claimThroughProxy(h *Rig, ticket string) error {
	c := client.New(h.URL)
	_, err := c.Claim(context.Background(), h.Project, api.ClaimInput{Ticket: api.Ref(ticket)})
	return err
}

func TestLoseAnswerCutsAfterCodeCentralAppliedTheWrite(t *testing.T) {
	h := New(t)
	id := h.ReadyTicket("Lose an answer")
	h.LoseAnswer("/claim", 1)
	err := claimThroughProxy(h, id)
	if err == nil || !strings.Contains(err.Error(), "cannot reach Code Central") {
		t.Fatalf("claim error = %v, want the connection cut", err)
	}
	if got := h.Ticket(id).State; got != domain.InProgress {
		t.Fatalf("state = %v, want In Progress: Code Central applied the claim", got)
	}
	if n := h.Refused("/api/p/" + h.Project + "/claim"); n != 1 {
		t.Fatalf("refused = %d, want 1", n)
	}
}

func TestLoseRequestCutsBeforeCodeCentralSeesIt(t *testing.T) {
	h := New(t)
	id := h.ReadyTicket("Lose a request")
	h.LoseRequest("/claim", 1)
	if err := claimThroughProxy(h, id); err == nil {
		t.Fatal("claim succeeded, want the connection cut")
	}
	if got := h.Ticket(id).State; got != domain.Ready {
		t.Fatalf("state = %v, want Ready: the claim never arrived", got)
	}
	if err := claimThroughProxy(h, id); err != nil {
		t.Fatalf("second claim: %v, want it through once the cuts are used", err)
	}
}

func TestLoseAnswerLogsTheCutNamingTheRequest(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") }))
	t.Cleanup(up.Close)
	ft := &fakeT{TB: t}
	h := &Rig{t: ft, Server: &codecentraltest.Server{URL: up.URL}, refused: map[string]int64{}}
	url := h.startProxy()
	h.LoseAnswer("/claim", 1)
	if _, err := http.Post(url+"/p/acme/claim", "application/json", nil); err == nil {
		t.Fatal("request succeeded, want the connection cut")
	}
	if got := ft.logged(); !strings.Contains(got, "POST /p/acme/claim") {
		t.Fatalf("logged %q, want a line naming POST /p/acme/claim", got)
	}
	resp, err := http.Post(url+"/p/acme/claim", "application/json", nil)
	if err != nil {
		t.Fatalf("second request: %v, want it through", err)
	}
	resp.Body.Close()
}

// postRelay posts a step to run rev-parse HEAD in dir to the relay of the
// proxy at url and returns the answer's status and body.
func postRelay(t *testing.T, url, dir string) (int, string) {
	t.Helper()
	b, err := json.Marshal(relayStep{Dir: dir, Args: []string{"rev-parse", "HEAD"}})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(url+relayPath, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestTheProxyRunsTheRelayOnTheFakeGitAndNeverForwardsIt(t *testing.T) {
	var forwarded atomic.Int64
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded.Add(1) }))
	t.Cleanup(up.Close)
	ft := &fakeT{TB: t}
	defer ft.finish()
	fake := git.NewFake(t)
	dir := t.TempDir()
	head := fake.Init(dir, map[string]string{"README.md": "Hello\n"}, "First")
	h := &Rig{t: ft, Server: &codecentraltest.Server{URL: up.URL}, refused: map[string]int64{}, fake: fake}
	url := h.startProxy()

	status, body := postRelay(t, url, dir)
	var res relayResult
	if err := json.Unmarshal([]byte(body), &res); status != http.StatusOK || err != nil || res != (relayResult{Output: head}) {
		t.Errorf("the relay answered %d %s, want 200 with the output %s", status, body, head)
	}
	if n := forwarded.Load(); n != 0 {
		t.Errorf("Code Central saw %d requests, want the relay never forwarded", n)
	}
}

func TestTheProxyHasNoRelayUnderRealGit(t *testing.T) {
	var paths []string
	var mu sync.Mutex
	ft, url := proxyTo(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		http.NotFound(w, r)
	}))
	defer ft.finish()

	if status, body := postRelay(t, url, t.TempDir()); status != http.StatusNotFound {
		t.Errorf("the relay answered %d %s, want Code Central's 404", status, body)
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(paths, []string{relayPath}) {
		t.Errorf("Code Central saw %q, want the relay's path forwarded to it", paths)
	}
}
