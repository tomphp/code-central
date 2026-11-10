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
	"fmt"
	"io"
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
	// Clock is the Factory's time for an outage's backoff and for dating
	// model responses: it stands still until the test moves it, so a
	// backoff is over only when the test says so.
	Clock *clock.Fake
	// URL is the Code Central the Factory and its sessions reach: Server
	// behind a proxy that SetDown can take down.
	URL       string
	down      atomic.Bool
	failing   atomic.Value // string: a path suffix the proxy fails
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
	// Code Central's clock starts where the Factory's does, so the model
	// responses the Factory dates fall on Code Central's today in Usage.
	now := time.Now()
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
// config, and a fixed author and committer.
var gitIdentity = map[string]string{
	"GIT_CONFIG_NOSYSTEM": "1",
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
	ps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if suffix, _ := h.failing.Load().(string); suffix != "" && strings.HasSuffix(r.URL.Path, suffix) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"code":"internal","message":"failing for the test"}`)
			return
		}
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

// SetFailing makes Code Central fail the Factory's and its sessions'
// requests to every path ending in suffix, such as "/model-choice", or
// stops failing them given "".
func (h *Harness) SetFailing(suffix string) { h.failing.Store(suffix) }

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

// LabelledTicket creates a Ready Ticket carrying label, such as a
// Wayfinder Label or prototype, on the same shared Spec, and returns its
// ID.
func (h *Harness) LabelledTicket(label, title string) string {
	h.t.Helper()
	return h.readyTicket(title, label)
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

// Usage reads Usage for the Project over the default range.
func (h *Harness) Usage() core.UsageView {
	h.t.Helper()
	u, err := h.Server.Client.Usage(context.Background(), h.Project, 0)
	must(h.t, err)
	return u
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
// the test, and returns once its first poll has finished (or it has
// exited), so whatever the test sets up next is first seen by a later
// poll. Polls after the first happen only on Tick.
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
		Now:       h.Clock.Now,
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
	r := h.launch(dir, args...)
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
