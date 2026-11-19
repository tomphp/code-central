// Package factorytest is the test harness for the local Factory: a real
// Code Central from codecentraltest, a real temporary git repo set up as
// a Project, and a fake `claude` on PATH. The fake is the test binary
// itself, linked or copied into the harness's bin folder as claude and run
// as the fake because the harness's environment names the fake's folder:
// it records its arguments, follows a Script (print stream-json, call the
// code-central CLI in-process, write a file, run git, exit) and needs
// nothing built. There is no shell, so it runs the same on every OS.
// script.go holds the Script's Steps and the Calls read back; fake.go
// holds the fakes themselves; settle.go the settled signal.
//
// A test package that uses it calls Main from its TestMain:
//
//	func TestMain(m *testing.M) { factorytest.Main(m) }
//
// # Settled
//
// A test never reads the Factory's output to learn where it is: it waits
// for the Factory to settle (Running.Settle), and then asserts what the
// Factory did, as Code Central, git and the fake's Calls show it. Poll is
// a Tick and a Settle, and Start returns once the first poll has
// finished and the Factory has settled.
//
// The Factory is settled when nothing it is doing will change anything
// without an outside event: the clock moving, an answer to a long-poll, a
// turn's claude printing or exiting, or a signal. It tells the harness of
// its work through factory.Work, each piece busy or resting:
//
//   - Busy (not settled): a poll in progress, a post of Session Events,
//     token usage, a Chat reply or a Heartbeat, git work, setting up a
//     worktree, starting a turn, and handling a turn's exit (its outcome,
//     push, finish checks and Failure Report).
//   - Resting (settled): a pending claim retry, the wake or Chat retry
//     pause, the wait for the next poll, an outage's backoff, a kill
//     grace in progress, an open wake or Chat long-poll, the wait for a
//     signal, and a turn whose claude is running.
//
// A timer that has fired counts as busy until the Factory has taken it,
// and work that follows other work starts before the other ends. On top
// of the Factory's own, Settle waits for:
//
//   - every turn whose claude is running to be parked in the fake's Wait
//     step, having noted each stop signal sent to it;
//   - no answer on its way through the proxy to the Factory: an open
//     long-poll counts as resting only once Code Central holds it and
//     until it answers, or while PassWakes holds its answer back;
//   - no other request through the proxy, and every signal sent taken.
//
// So a test whose turn runs a claude other than the scripted fake (a
// shim, a held session, a missing claude) can't Settle while it runs: it
// never parks in a Wait step. Such a test waits with WaitFor on the
// outside effect it needs, as a test does for Code Central state no
// Factory work changes, such as a Chat reply.
package factorytest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/factory"
)

// fakeEnv names the harness's data folder to a test binary run as a
// fake. It is in the Factory's environment, so its turns and git get it;
// the Factory ignores it.
const fakeEnv = "FACTORYTEST_FAKE"

// holdName is the program name HoldSession runs the test binary under.
const holdName = "hold-session"

// Main runs the fake the test binary was started as, named by the program
// name it was run under (claude, git or HoldSession's), and the tests
// otherwise.
func Main(m *testing.M) {
	if data := os.Getenv(fakeEnv); data != "" {
		os.Exit(runFake(data, os.Args))
	}
	code := m.Run()
	if seedDir != "" {
		os.RemoveAll(seedDir)
	}
	os.Exit(code)
}

// Timeout bounds every wait in the harness. A wait that holds returns at
// once, so it only costs time when a test is already failing; it is long
// because the Factory's tests start many processes, and a loaded CI
// runner can take longer than 30s to run one.
const Timeout = 2 * time.Minute

// EventBatchEvery is how often, on its clock, the Factory under test
// posts waiting Session Events and token usage: a few times a Tick. A
// test that needs a batch posted between polls moves the clock by it.
const EventBatchEvery = 10 * time.Second

// KillGrace is how long, on its clock, the Factory under test gives a
// turn between stop steps (SIGINT, SIGTERM): a test moves the clock by it
// to take a turn to the next step.
const KillGrace = 100 * time.Millisecond

// Harness is one Factory's world. Its fields are for assertions; change
// the Factory's environment with Setenv.
type Harness struct {
	t      testing.TB
	Server *codecentraltest.Server
	// Project is the Project Key the repo's committed .mcp.json names.
	Project string
	// Repo is the git repo's top level, the Home Repository's. Its parent
	// holds only it, the Factory's worktree folder, Repo + ".worktrees",
	// and the clones of the Repositories AddRepository adds.
	Repo string
	// Home is the Factory's HOME and its profile folder, so its state file
	// is under it even with HOME unset.
	Home string
	// Bin is the folder on PATH holding the fake claude (and FakeGit's git).
	Bin string
	// Backoff is the Factory's first wait before resuming turns an outage
	// cut off; set it before Start. New makes it a millisecond.
	Backoff time.Duration
	// Clock is the Factory's clock, every timer and timestamp of its: it
	// stands still until the test, Tick, Start, Poll, PassKillGrace,
	// Exited or Stop moves it. Tick moves it to the next poll's deadline,
	// through each timer due before it; Start and Poll pass a claim retry
	// pause or kill grace their poll waits on; Exited and Stop pass each
	// timer but the next poll's until the Factory has exited. A test that
	// moves it itself settles first. It starts at midday, as Server's
	// clock does, so that many Ticks keep the model responses the Factory
	// dates on Code Central's today in Usage.
	Clock *clock.Fake
	// URL is the Code Central the Factory and its sessions reach: Server
	// behind a proxy that SetDown can take down.
	URL       string
	down      atomic.Bool
	failing   atomic.Value // string: a path suffix the proxy fails
	missing   atomic.Value // string: a path suffix the proxy has no route for
	loseMu    sync.Mutex
	lose      []lossRule // cuts the proxy still owes, in the order set
	refusedMu sync.Mutex
	refused   map[string]int64 // requests refused while down or failed, by path
	holdMu    sync.Mutex
	wakesHeld chan struct{} // closed by PassWakes
	// inflight is the requests the proxy is serving, and polls what it
	// knows of the Factory's long-polls among them.
	inflight inflightRequests
	polls    longPolls
	// stall, if set before Start, is called by the proxy before it serves
	// each request but a long-poll: the harness's own tests hold the
	// Factory or a turn mid-way with it.
	stall func(*http.Request)
	// runs is the Factories running; change is closed at the next change
	// to their work or to the proxy's requests (changes).
	runMu    sync.Mutex
	runs     []*Running
	changeMu sync.Mutex
	change   chan struct{}
	data     string // scripts/, calls/, continue/, signals/ and waiting/
	env      map[string]string
	spec     string
}

// New starts a Code Central with Project "acme" and a git repo on branch
// main whose one commit holds a .mcp.json naming it.
func New(t testing.TB) *Harness {
	t.Helper()
	// git reports real paths, so resolve macOS's /var -> /private/var.
	root, err := filepath.EvalSymlinks(t.TempDir())
	must(t, err)
	// Code Central's clock starts where the Factory's does, at midday
	// today (Usage's days are UTC's), so the model responses the Factory
	// dates fall on Code Central's today in Usage, however many Ticks
	// move the Factory's clock on.
	y, m, d := time.Now().UTC().Date()
	now := time.Date(y, m, d, 12, 0, 0, 0, time.UTC)
	h := &Harness{
		t:       t,
		Server:  codecentraltest.NewServer(t, codecentraltest.WithStart(now)),
		Project: project,
		Repo:    filepath.Join(root, "work", "repo"),
		Home:    filepath.Join(root, "home"),
		Bin:     filepath.Join(root, "bin"),
		data:    filepath.Join(root, "data"),
		Backoff: time.Millisecond,
		Clock:   clock.NewFake(now),
		refused: map[string]int64{},
		polls:   longPolls{upstream: map[string]int{}},
		// The Factory polls only on Tick until a test passes its wakes.
		wakesHeld: make(chan struct{}),
	}
	template := filepath.Join(root, "git-template")
	for _, d := range []string{h.Home, h.Bin, template, filepath.Join(h.data, "scripts"), filepath.Join(h.data, "calls"), filepath.Join(h.data, "continue"), filepath.Join(h.data, "signals"), filepath.Join(h.data, "waiting")} {
		must(t, os.MkdirAll(d, 0o755))
	}
	h.URL = h.startProxy()
	gitPath, err := realGit()
	must(t, err)
	h.env = map[string]string{
		"PATH":             h.Bin + string(os.PathListSeparator) + filepath.Dir(gitPath),
		"HOME":             h.Home,
		"CODE_CENTRAL_URL": h.URL,
		// An empty template: git init writes no sample hooks to remove.
		"GIT_TEMPLATE_DIR": template,
		"TMPDIR":           os.TempDir(),
		"TEMP":             os.TempDir(),
		"TMP":              os.TempDir(),
		fakeEnv:            h.data,
	}
	for k, v := range gitIdentity {
		h.env[k] = v
	}
	h.install("claude")

	if _, err := h.Server.Client.CreateProject(context.Background(), api.CreateProjectInput{Key: h.Project}); err != nil {
		t.Fatal(err)
	}
	seed, err := seedRepo()
	must(t, err)
	must(t, os.CopyFS(h.Repo, os.DirFS(seed)))
	return h
}

// project is the Project Key of every Harness.
const project = "acme"

// gitIdentity is every git's environment in the harness: no system
// config, no background maintenance (its detached process can create
// .git/objects/maintenance.lock while the temp dir is being removed),
// and a fixed author and committer.
var gitIdentity = map[string]string{
	"GIT_CONFIG_NOSYSTEM": "1",
	"GIT_CONFIG_COUNT":    "2",
	"GIT_CONFIG_KEY_0":    "maintenance.auto",
	"GIT_CONFIG_VALUE_0":  "false",
	"GIT_CONFIG_KEY_1":    "gc.auto",
	"GIT_CONFIG_VALUE_1":  "0",
	"GIT_AUTHOR_NAME":     "Factory Test",
	"GIT_AUTHOR_EMAIL":    "factory@example.com",
	"GIT_COMMITTER_NAME":  "Factory Test",
	"GIT_COMMITTER_EMAIL": "factory@example.com",
}

// seedDir holds seedRepo's repo; Main removes it.
var seedDir string

// seedRepo is a repo on branch main whose one commit holds a .mcp.json
// naming the Project. It is made once per test binary and each
// Harness copies it, which costs no git processes.
var seedRepo = sync.OnceValues(func() (string, error) {
	git, err := realGit()
	if err != nil {
		return "", err
	}
	if seedDir, err = os.MkdirTemp("", "factorytest-seed"); err != nil {
		return "", err
	}
	repo := filepath.Join(seedDir, "repo")
	template := filepath.Join(seedDir, "git-template")
	for _, d := range []string{repo, template} {
		if err := os.Mkdir(d, 0o755); err != nil {
			return "", err
		}
	}
	mcp := `{"mcpServers":{"code-central":{"type":"http","url":"${CODE_CENTRAL_URL:-http://localhost:7733}/p/` + project + `/mcp"}}}` + "\n"
	if err := os.WriteFile(filepath.Join(repo, ".mcp.json"), []byte(mcp), 0o644); err != nil {
		return "", err
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", ".mcp.json"}, {"commit", "-q", "-m", "Set up Code Central"}} {
		cmd := exec.Command(git, args...)
		cmd.Dir = repo
		cmd.Env = []string{"HOME=" + seedDir, "GIT_TEMPLATE_DIR=" + template}
		for k, v := range gitIdentity {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	return repo, nil
})

// startProxy starts the proxy in front of Server and returns its URL.
// While the harness is down it drops every connection, as a stopped Code
// Central would refuse it.
func (h *Harness) startProxy() string {
	target, err := url.Parse(h.Server.URL)
	must(h.t, err)
	rp := httputil.NewSingleHostReverseProxy(target)
	inflight := &h.inflight
	inflight.changed = h.changed
	var finished atomic.Bool
	h.t.Cleanup(func() { finished.Store(true) })
	// A body-copy error carries no request, so its line names the
	// requests in flight.
	rp.ErrorLog = log.New(writerFunc(func(p []byte) (int, error) {
		if !finished.Load() {
			h.t.Logf("proxy: %s (in flight: %s)", strings.TrimSpace(string(p)), inflight.describe(time.Now()))
		}
		return len(p), nil
	}), "", 0)
	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		// A request its client gave up on, such as the Factory's Chat
		// long-poll when it stops, is no error worth a line.
		if r.Context().Err() == nil && !finished.Load() {
			h.t.Logf("proxy: %s %s: %v", r.Method, r.URL.Path, err)
		}
		w.WriteHeader(http.StatusBadGateway)
	}
	ps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if suffix, _ := h.failing.Load().(string); suffix != "" && strings.HasSuffix(r.URL.Path, suffix) {
			h.refuse(w, r, http.StatusInternalServerError, `{"code":"internal","message":"failing for the test"}`)
			return
		}
		if suffix, _ := h.missing.Load().(string); suffix != "" && strings.HasSuffix(r.URL.Path, suffix) {
			h.refuse(w, r, http.StatusNotFound, fmt.Sprintf(`{"code":"not_found","message":"no API resource at %s %s"}`, r.Method, r.URL.Path))
			return
		}
		if !h.down.Load() {
			if afterWrite, ok := h.takeLoss(r); ok {
				h.cut(rp, w, r, afterWrite, &finished)
				return
			}
			if name := longPollOf(r.URL.Path); name != "" {
				defer inflight.add(r, true)()
				h.serveLongPoll(rp, w, r, name)
				return
			}
			defer inflight.add(r, false)()
			if h.stall != nil {
				h.stall(r)
			}
			rp.ServeHTTP(w, r)
			return
		}
		h.countRefused(r)
		if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
			conn.Close()
		}
	}))
	h.t.Cleanup(ps.Close)
	return ps.URL
}

// writerFunc is an io.Writer made of a function.
type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

// inflightRequests records the requests the proxy is serving, telling
// changed of each change.
type inflightRequests struct {
	changed func()
	mu      sync.Mutex
	next    int
	reqs    map[int]inflightRequest
}

type inflightRequest struct {
	method, path string
	start        time.Time
	// longPoll marks a long-poll, which the Factory rests on.
	longPoll bool
}

// add records r, a long-poll or not, and returns the function that
// forgets it.
func (f *inflightRequests) add(r *http.Request, longPoll bool) func() {
	f.mu.Lock()
	if f.reqs == nil {
		f.reqs = map[int]inflightRequest{}
	}
	id := f.next
	f.next++
	f.reqs[id] = inflightRequest{r.Method, r.URL.Path, time.Now(), longPoll}
	f.mu.Unlock()
	f.changed()
	return func() {
		f.mu.Lock()
		delete(f.reqs, id)
		f.mu.Unlock()
		f.changed()
	}
}

// describe lists the requests in flight at now, oldest first.
func (f *inflightRequests) describe(now time.Time) string {
	parts := f.list(now, true)
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

// busy lists the requests in flight at now but the long-polls, oldest
// first.
func (f *inflightRequests) busy(now time.Time) []string { return f.list(now, false) }

func (f *inflightRequests) list(now time.Time, longPolls bool) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var list []inflightRequest
	for _, q := range f.reqs {
		if longPolls || !q.longPoll {
			list = append(list, q)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].start.Before(list[j].start) })
	var parts []string
	for _, q := range list {
		parts = append(parts, fmt.Sprintf("%s %s for %s", q.method, q.path, now.Sub(q.start).Round(time.Millisecond)))
	}
	return parts
}

// lossRule is a cut the proxy owes: the next n requests whose path ends in
// suffix lose their connection, after Code Central has served them if
// afterWrite.
type lossRule struct {
	suffix     string
	n          int
	afterWrite bool
}

// takeLoss reports whether r is to be cut, and if so whether after Code
// Central has served it, using up one of the cuts owed.
func (h *Harness) takeLoss(r *http.Request) (afterWrite, ok bool) {
	h.loseMu.Lock()
	defer h.loseMu.Unlock()
	for i := range h.lose {
		l := &h.lose[i]
		if l.n > 0 && strings.HasSuffix(r.URL.Path, l.suffix) {
			l.n--
			return l.afterWrite, true
		}
	}
	return false, false
}

// cut drops r's connection, counting it as refused. Given afterWrite it
// first lets Code Central serve r and throws the answer away.
func (h *Harness) cut(rp http.Handler, w http.ResponseWriter, r *http.Request, afterWrite bool, finished *atomic.Bool) {
	h.countRefused(r)
	when := "before it reached Code Central"
	if afterWrite {
		rp.ServeHTTP(httptest.NewRecorder(), r)
		when = "after Code Central served it"
	}
	if !finished.Load() {
		h.t.Logf("proxy: cut %s %s %s", r.Method, r.URL.Path, when)
	}
	if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
		conn.Close()
	}
}

// refuse answers r with status and the JSON error body, counting it.
func (h *Harness) refuse(w http.ResponseWriter, r *http.Request, status int, body string) {
	h.countRefused(r)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprint(w, body)
}

func (h *Harness) countRefused(r *http.Request) {
	h.refusedMu.Lock()
	defer h.refusedMu.Unlock()
	h.refused[r.URL.Path]++
}

// The ends of the paths of the Factory's long-polls: next_factory_wake
// and next_chat_work.
const (
	wakeSuffix     = "/factory/next-wake"
	chatWorkSuffix = "/chat/next-work"
)

// serveLongPoll proxies the Factory's long-poll called name, holding a
// next_factory_wake answer back until PassWakes. An answer on its way to
// the Factory is marked on it first, so the Factory isn't settled until
// it has taken it.
func (h *Harness) serveLongPoll(rp http.Handler, w http.ResponseWriter, r *http.Request, name string) {
	rec := httptest.NewRecorder()
	sent := h.sendLongPoll(name)
	rp.ServeHTTP(rec, r)
	h.holdMu.Lock()
	held := h.wakesHeld
	h.holdMu.Unlock()
	if name != factory.WakeLongPoll || held == nil {
		if r.Context().Err() == nil {
			h.answered(name)
		}
		sent(false)
	} else {
		sent(true)
		select {
		case <-held:
			h.answered(name)
			h.unpark()
		case <-r.Context().Done():
			h.unpark()
			return
		}
	}
	maps.Copy(w.Header(), rec.Header())
	w.WriteHeader(rec.Code)
	w.Write(rec.Body.Bytes())
}

// PassWakes lets the Factory's wakes through; a test calls it before
// Start. Until a test calls it, the harness holds back every answer from
// next_factory_wake, so the Factory polls only on Tick and what a test
// sets up, such as a Ready Ticket it then claims itself, is first seen by
// the poll it ticks.
func (h *Harness) PassWakes() {
	h.holdMu.Lock()
	defer h.holdMu.Unlock()
	if h.wakesHeld != nil {
		close(h.wakesHeld)
		h.wakesHeld = nil
	}
}

// SetDown takes Code Central down for the Factory and its sessions, or
// brings it back up. The harness's own reads still reach it.
func (h *Harness) SetDown(down bool) { h.down.Store(down) }

// SetFailing makes Code Central fail the Factory's and its sessions'
// requests to every path ending in suffix, such as "/model-choice", or
// stops failing them given "".
func (h *Harness) SetFailing(suffix string) { h.failing.Store(suffix) }

// SetMissing makes Code Central answer the Factory's and its sessions'
// requests to every path ending in suffix, such as "/factory/next-wake",
// as an older Code Central without that route does: not found.
func (h *Harness) SetMissing(suffix string) { h.missing.Store(suffix) }

// LoseAnswer cuts the connection of the next n requests whose path ends in
// suffix, such as "/claim", after Code Central has served them and before
// the answer is passed on: the write is applied and its client reads
// unexpected EOF.
func (h *Harness) LoseAnswer(suffix string, n int) { h.addLoss(lossRule{suffix, n, true}) }

// LoseRequest cuts the connection of the next n requests whose path ends in
// suffix before they reach Code Central: the write is never applied.
func (h *Harness) LoseRequest(suffix string, n int) { h.addLoss(lossRule{suffix, n, false}) }

func (h *Harness) addLoss(l lossRule) {
	h.loseMu.Lock()
	defer h.loseMu.Unlock()
	h.lose = append(h.lose, l)
}

// Refused is how many requests to path, such as "/api/session-events",
// were refused while Code Central was down, or failed by SetFailing or
// SetMissing, or cut by LoseAnswer or LoseRequest.
func (h *Harness) Refused(path string) int64 {
	h.refusedMu.Lock()
	defer h.refusedMu.Unlock()
	return h.refused[path]
}

// realGit is the git binary itself. On macOS /usr/bin/git is an xcrun
// shim whose lookup is cached per HOME, and every Harness has a fresh
// HOME, so going through the shim costs ~100ms of CPU per test; the git
// in git's exec path skips it. Elsewhere git on PATH is used as it is:
// on Windows the one in the exec path can't find its DLLs.
var realGit = sync.OnceValues(func() (string, error) {
	git, err := exec.LookPath("git")
	if err != nil || runtime.GOOS != "darwin" {
		return git, err
	}
	out, err := exec.Command(git, "--exec-path").Output()
	if err != nil {
		return git, nil
	}
	if p, err := filepath.EvalSymlinks(filepath.Join(strings.TrimSpace(string(out)), "git")); err == nil {
		return p, nil
	}
	return git, nil
})

func must(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// install puts the test binary in Bin as the fake name: a hard link, or a
// copy where the link can't be made.
func (h *Harness) install(name string) {
	self, err := os.Executable()
	must(h.t, err)
	dst := filepath.Join(h.Bin, name+exeSuffix)
	if os.Link(self, dst) == nil {
		return
	}
	src, err := os.Open(self)
	must(h.t, err)
	defer src.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	must(h.t, err)
	_, err = io.Copy(out, src)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	must(h.t, err)
}

// Setenv sets a variable in the Factory's environment; "" removes it.
func (h *Harness) Setenv(key, value string) {
	if value == "" {
		delete(h.env, key)
		return
	}
	h.env[key] = value
}

// Environ is the Factory's environment.
func (h *Harness) Environ() []string {
	var out []string
	for k, v := range h.env {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

// Claude is the fake claude's path, as the Factory finds it on PATH.
func (h *Harness) Claude() string { return filepath.Join(h.Bin, "claude"+exeSuffix) }

// RemoveClaude takes the fake claude off PATH.
func (h *Harness) RemoveClaude() { must(h.t, os.Remove(h.Claude())) }

// ShimClaude puts a claude.cmd on PATH in place of the fake claude, as
// installing claude with npm on Windows leaves.
func (h *Harness) ShimClaude() {
	h.RemoveClaude()
	must(h.t, os.WriteFile(filepath.Join(h.Bin, "claude.cmd"), []byte("@echo off\r\nnode \"%~dp0\\node_modules\\@anthropic-ai\\claude-code\\cli.js\" %*\r\n"), 0o755))
}

// HoldSession starts a process, outside the Factory, whose command line
// names session, as another claude driving that session would. It runs
// until the returned func or the end of the test.
func (h *Harness) HoldSession(session string) (release func()) {
	h.t.Helper()
	self, err := os.Executable()
	must(h.t, err)
	cmd := exec.Command(self)
	cmd.Args = []string{holdName, "--resume", session}
	cmd.Env = append(os.Environ(), fakeEnv+"="+h.data)
	must(h.t, cmd.Start())
	var once sync.Once
	release = func() {
		once.Do(func() {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		})
	}
	h.t.Cleanup(release)
	return release
}

// fakeGit is what the fake git reads from the data folder.
type fakeGit struct {
	// Version is the version it reports.
	Version string `json:"version"`
	// Git is the real git it runs for everything else.
	Git string `json:"git"`
}

// FakeGit puts a git on PATH that reports version and otherwise runs the
// real git.
func (h *Harness) FakeGit(version string) {
	real, err := realGit()
	must(h.t, err)
	b, err := json.Marshal(fakeGit{Version: version, Git: real})
	must(h.t, err)
	must(h.t, os.WriteFile(filepath.Join(h.data, "git.json"), b, 0o644))
	h.install("git")
}

// Git runs the real git in the repo and returns its trimmed output.
func (h *Harness) Git(args ...string) string { return h.GitIn(h.Repo, args...) }

// GitIn runs the real git in dir and returns its trimmed output.
func (h *Harness) GitIn(dir string, args ...string) string {
	h.t.Helper()
	git, err := realGit()
	must(h.t, err)
	var out []byte
	for attempt := 0; ; attempt++ {
		cmd := exec.Command(git, args...)
		cmd.Dir = dir
		cmd.Env = h.Environ()
		out, err = cmd.CombinedOutput()
		if err == nil || !failedToStart(err) || attempt == 4 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		h.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// failedToStart reports whether err is Windows failing to initialise the
// process (STATUS_DLL_INIT_FAILED, 0xc0000142), which happens when many
// processes start at once. The program never ran, so running it again is safe.
func failedToStart(err error) bool {
	var exit *exec.ExitError
	return runtime.GOOS == "windows" && errors.As(err, &exit) && uint32(exit.ExitCode()) == 0xc0000142
}

// Worktree is where the Factory keeps ticket's worktree.
func (h *Harness) Worktree(ticket string) string { return filepath.Join(h.Repo+".worktrees", ticket) }

// StateFile is the Factory's state file for the Project.
func (h *Harness) StateFile() string {
	return filepath.Join(h.Home, ".local", "state", "code-central-factory", h.Project+".json")
}

// ReadStateFile reads the Factory's state file, retrying while Windows
// refuses to open it because the Factory is renaming a new one into
// place.
func (h *Harness) ReadStateFile() ([]byte, error) { return readFile(h.StateFile()) }

// ReadyTicket creates a Ready implementation Ticket (on one shared Spec)
// and returns its ID.
func (h *Harness) ReadyTicket(title string) string {
	h.t.Helper()
	return h.readyTicket(title)
}

// StageTicket creates a Ready Stage Ticket, marked by stage's Label, on
// the same shared Spec, and returns its ID. A spec Stage's script, until
// a test sets its own, finishes it recommending split.
func (h *Harness) StageTicket(stage domain.StageName, title string) string {
	h.t.Helper()
	id := h.readyTicket(title, core.StageLabel(stage))
	if stage == "spec" {
		h.Script(id, FinishSpec("Done.", "split"), Result("success"))
	}
	return id
}

// LabelledTicket creates a Ready Ticket carrying label, such as a
// Wayfinder Label or prototype, on the same shared Spec, and returns its
// ID.
func (h *Harness) LabelledTicket(label, title string) string {
	h.t.Helper()
	return h.readyTicket(title, label)
}

// sharedSpec creates the shared Spec if it doesn't exist yet. It is
// Long-lived, so that its Tickets land one by one under the default Land
// per Spec, until MakeRequirement or MakePlainSpec takes the label off.
func (h *Harness) sharedSpec() {
	h.t.Helper()
	if h.spec != "" {
		return
	}
	ctx := context.Background()
	c := h.Server.Client
	s, err := c.CreateSpec(ctx, h.Project, api.CreateSpecInput{Title: "Work"})
	must(h.t, err)
	_, err = c.AddLabels(ctx, h.Project, api.LabelsInput{Target: api.Ref(s.ID), Labels: []string{domain.LongLivedLabel}})
	must(h.t, err)
	h.spec = s.ID
}

// MakePlainSpec takes the long-lived Label off the shared Spec, so that
// Land per Spec covers its Tickets.
func (h *Harness) MakePlainSpec() {
	h.t.Helper()
	h.sharedSpec()
	_, err := h.Server.Client.RemoveLabels(context.Background(), h.Project, api.LabelsInput{Target: api.Ref(h.spec), Labels: []string{domain.LongLivedLabel}})
	must(h.t, err)
}

func (h *Harness) readyTicket(title string, labels ...string) string {
	h.t.Helper()
	return h.NewTicket(api.CreateTicketInput{Title: title, State: domain.Ready, Labels: labels})
}

// NewTicket creates the Ticket in describes, such as a Draft one or one
// with Blockers, on the same shared Spec, and returns its ID.
func (h *Harness) NewTicket(in api.CreateTicketInput) string {
	h.t.Helper()
	h.sharedSpec()
	in.Spec = api.Ref(h.spec)
	tk, err := h.Server.Client.CreateTicket(context.Background(), h.Project, in)
	must(h.t, err)
	return tk.ID
}

// MakeRequirement makes the shared Spec a Requirement Spec, one with
// Stages, by adding a Draft Stage Ticket the Factory won't claim.
func (h *Harness) MakeRequirement() {
	h.t.Helper()
	ctx := context.Background()
	c := h.Server.Client
	h.MakePlainSpec()
	_, err := c.CreateTicket(ctx, h.Project, api.CreateTicketInput{Spec: api.Ref(h.spec), Title: "Stage", State: domain.Draft, Labels: []string{core.StageLabel("grilling")}})
	must(h.t, err)
}

// SpecID is the ID of the shared Spec the harness's Tickets are on.
func (h *Harness) SpecID() string { return h.spec }

// Ticket reads a Ticket from Code Central.
func (h *Harness) Ticket(id string) api.TicketView {
	h.t.Helper()
	tk, err := h.Server.Client.GetTicket(context.Background(), h.Project, api.Ref(id))
	must(h.t, err)
	return tk
}

// Transcript reads the Transcript of ticket's Claim number claim.
func (h *Harness) Transcript(ticket string, claim int) api.TranscriptView {
	h.t.Helper()
	tr, err := h.Server.Client.Transcript(context.Background(), h.Project, api.Ref(ticket), claim)
	must(h.t, err)
	return tr
}

// Usage reads Usage for the Project over the default range.
func (h *Harness) Usage() api.UsageView {
	h.t.Helper()
	u, err := h.Server.Client.Usage(context.Background(), h.Project, 0)
	must(h.t, err)
	return u
}

// Chat reads the Project's current Conversation, as the human.
func (h *Harness) Chat() api.ConversationView {
	h.t.Helper()
	cv, err := h.Human().GetChat(context.Background(), h.Project)
	must(h.t, err)
	return cv
}

// SendChat sends text as the human's Chat Message, as soon as Code
// Central takes it: it refuses one until a Factory's Heartbeat reports
// Chat support, and while a reply is running.
func (h *Harness) SendChat(text string) {
	h.t.Helper()
	var last error
	h.WaitForDescribed(func() string { return fmt.Sprintf("send_chat_message to be taken (last refused: %v)", last) }, func() bool {
		_, last = h.Human().SendChatMessage(context.Background(), h.Project, api.SendChatMessageInput{Text: text})
		return last == nil
	})
}

// WaitForReply waits for the Conversation's last Chat Message, Claude's
// reply, to end, and returns the Conversation.
func (h *Harness) WaitForReply() api.ConversationView {
	h.t.Helper()
	var cv api.ConversationView
	h.WaitForDescribed(func() string { return fmt.Sprintf("the Chat reply to end; the Conversation is %+v", cv) }, func() bool {
		cv = h.Chat()
		n := len(cv.Messages)
		return !cv.Running && n > 0 && cv.Messages[n-1].Outcome != ""
	})
	return cv
}

// SessionUsage reads one Usage session of the Project, agent being ""
// for the main session.
func (h *Harness) SessionUsage(session, agent string) api.SessionUsageView {
	h.t.Helper()
	q := url.Values{"project": {h.Project}, "session": {session}, "agent": {agent}}
	resp, err := http.Get(h.Server.URL + "/api/usage/session?" + q.Encode())
	must(h.t, err)
	defer resp.Body.Close()
	var v api.SessionUsageView
	must(h.t, json.NewDecoder(resp.Body).Decode(&v))
	return v
}

// Human is a client of the harness's Code Central acting as the human.
func (h *Harness) Human() *client.Client {
	c := client.New(h.Server.URL)
	c.Actor = api.Human
	return c
}

// Answer answers every open Question on ticket with text, as the human.
func (h *Harness) Answer(ticket, text string) {
	h.t.Helper()
	for _, r := range h.Ticket(ticket).Rounds {
		for _, q := range r.Questions {
			if q.Status == domain.QuestionOpen {
				_, err := h.Human().Answer(context.Background(), h.Project, api.AnswerInput{Ticket: api.Ref(ticket), Question: q.Number, Text: text})
				must(h.t, err)
			}
		}
	}
}

// WaitFor polls cond until it holds, failing the test after Timeout. It
// is for effects outside the Factory, such as Code Central state no
// Factory work changes, like a Chat reply: what the Factory does a test
// waits for with Running.Settle.
func (h *Harness) WaitFor(what string, cond func() bool) {
	h.t.Helper()
	h.WaitForDescribed(func() string { return what }, cond)
}

// WaitForDescribed is WaitFor with what read when the wait times out, so
// it can report the state the wait ended in.
func (h *Harness) WaitForDescribed(what func() string, cond func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(Timeout)
	for !cond() {
		if time.Now().After(deadline) {
			h.t.Fatalf("timed out waiting for %s", what())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// WantState reads the Ticket and fails the test unless it is in state,
// with its latest Failure Report if it has one. A test settles the
// Factory first.
func (h *Harness) WantState(id string, state domain.State) api.TicketView {
	h.t.Helper()
	tk := h.Ticket(id)
	if tk.State != state {
		h.t.Fatalf("%s is %s, want %s%s", id, tk.State, state, lastFailure(tk))
	}
	return tk
}

// lastFailure is tk's latest Failure Report, for a failing test to show;
// empty when it has none.
func lastFailure(tk api.TicketView) string {
	if len(tk.FailureReports) == 0 {
		return ""
	}
	r := tk.FailureReports[len(tk.FailureReports)-1]
	return fmt.Sprintf(": %s %s", r.WhatBroke, r.Why)
}

// Finished claims ticket outside the Factory and finishes it with this
// branch and commit, and the work in any other repositories, so it is In
// Review with work to land.
func (h *Harness) Finished(ticket, branch, commit string, repositories ...api.RepositoryWork) {
	h.t.Helper()
	ctx := context.Background()
	c := h.Server.Client
	res, err := c.Claim(ctx, h.Project, api.ClaimInput{Ticket: api.Ref(ticket)})
	must(h.t, err)
	_, err = c.Finish(ctx, h.Project, api.FinishInput{Ticket: api.Ref(ticket), ClaimID: res.ClaimID, Summary: "Done.", Branch: branch, Commit: commit, Repositories: repositories})
	must(h.t, err)
}

// NeedsMeItems is the Project's Needs Me items of reason, in Needs Me's
// order.
func (h *Harness) NeedsMeItems(reason api.NeedsMeReason) []api.NeedsMeRow {
	h.t.Helper()
	v, err := h.Human().NeedsMe(context.Background())
	must(h.t, err)
	var out []api.NeedsMeRow
	for _, g := range v.Groups {
		for _, row := range g.Tickets {
			if g.Reason == reason && row.Project == h.Project {
				out = append(out, row)
			}
		}
	}
	return out
}

// Accept accepts ticket as the human.
func (h *Harness) Accept(ticket string) {
	h.t.Helper()
	_, err := h.Human().Accept(context.Background(), h.Project, api.TicketInput{Ticket: api.Ref(ticket)})
	must(h.t, err)
}

// SetLandingMethod sets the Project's Landing Method as the human.
func (h *Harness) SetLandingMethod(in api.SetLandingMethodInput) {
	h.t.Helper()
	_, err := h.Human().SetLandingMethod(context.Background(), h.Project, in)
	must(h.t, err)
}

// Remote is a bare repository added as a named remote of the harness's
// repo.
type Remote struct {
	h *Harness
	// Name is the remote's name in the repo.
	Name string
	// Path is the bare repository.
	Path string
}

// AddRemote makes a bare repository and adds it to the repo as the
// remote name, with nothing pushed to it yet.
func (h *Harness) AddRemote(name string) *Remote {
	h.t.Helper()
	path := filepath.Join(filepath.Dir(filepath.Dir(h.Repo)), "remotes", name+".git")
	must(h.t, os.MkdirAll(path, 0o755))
	h.GitIn(path, "init", "-q", "--bare", "-b", "main")
	h.Git("remote", "add", name, path)
	return &Remote{h: h, Name: name, Path: path}
}

// Git runs the real git in the bare repository and returns its trimmed
// output.
func (r *Remote) Git(args ...string) string {
	r.h.t.Helper()
	return r.h.GitIn(r.Path, args...)
}

// Break makes every push to the remote fail, until Restore.
func (r *Remote) Break() {
	r.h.t.Helper()
	must(r.h.t, os.Rename(r.Path, r.Path+".broken"))
}

// Restore undoes Break.
func (r *Remote) Restore() {
	r.h.t.Helper()
	must(r.h.t, os.Rename(r.Path+".broken", r.Path))
}

// RefusePushes makes the remote refuse every push but still serve fetches,
// until AcceptPushes.
func (r *Remote) RefusePushes() {
	r.h.t.Helper()
	hook := filepath.Join(r.Path, "hooks", "pre-receive")
	must(r.h.t, os.MkdirAll(filepath.Dir(hook), 0o755))
	must(r.h.t, os.WriteFile(hook, []byte("#!/bin/sh\necho 'pushes are refused' >&2\nexit 1\n"), 0o755))
}

// AcceptPushes undoes RefusePushes.
func (r *Remote) AcceptPushes() {
	r.h.t.Helper()
	must(r.h.t, os.Remove(filepath.Join(r.Path, "hooks", "pre-receive")))
}

// Repository is a second real repository of the Project's (Spec
// code-central-848), beside the Home Repository.
type Repository struct {
	// Name is its Repository Name.
	Name string
	// CloneURL is its own bare remote, holding one commit on main; empty
	// for a local-only Repository (AddLocalRepository).
	CloneURL string
	// Path is where the Factory clones it: beside the Home Repository,
	// at <Repo's parent>/<Name>.
	Path string
	h    *Harness
}

// CommitOn commits file with content on branch in r's clone at Path, as
// Harness.CommitOn does in the Home Repository, and returns the commit.
func (r *Repository) CommitOn(branch, file, content string) string {
	r.h.t.Helper()
	return r.h.commitOn(r.Path, branch, file, content)
}

// Remote is r's bare remote, its Clone URL, as the remote name of the
// Factory's clone: the Push remote's name, which it clones under.
func (r *Repository) Remote(name string) *Remote {
	return &Remote{h: r.h, Name: name, Path: r.CloneURL}
}

// Worktree is where the Factory keeps ticket's worktree in r, beside its
// clone, at <Path's parent>/<Name>.worktrees/<ticket>; Worktree("chat")
// is r's Chat worktree.
func (r *Repository) Worktree(ticket string) string {
	return filepath.Join(filepath.Dir(r.Path), r.Name+".worktrees", ticket)
}

// AddRepository makes a bare repository named name, holding one commit
// on main, and adds it to the Project through add_repository, with no
// local path, so the Factory clones it beside the Home Repository.
func (h *Harness) AddRepository(name string) *Repository {
	h.t.Helper()
	r := h.MakeRepository(name)
	if _, err := h.Human().AddRepository(context.Background(), h.Project, api.AddRepositoryInput{Name: name, CloneURL: r.CloneURL}); err != nil {
		h.t.Fatalf("add_repository %s: %v", name, err)
	}
	return r
}

// MakeRepository makes AddRepository's bare repository without adding it
// to the Project.
func (h *Harness) MakeRepository(name string) *Repository {
	h.t.Helper()
	root := filepath.Dir(filepath.Dir(h.Repo))
	seed := filepath.Join(root, "seeds", name)
	must(h.t, os.MkdirAll(seed, 0o755))
	h.GitIn(seed, "init", "-q", "-b", "main")
	must(h.t, os.WriteFile(filepath.Join(seed, "README.md"), []byte(name+"\n"), 0o644))
	h.GitIn(seed, "add", "README.md")
	h.GitIn(seed, "commit", "-q", "-m", "Start "+name)
	r := &Repository{Name: name, CloneURL: filepath.Join(root, "repositories", name+".git"), Path: filepath.Join(filepath.Dir(h.Repo), name), h: h}
	h.GitIn(root, "clone", "-q", "--bare", seed, r.CloneURL)
	return r
}

// AddLocalRepository makes a plain git repository named name beside the
// Home Repository, with no remote and one commit on main, and adds it to
// the Project through add_repository with that local path and no Clone
// URL (Spec code-central-879): a local-only Repository, which the
// Factory adopts and never clones.
func (h *Harness) AddLocalRepository(name string) *Repository {
	h.t.Helper()
	r := &Repository{Name: name, Path: filepath.Join(filepath.Dir(h.Repo), name), h: h}
	must(h.t, os.MkdirAll(r.Path, 0o755))
	h.GitIn(r.Path, "init", "-q", "-b", "main")
	must(h.t, os.WriteFile(filepath.Join(r.Path, "README.md"), []byte(name+"\n"), 0o644))
	h.GitIn(r.Path, "add", "README.md")
	h.GitIn(r.Path, "commit", "-q", "-m", "Start "+name)
	if _, err := h.Human().AddRepository(context.Background(), h.Project, api.AddRepositoryInput{Name: name, LocalPath: r.Path}); err != nil {
		h.t.Fatalf("add_repository %s: %v", name, err)
	}
	return r
}

// CommitOn commits file with content on branch, made from main when it
// doesn't exist, and returns the commit. The repo is left on main.
func (h *Harness) CommitOn(branch, file, content string) string {
	h.t.Helper()
	return h.commitOn(h.Repo, branch, file, content)
}

// commitOn is CommitOn in the clone at dir.
func (h *Harness) commitOn(dir, branch, file, content string) string {
	h.t.Helper()
	if branch != "main" {
		if !strings.Contains(h.GitIn(dir, "branch", "--list", branch), branch) {
			h.GitIn(dir, "branch", branch, "main")
		}
		h.GitIn(dir, "checkout", "-q", branch)
		defer h.GitIn(dir, "checkout", "-q", "main")
	}
	must(h.t, os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644))
	h.GitIn(dir, "add", file)
	h.GitIn(dir, "commit", "-q", "-m", "Change "+file)
	return h.GitIn(dir, "rev-parse", "HEAD")
}

// Running is a Factory running in-process.
type Running struct {
	h              *Harness
	cancel         context.CancelFunc
	done           chan struct{}
	code           int
	stdout, stderr *buffer
	signals        chan os.Signal
	// track hears of the Factory's work in flight.
	track *tracker
}

// Start runs the Factory in the repo with args until Stop or the end of
// the test, and returns once its first poll has finished (or it has
// exited) and it has settled, so whatever the test sets up next is first
// seen by a later poll. Polls after the first happen only on Tick, or on
// a wake from next_factory_wake.
func (h *Harness) Start(args ...string) *Running {
	h.t.Helper()
	return h.StartIn(h.Repo, args...)
}

// StartIn is Start from dir.
func (h *Harness) StartIn(dir string, args ...string) *Running {
	h.t.Helper()
	r := h.launch(dir, args...)
	r.started()
	r.waitPoll()
	return r
}

// launch runs the Factory in dir with args, without waiting.
func (h *Harness) launch(dir string, args ...string) *Running {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Running{h: h, cancel: cancel, done: make(chan struct{}), stdout: &buffer{}, stderr: &buffer{}, signals: make(chan os.Signal, 2), track: newTracker(h)}
	env := factory.Env{
		Dir:             dir,
		Environ:         h.Environ(),
		UserHomeDir:     func() (string, error) { return h.Home, nil },
		Stdout:          r.stdout,
		Stderr:          r.stderr,
		Clock:           h.Clock,
		KillGrace:       KillGrace,
		Backoff:         h.Backoff,
		EventBatchEvery: EventBatchEvery,
		// A post that times out is dropped, so on a loaded machine a
		// test could wait for an event that never comes.
		EventPostTimeout: Timeout,
		Signals:          r.signals,
		Work:             r.track,
	}
	h.runMu.Lock()
	h.runs = append(h.runs, r)
	h.runMu.Unlock()
	go func() {
		defer h.changed()
		defer close(r.done)
		r.code = factory.Run(ctx, args, env)
	}()
	h.t.Cleanup(func() {
		r.Stop()
		// A wait that timed out says what it waited for, not why: what
		// the Factory said is the why.
		if h.t.Failed() {
			h.t.Logf("the Factory's stdout:\n%s\nits stderr:\n%s", r.stdout, r.stderr)
		}
	})
	return r
}

// Run runs the Factory to its exit, as for a refusal, and returns its exit
// code and output.
func (h *Harness) Run(args ...string) (code int, stdout, stderr string) {
	h.t.Helper()
	return h.RunIn(h.Repo, args...)
}

// RunIn is Run from dir.
func (h *Harness) RunIn(dir string, args ...string) (code int, stdout, stderr string) {
	h.t.Helper()
	r := h.launch(dir, args...)
	select {
	case <-r.done:
	case <-time.After(Timeout):
		h.t.Fatalf("the Factory didn't exit; stdout:\n%s\nstderr:\n%s", r.stdout, r.stderr)
	}
	return r.code, r.stdout.String(), r.stderr.String()
}

// Signal sends the Factory sig, as the terminal or the OS would, on
// Env.Signals: the channel the binary's main fills from signal.Notify, so
// the Factory takes the same path as for a real Ctrl-C on every OS. The
// Factory isn't settled until it has taken it.
func (r *Running) Signal(sig os.Signal) {
	r.track.signal()
	r.signals <- sig
}

// Interrupt is the human's Ctrl-C.
func (r *Running) Interrupt() { r.Signal(os.Interrupt) }

// Stop stops the Factory and returns its exit code. It keeps moving the
// clock past each kill grace until the Factory has exited, so a turn that
// lingers after a stop step is taken to the next.
func (r *Running) Stop() int {
	r.cancel()
	return r.exited("the Factory didn't stop")
}

// Exited waits for the Factory to exit on its own and returns its exit
// code. It passes each claim retry pause and kill grace meanwhile, as
// Poll does: a Factory stopping itself stops its turns on its way out.
// A draining Factory exits once its turns end, so a test waits for that
// with Exited.
func (r *Running) Exited() int {
	r.h.t.Helper()
	return r.exited("the Factory didn't exit")
}

// WantOutput fails the test unless the Factory has printed s. A test
// settles the Factory first: its output says what it did, never where it
// is.
func (r *Running) WantOutput(s string) {
	r.h.t.Helper()
	if out := r.stdout.String(); !strings.Contains(out, s) {
		r.h.t.Fatalf("the Factory hasn't printed %q; stdout:\n%s", s, out)
	}
}

// Stdout is what the Factory has printed so far.
func (r *Running) Stdout() string { return r.stdout.String() }

// Stderr is what the Factory has printed to stderr so far.
func (r *Running) Stderr() string { return r.stderr.String() }

type buffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *buffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}
