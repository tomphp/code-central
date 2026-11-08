// Package factorytest is the test harness for the local Factory: a real
// Code Central from codecentraltest, a real temporary git repo set up as
// a Project, and a fake `claude` on PATH. The fake is the test binary
// itself, run again through a shell shim: it records its arguments,
// follows a Script (print stream-json, call the code-central CLI
// in-process, run a shell command, exit) and needs nothing built.
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
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/tomoram/code-central/cli"
	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory"
	"github.com/tomoram/code-central/store"
)

// fakeEnv names the harness folder to a test binary run as the fake
// claude. The shim sets it; the Factory never sees it.
const fakeEnv = "FACTORYTEST_FAKE_CLAUDE"

// Main runs the fake claude when the test binary was started as one, and
// the tests otherwise.
func Main(m *testing.M) {
	if dir := os.Getenv(fakeEnv); dir != "" {
		os.Exit(runFake(dir, os.Args[1:]))
	}
	os.Exit(m.Run())
}

// Timeout bounds every wait in the harness.
const Timeout = 10 * time.Second

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
	// Home is the Factory's HOME, so its state file is under it.
	Home string
	// Bin is the folder on PATH holding the fake claude (and FakeGit's git).
	Bin  string
	data string // scripts/ and calls/
	env  map[string]string
	spec string
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
	}
	for _, d := range []string{h.Repo, h.Home, h.Bin, filepath.Join(h.data, "scripts"), filepath.Join(h.data, "calls"), filepath.Join(h.data, "continue"), filepath.Join(h.data, "signals")} {
		must(t, os.MkdirAll(d, 0o755))
	}
	gitPath, err := exec.LookPath("git")
	must(t, err)
	h.env = map[string]string{
		"PATH":                h.Bin + string(os.PathListSeparator) + filepath.Dir(gitPath) + ":/usr/bin:/bin",
		"HOME":                h.Home,
		"CODE_CENTRAL_URL":    h.Server.URL,
		"GIT_CONFIG_NOSYSTEM": "1",
		"GIT_AUTHOR_NAME":     "Factory Test",
		"GIT_AUTHOR_EMAIL":    "factory@example.com",
		"GIT_COMMITTER_NAME":  "Factory Test",
		"GIT_COMMITTER_EMAIL": "factory@example.com",
		"TMPDIR":              os.TempDir(),
	}
	self, err := os.Executable()
	must(t, err)
	h.writeShim("claude", fmt.Sprintf("#!/bin/sh\n%s=%q exec %q \"$@\"\n", fakeEnv, h.data, self))

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

func must(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func (h *Harness) writeShim(name, body string) {
	must(h.t, os.WriteFile(filepath.Join(h.Bin, name), []byte(body), 0o755))
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
func (h *Harness) RemoveClaude() { must(h.t, os.Remove(filepath.Join(h.Bin, "claude"))) }

// FakeGit puts a git on PATH that reports version and otherwise runs the
// real git.
func (h *Harness) FakeGit(version string) {
	real, err := exec.LookPath("git")
	must(h.t, err)
	h.writeShim("git", fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = version ] || [ \"$1\" = --version ]; then echo 'git version %s'; exit 0; fi\nexec %q \"$@\"\n", version, real))
}

// Git runs the real git in the repo and returns its trimmed output.
func (h *Harness) Git(args ...string) string { return h.GitIn(h.Repo, args...) }

// GitIn runs the real git in dir and returns its trimmed output.
func (h *Harness) GitIn(dir string, args ...string) string {
	h.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = h.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		h.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// Worktree is where the Factory keeps ticket's worktree.
func (h *Harness) Worktree(ticket string) string { return h.Repo + ".worktrees/" + ticket }

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

// Running is a Factory running in-process.
type Running struct {
	h              *Harness
	cancel         context.CancelFunc
	done           chan struct{}
	code           int
	stdout, stderr *buffer
	ticks          chan time.Time
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
	r := &Running{h: h, cancel: cancel, done: make(chan struct{}), stdout: &buffer{}, stderr: &buffer{}, ticks: make(chan time.Time)}
	env := factory.Env{
		Dir:     dir,
		Environ: h.Environ(),
		Stdout:  r.stdout,
		Stderr:  r.stderr,
		After: func(time.Duration) <-chan time.Time {
			r.waits.Add(1)
			return r.ticks
		},
		KillGrace: KillGrace,
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

// Step is one thing the fake claude does. In CLI args, Shell and Stdout,
// {{ticket}}, {{claim}} and {{session}} stand for the run's Ticket, Claim
// and session ID.
type Step struct {
	// CLI runs the code-central CLI with these args in the worktree.
	CLI []string `json:"cli,omitempty"`
	// Shell runs sh -c in the worktree.
	Shell string `json:"shell,omitempty"`
	// Stdout prints one line, such as a stream-json message.
	Stdout string `json:"stdout,omitempty"`
	// Exit exits with this code, skipping the rest.
	Exit *int `json:"exit,omitempty"`
	// Wait blocks until the test calls Continue for the run's Ticket. It
	// notes each SIGINT and SIGTERM it gets (see Signals), ignores SIGINT
	// and exits 143 on SIGTERM.
	Wait bool `json:"wait,omitempty"`
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
	cmd := exec.Command("/bin/sh", "-c", "sleep 60; :", "claude", "--resume", session)
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
// order: "INT" and "TERM".
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

// Shell runs a shell command in the worktree.
func Shell(cmd string) Step { return Step{Shell: cmd} }

// Finish finishes the run's Ticket under its Claim.
func Finish(summary string) Step {
	return CLI("finish", "{{ticket}}", "--claim", "{{claim}}", "--summary", summary)
}

// Result prints the stream-json result message with this subtype.
func Result(subtype string) Step {
	return Step{Stdout: `{"type":"result","subtype":"` + subtype + `","is_error":` + fmt.Sprint(subtype != "success") + `,"session_id":"{{session}}","result":"Turn over."}`}
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

// runFake is the fake claude: claude [flags] <prompt>.
func runFake(data string, args []string) int {
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

	steps := DefaultScript
	if b, err := os.ReadFile(filepath.Join(data, "scripts", c.Ticket+".json")); err == nil {
		steps = nil
		if err := json.Unmarshal(b, &steps); err != nil {
			fmt.Fprintln(os.Stderr, "fake claude:", err)
			return 70
		}
	}
	fill := strings.NewReplacer("{{ticket}}", c.Ticket, "{{claim}}", c.Claim, "{{session}}", c.Session).Replace
	fmt.Println(`{"type":"system","subtype":"init","session_id":"` + c.Session + `"}`)
	for _, s := range steps {
		switch {
		case s.Exit != nil:
			return *s.Exit
		case s.Wait:
			if code, stop := wait(data, c.Ticket); stop {
				return code
			}
		case s.Stdout != "":
			fmt.Println(fill(s.Stdout))
		case s.Shell != "":
			cmd := exec.Command("/bin/sh", "-c", fill(s.Shell))
			cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Fprintln(os.Stderr, "fake claude: shell:", err)
				return 70
			}
		case s.CLI != nil:
			var filled []string
			for _, a := range s.CLI {
				filled = append(filled, fill(a))
			}
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
// run, or with stop set when SIGTERM ends it.
func wait(data, ticket string) (code int, stop bool) {
	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigs)
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
			if s == syscall.SIGTERM {
				note("TERM")
				return 143, true
			}
			note("INT")
		case <-time.After(5 * time.Millisecond):
			if os.Remove(cont) == nil {
				return 0, false
			}
		}
	}
}
