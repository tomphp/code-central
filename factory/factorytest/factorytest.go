// Package factorytest is the test harness for the local Factory: a real
// Code Central from codecentraltest, a real temporary git repo set up as
// a Project, and a fake `claude` on PATH. The fake is the test binary
// itself, linked or copied into the harness's bin folder as claude and run
// as the fake because the harness's environment names the fake's folder:
// it records its arguments, follows a Script (print stream-json, call the
// code-central CLI in-process, write a file, run git, exit) and needs
// nothing built. There is no shell, so it runs the same on every OS.
// script.go holds the Script's Steps and the Calls read back; fake.go
// holds the fakes themselves.
//
// A test package that uses it calls Main from its TestMain:
//
//	func TestMain(m *testing.M) { factorytest.Main(m) }
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
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory"
	"github.com/tomoram/code-central/store"
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
	// Repo is the git repo's top level. Its parent holds only it and the
	// Factory's worktree folder, Repo + ".worktrees".
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
	// stands still until the test, Tick, Start, Poll or Stop moves it.
	// Tick moves it to the next poll's deadline; Start and Poll end a
	// claim retry pause or kill grace their poll waits on; Stop moves it
	// past the kill grace until the Factory has exited. It starts at
	// midday, as
	// Server's clock does, so that many Ticks keep the model responses
	// the Factory dates on Code Central's today in Usage.
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
	data      string        // scripts/ and calls/
	env       map[string]string
	spec      string
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
		// The Factory polls only on Tick until a test passes its wakes.
		wakesHeld: make(chan struct{}),
	}
	template := filepath.Join(root, "git-template")
	for _, d := range []string{h.Home, h.Bin, template, filepath.Join(h.data, "scripts"), filepath.Join(h.data, "calls"), filepath.Join(h.data, "continue"), filepath.Join(h.data, "signals")} {
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

	if _, err := h.Server.Client.CreateProject(context.Background(), core.CreateProjectInput{Key: h.Project}); err != nil {
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
	inflight := &inflightRequests{}
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
			defer inflight.add(r)()
			if strings.HasSuffix(r.URL.Path, wakeSuffix) {
				h.serveWake(rp, w, r)
				return
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

// inflightRequests records the requests the proxy is serving.
type inflightRequests struct {
	mu   sync.Mutex
	next int
	reqs map[int]inflightRequest
}

type inflightRequest struct {
	method, path string
	start        time.Time
}

// add records r and returns the function that forgets it.
func (f *inflightRequests) add(r *http.Request) func() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.reqs == nil {
		f.reqs = map[int]inflightRequest{}
	}
	id := f.next
	f.next++
	f.reqs[id] = inflightRequest{r.Method, r.URL.Path, time.Now()}
	return func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		delete(f.reqs, id)
	}
}

// describe lists the requests in flight at now, oldest first.
func (f *inflightRequests) describe(now time.Time) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var list []inflightRequest
	for _, q := range f.reqs {
		list = append(list, q)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].start.Before(list[j].start) })
	var parts []string
	for _, q := range list {
		parts = append(parts, fmt.Sprintf("%s %s for %s", q.method, q.path, now.Sub(q.start).Round(time.Millisecond)))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
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

// wakeSuffix ends the path of the Factory's next_factory_wake.
const wakeSuffix = "/factory/next-wake"

// serveWake proxies a next_factory_wake call, holding its answer back
// until PassWakes.
func (h *Harness) serveWake(rp http.Handler, w http.ResponseWriter, r *http.Request) {
	rec := httptest.NewRecorder()
	rp.ServeHTTP(rec, r)
	h.holdMu.Lock()
	held := h.wakesHeld
	h.holdMu.Unlock()
	if held != nil {
		select {
		case <-held:
		case <-r.Context().Done():
			return
		}
	}
	maps.Copy(w.Header(), rec.Header())
	w.WriteHeader(rec.Code)
	w.Write(rec.Body.Bytes())
}

// PassWakes lets the Factory's wakes through. Until a test calls it, the
// harness holds back every answer from next_factory_wake, so the Factory
// polls only on Tick and what a test sets up, such as a Ready Ticket it
// then claims itself, is first seen by the poll it ticks.
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

// WaitForWorktree waits until ticket's worktree is a usable git worktree: its
// directory appears before git has finished making it.
func (h *Harness) WaitForWorktree(ticket string) {
	h.t.Helper()
	git, err := realGit()
	must(h.t, err)
	h.WaitFor("the worktree of "+ticket, func() bool {
		cmd := exec.Command(git, "rev-parse", "HEAD")
		cmd.Dir = h.Worktree(ticket)
		cmd.Env = h.Environ()
		return cmd.Run() == nil
	})
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
func (h *Harness) StageTicket(stage core.StageName, title string) string {
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
	s, err := c.CreateSpec(ctx, h.Project, core.CreateSpecInput{Title: "Work"})
	must(h.t, err)
	_, err = c.AddLabels(ctx, h.Project, core.LabelsInput{Target: core.Ref(s.ID), Labels: []string{core.LongLivedLabel}})
	must(h.t, err)
	h.spec = s.ID
}

// MakePlainSpec takes the long-lived Label off the shared Spec, so that
// Land per Spec covers its Tickets.
func (h *Harness) MakePlainSpec() {
	h.t.Helper()
	h.sharedSpec()
	_, err := h.Server.Client.RemoveLabels(context.Background(), h.Project, core.LabelsInput{Target: core.Ref(h.spec), Labels: []string{core.LongLivedLabel}})
	must(h.t, err)
}

func (h *Harness) readyTicket(title string, labels ...string) string {
	h.t.Helper()
	return h.NewTicket(core.CreateTicketInput{Title: title, State: store.Ready, Labels: labels})
}

// NewTicket creates the Ticket in describes, such as a Draft one or one
// with Blockers, on the same shared Spec, and returns its ID.
func (h *Harness) NewTicket(in core.CreateTicketInput) string {
	h.t.Helper()
	h.sharedSpec()
	in.Spec = core.Ref(h.spec)
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
	_, err := c.CreateTicket(ctx, h.Project, core.CreateTicketInput{Spec: core.Ref(h.spec), Title: "Stage", State: store.Draft, Labels: []string{core.StageLabel("grilling")}})
	must(h.t, err)
}

// SpecID is the ID of the shared Spec the harness's Tickets are on.
func (h *Harness) SpecID() string { return h.spec }

// Ticket reads a Ticket from Code Central.
func (h *Harness) Ticket(id string) core.TicketView {
	h.t.Helper()
	tk, err := h.Server.Client.GetTicket(context.Background(), h.Project, core.Ref(id))
	must(h.t, err)
	return tk
}

// Transcript reads the Transcript of ticket's Claim number claim.
func (h *Harness) Transcript(ticket string, claim int) core.TranscriptView {
	h.t.Helper()
	tr, err := h.Server.Client.Transcript(context.Background(), h.Project, core.Ref(ticket), claim)
	must(h.t, err)
	return tr
}

// Usage reads Usage for the Project over the default range.
func (h *Harness) Usage() core.UsageView {
	h.t.Helper()
	u, err := h.Server.Client.Usage(context.Background(), h.Project, 0)
	must(h.t, err)
	return u
}

// Chat reads the Project's current Conversation, as the human.
func (h *Harness) Chat() core.ConversationView {
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
		_, last = h.Human().SendChatMessage(context.Background(), h.Project, core.SendChatMessageInput{Text: text})
		return last == nil
	})
}

// WaitForReply waits for the Conversation's last Chat Message, Claude's
// reply, to end, and returns the Conversation.
func (h *Harness) WaitForReply() core.ConversationView {
	h.t.Helper()
	var cv core.ConversationView
	h.WaitForDescribed(func() string { return fmt.Sprintf("the Chat reply to end; the Conversation is %+v", cv) }, func() bool {
		cv = h.Chat()
		n := len(cv.Messages)
		return !cv.Running && n > 0 && cv.Messages[n-1].Outcome != ""
	})
	return cv
}

// SessionUsage reads one Usage session of the Project, agent being ""
// for the main session.
func (h *Harness) SessionUsage(session, agent string) core.SessionUsageView {
	h.t.Helper()
	q := url.Values{"project": {h.Project}, "session": {session}, "agent": {agent}}
	resp, err := http.Get(h.Server.URL + "/api/usage/session?" + q.Encode())
	must(h.t, err)
	defer resp.Body.Close()
	var v core.SessionUsageView
	must(h.t, json.NewDecoder(resp.Body).Decode(&v))
	return v
}

// Human is a client of the harness's Code Central acting as the human.
func (h *Harness) Human() *client.Client {
	c := client.New(h.Server.URL)
	c.Actor = core.Human
	return c
}

// Answer answers every open Question on ticket with text, as the human.
func (h *Harness) Answer(ticket, text string) {
	h.t.Helper()
	for _, r := range h.Ticket(ticket).Rounds {
		for _, q := range r.Questions {
			if q.Status == store.QuestionOpen {
				_, err := h.Human().Answer(context.Background(), h.Project, core.AnswerInput{Ticket: core.Ref(ticket), Question: q.Number, Text: text})
				must(h.t, err)
			}
		}
	}
}

// WaitFor polls cond until it holds, failing the test after Timeout.
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

// WaitForState waits for the Ticket to reach state. A Ticket that is
// Failed or Cancelled instead fails the test at once: only the human
// moves it on, so waiting would only time out.
func (h *Harness) WaitForState(id string, state store.State) core.TicketView {
	h.t.Helper()
	var tk core.TicketView
	h.WaitFor(fmt.Sprintf("%s to be %s", id, state), func() bool {
		tk = h.Ticket(id)
		if tk.State != state && (tk.State == store.Failed || tk.State == store.Cancelled) {
			h.t.Fatalf("waiting for %s to be %s, it is %s%s", id, state, tk.State, lastFailure(tk))
		}
		return tk.State == state
	})
	return tk
}

// lastFailure is tk's latest Failure Report, for a failing test to show;
// empty when it has none.
func lastFailure(tk core.TicketView) string {
	if len(tk.FailureReports) == 0 {
		return ""
	}
	r := tk.FailureReports[len(tk.FailureReports)-1]
	return fmt.Sprintf(": %s %s", r.WhatBroke, r.Why)
}

// Finished claims ticket outside the Factory and finishes it with this
// branch and commit, so it is In Review with work to land.
func (h *Harness) Finished(ticket, branch, commit string) {
	h.t.Helper()
	ctx := context.Background()
	c := h.Server.Client
	res, err := c.Claim(ctx, h.Project, core.ClaimInput{Ticket: core.Ref(ticket)})
	must(h.t, err)
	_, err = c.Finish(ctx, h.Project, core.FinishInput{Ticket: core.Ref(ticket), ClaimID: res.ClaimID, Summary: "Done.", Branch: branch, Commit: commit})
	must(h.t, err)
}

// Accept accepts ticket as the human.
func (h *Harness) Accept(ticket string) {
	h.t.Helper()
	_, err := h.Human().Accept(context.Background(), h.Project, core.TicketInput{Ticket: core.Ref(ticket)})
	must(h.t, err)
}

// SetLandingMethod sets the Project's Landing Method as the human.
func (h *Harness) SetLandingMethod(in core.SetLandingMethodInput) {
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

// CommitOn commits file with content on branch, made from main when it
// doesn't exist, and returns the commit. The repo is left on main.
func (h *Harness) CommitOn(branch, file, content string) string {
	h.t.Helper()
	if branch != "main" {
		if !strings.Contains(h.Git("branch", "--list", branch), branch) {
			h.Git("branch", branch, "main")
		}
		h.Git("checkout", "-q", branch)
		defer h.Git("checkout", "-q", "main")
	}
	must(h.t, os.WriteFile(filepath.Join(h.Repo, file), []byte(content), 0o644))
	h.Git("add", file)
	h.Git("commit", "-q", "-m", "Change "+file)
	return h.Git("rev-parse", "HEAD")
}

// Running is a Factory running in-process.
type Running struct {
	h              *Harness
	cancel         context.CancelFunc
	done           chan struct{}
	code           int
	stdout, stderr *buffer
	signals        chan os.Signal
	clock          *factoryClock
	ticked         int // the number of the last wait for a poll Tick fired
}

// The Factory's durations the harness knows, to tell its timers apart.
const (
	// defaultPoll is the Factory's --poll when a test passes none.
	defaultPoll = 30 * time.Second
	// claimRetryPause is the Factory's pause between the tries of a claim
	// whose answer was lost.
	claimRetryPause = time.Second
)

// factoryClock is the Factory's clock: the harness's Clock, noting each
// wait for a poll, a timer of the Factory's --poll, so that Tick can move
// the clock to the poll's deadline and Start and Poll can see a poll
// finish, and each pause a poll can wait on, a claim retry pause or the
// kill grace of a turn it stops, so that they can end it.
type factoryClock struct {
	*clock.Fake
	poll time.Duration

	mu     sync.Mutex
	waits  []pollWait // every wait for a poll, oldest first
	pauses []pause    // the claim retry pauses and kill graces not over yet
}

// pause is a claim retry pause or a kill grace, due at. The harness ends
// it no sooner than ripe, in real time: a kill grace only once it has run
// KillGrace in real time as well, so that the turn has had the time to
// take the stop step before it, as it would with no fake clock.
type pause struct {
	at, ripe time.Time
	grace    bool
}

// pollWait is one of the Factory's waits for its next poll: its timer,
// due at.
type pollWait struct {
	at    time.Time
	timer <-chan time.Time
}

// After is the fake clock's After, noting a wait for a poll, a claim
// retry pause or a kill grace.
func (c *factoryClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	at := c.Now().Add(d)
	timer := c.Fake.After(d)
	switch d {
	case c.poll:
		c.waits = append(c.waits, pollWait{at, timer})
	case claimRetryPause:
		c.pauses = append(c.pauses, pause{at: at, ripe: time.Now()})
	case KillGrace:
		c.pauses = append(c.pauses, pause{at: at, ripe: time.Now().Add(KillGrace), grace: true})
	}
	return timer
}

// latestWait is the Factory's latest wait for a poll and its number,
// counting from 1, or 0 when it hasn't waited for one yet.
func (c *factoryClock) latestWait() (int, pollWait) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.waits) == 0 {
		return 0, pollWait{}
	}
	return len(c.waits), c.waits[len(c.waits)-1]
}

// endPause moves the clock to the end of the earliest ripe claim retry
// pause or kill grace still running, if one is: a poll waiting on it
// can't finish until it ends.
func (c *factoryClock) endPause() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dropEnded()
	var end time.Time
	for _, p := range c.pauses {
		if !time.Now().Before(p.ripe) && (end.IsZero() || p.at.Before(end)) {
			end = p.at
		}
	}
	if !end.IsZero() {
		c.Set(end)
	}
}

// inKillGrace reports whether a turn being stopped is in its kill grace.
func (c *factoryClock) inKillGrace() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dropEnded()
	return slices.ContainsFunc(c.pauses, func(p pause) bool { return p.grace })
}

// dropEnded forgets the pauses the clock has passed the end of.
func (c *factoryClock) dropEnded() {
	now := c.Now()
	c.pauses = slices.DeleteFunc(c.pauses, func(p pause) bool { return !p.at.After(now) })
}

// pollFlag is the --poll args give the Factory, or its default.
func pollFlag(args []string) time.Duration {
	poll := defaultPoll
	for i, a := range args {
		if a == "--" {
			break
		}
		v, ok := strings.CutPrefix(a, "--poll=")
		if !ok && a == "--poll" && i+1 < len(args) {
			v, ok = args[i+1], true
		}
		if d, err := time.ParseDuration(v); ok && err == nil {
			poll = d
		}
	}
	return poll
}

// Start runs the Factory in the repo with args until Stop or the end of
// the test, and returns once its first poll has finished (or it has
// exited), so whatever the test sets up next is first seen by a later
// poll. Polls after the first happen only on Tick, or on a wake from
// next_factory_wake.
func (h *Harness) Start(args ...string) *Running {
	h.t.Helper()
	return h.StartIn(h.Repo, args...)
}

// StartIn is Start from dir.
func (h *Harness) StartIn(dir string, args ...string) *Running {
	h.t.Helper()
	r := h.launch(dir, args...)
	r.waitForPolls(1)
	return r
}

// launch runs the Factory in dir with args, without waiting.
func (h *Harness) launch(dir string, args ...string) *Running {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Running{h: h, cancel: cancel, done: make(chan struct{}), stdout: &buffer{}, stderr: &buffer{}, signals: make(chan os.Signal, 2),
		clock: &factoryClock{Fake: h.Clock, poll: pollFlag(args)}}
	env := factory.Env{
		Dir:             dir,
		Environ:         h.Environ(),
		UserHomeDir:     func() (string, error) { return h.Home, nil },
		Stdout:          r.stdout,
		Stderr:          r.stderr,
		Clock:           r.clock,
		KillGrace:       KillGrace,
		Backoff:         h.Backoff,
		EventBatchEvery: EventBatchEvery,
		// A post that times out is dropped, so on a loaded machine a
		// test could wait for an event that never comes.
		EventPostTimeout: Timeout,
		Signals:          r.signals,
	}
	go func() {
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

// Tick lets the Factory make its next poll: it moves the clock to the
// poll's deadline, firing every timer due by then on the way.
func (r *Running) Tick() {
	r.h.t.Helper()
	r.tick()
}

// tick moves the clock to the deadline of the Factory's latest wait for a
// poll that no Tick has fired, once it waits, and returns the wait's
// number once the Factory has taken its timer: the poll it starts has
// finished once the Factory waits again. A wait a wake ended first
// leaves the tick for the next one. It returns 0 once the Factory has
// exited.
func (r *Running) tick() int {
	r.h.t.Helper()
	deadline := time.Now().Add(Timeout)
	for {
		select {
		case <-r.done:
			return 0
		default:
		}
		if n, w := r.clock.latestWait(); n > r.ticked {
			if w.at.After(r.h.Clock.Now()) {
				r.h.Clock.Set(w.at)
			}
			if len(w.timer) == 0 {
				r.ticked = n
				return n
			}
		}
		r.clock.endPause()
		if time.Now().After(deadline) {
			r.h.t.Fatal("the Factory didn't wait for its next poll")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Poll waits for the Factory's current poll to finish, lets it make its
// next one and waits for that to finish too. Turns the poll started may
// still be running.
func (r *Running) Poll() {
	r.h.t.Helper()
	r.waitForPolls(r.tick() + 1)
}

// PollUntil polls until cond holds, failing the test after Timeout. The
// Factory posts its Session Events, token usage and Chat items in batches
// on its clock, and each poll moves the clock past a batch, so cond can
// wait on what a post makes true. A post goes on after the poll, so cond
// is checked a while before the next poll.
func (r *Running) PollUntil(what string, cond func() bool) {
	r.h.t.Helper()
	deadline := time.Now().Add(Timeout)
	for {
		for range 10 {
			if cond() {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		if time.Now().After(deadline) {
			r.h.t.Fatalf("timed out polling for %s", what)
		}
		r.Poll()
	}
}

// WaitForLongPoll waits until the Factory holds a next_factory_wake call
// open that no wake has ended yet, so a wake from here on reaches it.
func (r *Running) WaitForLongPoll() {
	r.h.t.Helper()
	r.h.WaitFor("the Factory's next_factory_wake call", func() bool {
		return r.h.Server.Core.WaitingFactoryWakes(r.h.Project) > 0
	})
}

// WaitForTurn waits for the Factory to have started a turn. Its poll can
// lose a claim to a dropped connection, which it retries only at its next
// poll, so WaitForTurn makes that poll whenever a claim has failed.
func (r *Running) WaitForTurn(what string) {
	r.h.t.Helper()
	failed := 0
	r.h.WaitFor(what, func() bool {
		if len(r.h.Calls()) > 0 {
			return true
		}
		if n := strings.Count(r.stdout.String(), "claim: "); n > failed {
			failed = n
			r.Poll()
		}
		return false
	})
}

// WaitForClaimed waits for id to be in state, re-polling whenever a claim
// has failed to a dropped connection, as WaitForTurn does.
func (r *Running) WaitForClaimed(id string, state store.State) {
	r.h.t.Helper()
	failed := 0
	r.h.WaitFor(fmt.Sprintf("%s to be %s", id, state), func() bool {
		if r.h.Ticket(id).State == state {
			return true
		}
		if n := strings.Count(r.stdout.String(), "claim: "); n > failed {
			failed = n
			r.Poll()
		}
		return false
	})
}

// waitForPolls waits for the Factory to have waited for a poll n times,
// ending each claim retry pause and kill grace a poll waits on
// meanwhile.
func (r *Running) waitForPolls(n int) {
	r.h.t.Helper()
	r.h.WaitFor(fmt.Sprintf("poll %d to finish (stdout %q)", n, r.stdout), func() bool {
		select {
		case <-r.done:
			return true
		default:
		}
		if waits, _ := r.clock.latestWait(); waits >= n {
			return true
		}
		r.clock.endPause()
		return false
	})
}

// WaitForOutput waits for s on the Factory's stdout.
func (r *Running) WaitForOutput(s string) {
	r.h.t.Helper()
	r.h.WaitFor(fmt.Sprintf("%q on stdout (have %q, stderr %q)", s, r.stdout, r.stderr), func() bool {
		return strings.Contains(r.stdout.String(), s)
	})
}

// Signal sends the Factory sig, as the terminal or the OS would, on
// Env.Signals: the channel the binary's main fills from signal.Notify, so
// the Factory takes the same path as for a real Ctrl-C on every OS.
func (r *Running) Signal(sig os.Signal) { r.signals <- sig }

// Interrupt is the human's Ctrl-C.
func (r *Running) Interrupt() { r.Signal(os.Interrupt) }

// PassKillGrace waits for a turn being stopped to be in its kill grace
// and moves the clock past it, taking the turn to its next stop step.
func (r *Running) PassKillGrace() {
	r.h.t.Helper()
	r.h.WaitFor("a turn's kill grace", r.clock.inKillGrace)
	r.h.Clock.Advance(KillGrace)
}

// Stop stops the Factory and returns its exit code. It keeps moving the
// clock past each kill grace until the Factory has exited, so a turn that
// lingers after a stop step is taken to the next.
func (r *Running) Stop() int {
	r.cancel()
	return r.exited("the Factory didn't stop")
}

// Exited waits for the Factory to exit on its own and returns its exit
// code. It ends each claim retry pause and kill grace meanwhile, as
// Poll does: a Factory stopping itself stops its turns on its way out.
func (r *Running) Exited() int {
	r.h.t.Helper()
	return r.exited("the Factory didn't exit")
}

// exited waits for the Factory to exit, ending each claim retry pause and
// kill grace meanwhile, and returns its exit code. It fails the test with
// failure after Timeout.
func (r *Running) exited(failure string) int {
	r.h.t.Helper()
	timeout := time.After(Timeout)
	for {
		select {
		case <-r.done:
			return r.code
		case <-timeout:
			r.h.t.Fatalf("%s; stdout:\n%s", failure, r.stdout)
		case <-time.After(5 * time.Millisecond):
			r.clock.endPause()
		}
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
