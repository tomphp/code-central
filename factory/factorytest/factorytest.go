// Package factorytest is the test harness for the local Factory: a real
// Code Central from codecentraltest, a real temporary git repo set up as
// a Project, and a fake `claude` on PATH. The fake is the test binary
// itself, linked or copied into the harness's bin folder as claude and run
// as the fake because the harness's environment names the fake's folder:
// it records its arguments, follows a Script (print stream-json, call the
// code-central CLI in-process, write a file, run git, exit) and needs
// nothing built. There is no shell, so it runs the same on every OS.
//
// A test package that uses it calls Main from its TestMain:
//
//	func TestMain(m *testing.M) { factorytest.Main(m) }
package factorytest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tomoram/code-central/cli"
	"github.com/tomoram/code-central/client"
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
	os.Exit(m.Run())
}

func runFake(data string, args []string) int {
	switch name := strings.TrimSuffix(filepath.Base(args[0]), exeSuffix); name {
	case "claude":
		return runFakeClaude(data, args[1:])
	case "git":
		return runFakeGit(data, args[1:])
	case holdName:
		// Long enough for any test; HoldSession's release kills it sooner.
		time.Sleep(time.Minute)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "factorytest: %s=%s but run as %q, not a fake\n", fakeEnv, data, name)
		return 70
	}
}

// Timeout bounds every wait in the harness.
const Timeout = 10 * time.Second

// EventBatchEvery is how often the Factory under test posts waiting
// Session Events.
const EventBatchEvery = 10 * time.Millisecond

// KillGrace is how long the Factory under test gives a turn between
// SIGINT and SIGTERM.
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
	// URL is the Code Central the Factory and its sessions reach: Server
	// behind a proxy that SetDown can take down.
	URL       string
	down      atomic.Bool
	refusedMu sync.Mutex
	refused   map[string]int64 // requests refused while down, by path
	data      string           // scripts/ and calls/
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
	h := &Harness{
		t:       t,
		Server:  codecentraltest.NewServer(t),
		Project: "acme",
		Repo:    filepath.Join(root, "work", "repo"),
		Home:    filepath.Join(root, "home"),
		Bin:     filepath.Join(root, "bin"),
		data:    filepath.Join(root, "data"),
		Backoff: time.Millisecond,
		refused: map[string]int64{},
	}
	for _, d := range []string{h.Repo, h.Home, h.Bin, filepath.Join(h.data, "scripts"), filepath.Join(h.data, "calls"), filepath.Join(h.data, "continue"), filepath.Join(h.data, "signals")} {
		must(t, os.MkdirAll(d, 0o755))
	}
	h.URL = h.startProxy()
	gitPath, err := realGit()
	must(t, err)
	h.env = map[string]string{
		"PATH":                h.Bin + string(os.PathListSeparator) + filepath.Dir(gitPath),
		"HOME":                h.Home,
		"CODE_CENTRAL_URL":    h.URL,
		"GIT_CONFIG_NOSYSTEM": "1",
		"GIT_AUTHOR_NAME":     "Factory Test",
		"GIT_AUTHOR_EMAIL":    "factory@example.com",
		"GIT_COMMITTER_NAME":  "Factory Test",
		"GIT_COMMITTER_EMAIL": "factory@example.com",
		"TMPDIR":              os.TempDir(),
		"TEMP":                os.TempDir(),
		"TMP":                 os.TempDir(),
		fakeEnv:               h.data,
	}
	h.install("claude")

	if _, err := h.Server.Client.CreateProject(context.Background(), core.CreateProjectInput{Key: h.Project}); err != nil {
		t.Fatal(err)
	}
	mcp := `{"mcpServers":{"code-central":{"type":"http","url":"${CODE_CENTRAL_URL:-http://localhost:7733}/p/` + h.Project + `/mcp"}}}` + "\n"
	h.Git("init", "-q", "-b", "main")
	must(t, os.WriteFile(filepath.Join(h.Repo, ".mcp.json"), []byte(mcp), 0o644))
	h.Git("add", ".mcp.json")
	h.Git("commit", "-q", "-m", "Set up Code Central")
	return h
}

// startProxy starts the proxy in front of Server and returns its URL.
// While the harness is down it drops every connection, as a stopped Code
// Central would refuse it.
func (h *Harness) startProxy() string {
	target, err := url.Parse(h.Server.URL)
	must(h.t, err)
	rp := httputil.NewSingleHostReverseProxy(target)
	ps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.down.Load() {
			rp.ServeHTTP(w, r)
			return
		}
		h.refusedMu.Lock()
		h.refused[r.URL.Path]++
		h.refusedMu.Unlock()
		if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
			conn.Close()
		}
	}))
	h.t.Cleanup(ps.Close)
	return ps.URL
}

// SetDown takes Code Central down for the Factory and its sessions, or
// brings it back up. The harness's own reads still reach it.
func (h *Harness) SetDown(down bool) { h.down.Store(down) }

// Refused is how many requests to path, such as "/api/session-events",
// were refused while Code Central was down.
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

// RemoveClaude takes the fake claude off PATH.
func (h *Harness) RemoveClaude() { must(h.t, os.Remove(filepath.Join(h.Bin, "claude"+exeSuffix))) }

// ShimClaude puts a claude.cmd on PATH in place of the fake claude, as
// installing claude with npm on Windows leaves.
func (h *Harness) ShimClaude() {
	h.RemoveClaude()
	must(h.t, os.WriteFile(filepath.Join(h.Bin, "claude.cmd"), []byte("@echo off\r\nnode \"%~dp0\\node_modules\\@anthropic-ai\\claude-code\\cli.js\" %*\r\n"), 0o755))
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
	cmd := exec.Command(git, args...)
	cmd.Dir = dir
	cmd.Env = h.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		h.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// Worktree is where the Factory keeps ticket's worktree.
func (h *Harness) Worktree(ticket string) string { return filepath.Join(h.Repo+".worktrees", ticket) }

// StateFile is the Factory's state file for the Project.
func (h *Harness) StateFile() string {
	return filepath.Join(h.Home, ".local", "state", "code-central-factory", h.Project+".json")
}

// ReadyTicket creates a Ready implementation Ticket (on one shared Spec)
// and returns its ID.
func (h *Harness) ReadyTicket(title string) string {
	h.t.Helper()
	return h.readyTicket(title)
}

// StageTicket creates a Ready Stage Ticket, marked by stage's Label, on
// the same shared Spec, and returns its ID.
func (h *Harness) StageTicket(stage, title string) string {
	h.t.Helper()
	return h.readyTicket(title, core.StageLabel(stage))
}

func (h *Harness) readyTicket(title string, labels ...string) string {
	h.t.Helper()
	ctx := context.Background()
	c := h.Server.Client
	if h.spec == "" {
		s, err := c.CreateSpec(ctx, h.Project, core.CreateSpecInput{Title: "Work"})
		must(h.t, err)
		h.spec = s.ID
	}
	tk, err := c.CreateTicket(ctx, h.Project, core.CreateTicketInput{Spec: core.Ref(h.spec), Title: title, State: store.Ready, Labels: labels})
	must(h.t, err)
	return tk.ID
}

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
	deadline := time.Now().Add(Timeout)
	for !cond() {
		if time.Now().After(deadline) {
			h.t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// WaitForState waits for the Ticket to reach state.
func (h *Harness) WaitForState(id string, state store.State) core.TicketView {
	h.t.Helper()
	var tk core.TicketView
	h.WaitFor(fmt.Sprintf("%s to be %s", id, state), func() bool {
		tk = h.Ticket(id)
		return tk.State == state
	})
	return tk
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
	ticks          chan time.Time
	signals        chan os.Signal
	waits          atomic.Int64 // times the Factory has waited for a poll
	polls          int64        // ticks sent by Poll
}

// Start runs the Factory in the repo with args until Stop or the end of
// the test. Polls after the first happen only on Tick.
func (h *Harness) Start(args ...string) *Running {
	return h.StartIn(h.Repo, args...)
}

// StartIn is Start from dir.
func (h *Harness) StartIn(dir string, args ...string) *Running {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Running{h: h, cancel: cancel, done: make(chan struct{}), stdout: &buffer{}, stderr: &buffer{}, ticks: make(chan time.Time), signals: make(chan os.Signal, 2)}
	env := factory.Env{
		Dir:         dir,
		Environ:     h.Environ(),
		UserHomeDir: func() (string, error) { return h.Home, nil },
		Stdout:      r.stdout,
		Stderr:      r.stderr,
		After: func(time.Duration) <-chan time.Time {
			r.waits.Add(1)
			return r.ticks
		},
		KillGrace: KillGrace,
		Backoff:   h.Backoff,
		// Session Events are posted often, so tests needn't wait a second.
		EventBatchEvery: EventBatchEvery,
		Signals:         r.signals,
	}
	go func() {
		defer close(r.done)
		r.code = factory.Run(ctx, args, env)
	}()
	h.t.Cleanup(func() { r.Stop() })
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
	r := h.StartIn(dir, args...)
	select {
	case <-r.done:
	case <-time.After(Timeout):
		h.t.Fatalf("the Factory didn't exit; stdout:\n%s\nstderr:\n%s", r.stdout, r.stderr)
	}
	return r.code, r.stdout.String(), r.stderr.String()
}

// Tick lets the Factory make its next poll.
func (r *Running) Tick() {
	select {
	case r.ticks <- time.Now():
	case <-r.done:
	case <-time.After(Timeout):
		r.h.t.Fatal("the Factory didn't wait for its next poll")
	}
}

// Poll waits for the Factory's current poll to finish, lets it make its
// next one and waits for that to finish too. Turns the poll started may
// still be running.
func (r *Running) Poll() {
	r.h.t.Helper()
	r.waitForPolls(r.polls + 1)
	r.Tick()
	r.polls++
	r.waitForPolls(r.polls + 1)
}

func (r *Running) waitForPolls(n int64) {
	r.h.t.Helper()
	r.h.WaitFor(fmt.Sprintf("poll %d to finish (stdout %q)", n, r.stdout), func() bool {
		select {
		case <-r.done:
			return true
		default:
		}
		return r.waits.Load() >= n
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

// Stop stops the Factory and returns its exit code.
func (r *Running) Stop() int {
	r.cancel()
	select {
	case <-r.done:
	case <-time.After(Timeout):
		r.h.t.Fatal("the Factory didn't stop")
	}
	return r.code
}

// Exited waits for the Factory to exit on its own and returns its exit
// code.
func (r *Running) Exited() int {
	r.h.t.Helper()
	select {
	case <-r.done:
	case <-time.After(Timeout):
		r.h.t.Fatalf("the Factory didn't exit; stdout:\n%s", r.stdout)
	}
	return r.code
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

// Step is one thing the fake claude does. In CLI and Git args, Write's
// content and Stdout, {{ticket}}, {{claim}} and {{session}} stand for the
// run's Ticket, Claim and session ID.
type Step struct {
	// CLI runs the code-central CLI with these args in the worktree.
	CLI []string `json:"cli,omitempty"`
	// Write writes a file in the worktree.
	Write *File `json:"write,omitempty"`
	// Git runs git on PATH with these args in the worktree.
	Git []string `json:"git,omitempty"`
	// Need fails the run unless this file is in the worktree.
	Need string `json:"need,omitempty"`
	// Stdout prints one line, such as a stream-json message.
	Stdout string `json:"stdout,omitempty"`
	// Exit exits with this code, skipping the rest.
	Exit *int `json:"exit,omitempty"`
	// Wait blocks until the test calls Continue for the run's Ticket. It
	// notes each stop signal it gets (see Signals): on Unix it ignores
	// SIGINT and exits 143 on SIGTERM; on Windows it ignores Ctrl-Break.
	Wait bool `json:"wait,omitempty"`
}

// File is a file a Write step writes, Name relative to the worktree.
type File struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// Wait blocks the run until Continue, so the turn stays running.
func Wait() Step { return Step{Wait: true} }

// PostRound posts a Round of one Question under the run's Claim.
func PostRound(title string) Step {
	q, _ := json.Marshal([]core.QuestionInput{{Title: title, Body: title}})
	return CLI("post-round", "{{ticket}}", "--claim", "{{claim}}", "--session", "{{session}}", "--questions", string(q))
}

// Continue lets the run of ticket now in a Wait step carry on.
func (h *Harness) Continue(ticket string) {
	h.t.Helper()
	must(h.t, os.WriteFile(filepath.Join(h.data, "continue", ticket), nil, 0o644))
	h.WaitFor(ticket+"'s run to carry on", func() bool {
		_, err := os.Stat(filepath.Join(h.data, "continue", ticket))
		return os.IsNotExist(err)
	})
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

// Signals is the signals runs of ticket got while in a Wait step, in
// order: "INT" and "TERM" on Unix, "BREAK" on Windows.
func (h *Harness) Signals(ticket string) []string {
	b, err := os.ReadFile(filepath.Join(h.data, "signals", ticket))
	if os.IsNotExist(err) {
		return nil
	}
	must(h.t, err)
	return strings.Fields(string(b))
}

// CLI calls the code-central CLI.
func CLI(args ...string) Step { return Step{CLI: args} }

// WriteFile writes name in the worktree with content.
func WriteFile(name, content string) Step { return Step{Write: &File{Name: name, Content: content}} }

// Git runs git with args in the worktree.
func Git(args ...string) Step { return Step{Git: args} }

// NeedFile fails the run unless name is in the worktree.
func NeedFile(name string) Step { return Step{Need: name} }

// Finish finishes the run's Ticket under its Claim.
func Finish(summary string) Step {
	return CLI("finish", "{{ticket}}", "--claim", "{{claim}}", "--summary", summary)
}

// Result prints the stream-json result message with this subtype.
func Result(subtype string) Step {
	return Step{Stdout: `{"type":"result","subtype":"` + subtype + `","is_error":` + fmt.Sprint(subtype != "success") + `,"session_id":"{{session}}","result":"Turn over."}`}
}

// Denial is one call auto mode denied, as a result's permission_denials
// lists it.
type Denial struct {
	Tool  string         `json:"tool_name"`
	ID    string         `json:"tool_use_id"`
	Input map[string]any `json:"tool_input"`
}

// Denied is a denied Bash call running command.
func Denied(command string) Denial {
	return Denial{Tool: "Bash", ID: "toolu_" + fmt.Sprint(len(command)), Input: map[string]any{"command": command}}
}

// ResultDenied prints a success result listing these denied calls.
func ResultDenied(denials ...Denial) Step {
	b, _ := json.Marshal(map[string]any{
		"type": "result", "subtype": "success", "is_error": false, "session_id": "{{session}}",
		"result": "I couldn't do that.", "permission_denials": denials,
	})
	return Step{Stdout: string(b)}
}

// APIError prints the stream-json assistant message claude prints when an
// API call fails for good, with this error code ("rate_limit",
// "authentication_failed", ...).
func APIError(code string) Step {
	b, _ := json.Marshal(map[string]any{
		"type":    "assistant",
		"message": map[string]any{"role": "assistant", "content": []map[string]string{{"type": "text", "text": "API Error: " + code}}},
		"error":   code,
	})
	return Step{Stdout: string(b)}
}

// Say prints a stream-json assistant message with this text.
func Say(text string) Step {
	b, _ := json.Marshal(map[string]any{
		"type":    "assistant",
		"message": map[string]any{"role": "assistant", "content": []map[string]string{{"type": "text", "text": text}}},
	})
	return Step{Stdout: string(b)}
}

// ToolUse prints a stream-json assistant message calling tool with input,
// a JSON object kept as written so its key order holds.
func ToolUse(tool, input string) Step {
	b, _ := json.Marshal(map[string]any{
		"type": "assistant",
		"message": map[string]any{"role": "assistant", "content": []map[string]any{
			{"type": "tool_use", "id": "toolu_" + tool, "name": tool, "input": json.RawMessage(input)},
		}},
	})
	return Step{Stdout: string(b)}
}

// Think prints a stream-json assistant message with a thinking block.
func Think(thinking string) Step {
	b, _ := json.Marshal(map[string]any{
		"type":    "assistant",
		"message": map[string]any{"role": "assistant", "content": []map[string]string{{"type": "thinking", "thinking": thinking}}},
	})
	return Step{Stdout: string(b)}
}

// ToolResult prints a stream-json user message answering the tool call
// toolUseID with content, a string or a list of content blocks.
func ToolResult(toolUseID string, content any, isError bool) Step {
	b, _ := json.Marshal(map[string]any{
		"type": "user",
		"message": map[string]any{"role": "user", "content": []map[string]any{
			{"type": "tool_result", "tool_use_id": toolUseID, "content": content, "is_error": isError},
		}},
	})
	return Step{Stdout: string(b)}
}

// Exit exits with code.
func Exit(code int) Step { return Step{Exit: &code} }

// DefaultScript is what the fake does for a Ticket with no Script: finish
// the Ticket, then end the turn with a success result.
var DefaultScript = []Step{Finish("Done."), Result("success")}

// Script sets what the fake does on every run for ticket.
func (h *Harness) Script(ticket string, steps ...Step) {
	b, err := json.Marshal(steps)
	must(h.t, err)
	must(h.t, os.WriteFile(filepath.Join(h.data, "scripts", ticket+".json"), b, 0o644))
}

// Call is one run of the fake claude.
type Call struct {
	Args    []string `json:"args"`
	Dir     string   `json:"dir"`
	Prompt  string   `json:"prompt"`
	Ticket  string   `json:"ticket"`
	Claim   string   `json:"claim"`
	Session string   `json:"session"`
}

// Flag is the value after flag in the call's args, and whether it is there.
func (c Call) Flag(flag string) (string, bool) {
	for i, a := range c.Args {
		if a == flag && i+1 < len(c.Args) {
			return c.Args[i+1], true
		}
	}
	return "", false
}

// Has reports whether arg is one of the call's args.
func (c Call) Has(arg string) bool {
	for _, a := range c.Args {
		if a == arg {
			return true
		}
	}
	return false
}

// Calls is every run of the fake claude so far, oldest first.
func (h *Harness) Calls() []Call {
	h.t.Helper()
	dir := filepath.Join(h.data, "calls")
	entries, err := os.ReadDir(dir)
	must(h.t, err)
	var out []Call
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		must(h.t, err)
		var c Call
		must(h.t, json.Unmarshal(b, &c))
		out = append(out, c)
	}
	return out
}

var (
	ticketLine = regexp.MustCompile(`(?m)^Ticket: (\S+)$`)
	claimLine  = regexp.MustCompile(`(?m)^Claim: (\S+)$`)
)

// runFakeGit is the fake git: it reports the version FakeGit set and runs
// the real git for everything else.
func runFakeGit(data string, args []string) int {
	var g fakeGit
	b, err := os.ReadFile(filepath.Join(data, "git.json"))
	if err == nil {
		err = json.Unmarshal(b, &g)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake git:", err)
		return 70
	}
	if len(args) > 0 && (args[0] == "version" || args[0] == "--version") {
		fmt.Println("git version " + g.Version)
		return 0
	}
	cmd := exec.Command(g.Git, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode()
		}
		fmt.Fprintln(os.Stderr, "fake git:", err)
		return 70
	}
	return 0
}

// runFakeClaude is the fake claude: claude [flags] <prompt>.
func runFakeClaude(data string, args []string) int {
	wd, _ := os.Getwd()
	c := Call{Args: args, Dir: wd}
	if len(args) > 0 {
		c.Prompt = args[len(args)-1]
	}
	if m := ticketLine.FindStringSubmatch(c.Prompt); m != nil {
		c.Ticket = m[1]
	}
	if m := claimLine.FindStringSubmatch(c.Prompt); m != nil {
		c.Claim = m[1]
	}
	if s, ok := c.Flag("--session-id"); ok {
		c.Session = s
	} else if s, ok := c.Flag("--resume"); ok {
		c.Session = s
	}
	steps := DefaultScript
	if b, err := os.ReadFile(filepath.Join(data, "scripts", c.Ticket+".json")); err == nil {
		steps = nil
		if err := json.Unmarshal(b, &steps); err != nil {
			fmt.Fprintln(os.Stderr, "fake claude:", err)
			return 70
		}
	}
	// A test stops a waiting run as soon as it sees the call, so take the
	// stop signals before recording it: none is lost before Wait starts.
	sigs := make(chan os.Signal, 2)
	if slices.ContainsFunc(steps, func(s Step) bool { return s.Wait }) {
		signal.Notify(sigs, waitSignals...)
		defer signal.Stop(sigs)
	}
	b, _ := json.Marshal(c)
	name := fmt.Sprintf("%020d-%d.json", time.Now().UnixNano(), os.Getpid())
	// Write beside calls/ and rename in, so Calls never reads half a file.
	tmp := filepath.Join(data, name)
	err := os.WriteFile(tmp, b, 0o644)
	if err == nil {
		err = os.Rename(tmp, filepath.Join(data, "calls", name))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake claude:", err)
		return 70
	}

	fill := strings.NewReplacer("{{ticket}}", c.Ticket, "{{claim}}", c.Claim, "{{session}}", c.Session).Replace
	fillAll := func(args []string) []string {
		var filled []string
		for _, a := range args {
			filled = append(filled, fill(a))
		}
		return filled
	}
	fmt.Println(`{"type":"system","subtype":"init","session_id":"` + c.Session + `"}`)
	for _, s := range steps {
		switch {
		case s.Exit != nil:
			return *s.Exit
		case s.Wait:
			if code, stop := wait(data, c.Ticket, sigs); stop {
				return code
			}
		case s.Stdout != "":
			fmt.Println(fill(s.Stdout))
		case s.Write != nil:
			if err := os.WriteFile(s.Write.Name, []byte(fill(s.Write.Content)), 0o644); err != nil {
				fmt.Fprintln(os.Stderr, "fake claude:", err)
				return 70
			}
		case s.Git != nil:
			filled := fillAll(s.Git)
			cmd := exec.Command("git", filled...)
			cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "fake claude: git %s: %v\n", strings.Join(filled, " "), err)
				return 70
			}
		case s.Need != "":
			if _, err := os.Stat(s.Need); err != nil {
				fmt.Fprintln(os.Stderr, "fake claude: need", s.Need+":", err)
				return 70
			}
		case s.CLI != nil:
			filled := fillAll(s.CLI)
			code := cli.Run(context.Background(), filled, cli.Env{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: os.Stderr, Getenv: os.Getenv})
			if code != 0 {
				fmt.Fprintf(os.Stderr, "fake claude: code-central %s exited %d\n", strings.Join(filled, " "), code)
				return 70
			}
		}
	}
	return 0
}

// wait is the fake's Wait step: it returns when the test continues the
// run, or with stop set when a signal on sigs ends it.
func wait(data, ticket string, sigs <-chan os.Signal) (code int, stop bool) {
	note := func(s string) {
		f, err := os.OpenFile(filepath.Join(data, "signals", ticket), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintln(f, s)
			f.Close()
		}
	}
	cont := filepath.Join(data, "continue", ticket)
	for {
		select {
		case s := <-sigs:
			name, ends := signalName(s)
			note(name)
			if ends {
				return 143, true
			}
		case <-time.After(5 * time.Millisecond):
			if os.Remove(cont) == nil {
				return 0, false
			}
		}
	}
}
