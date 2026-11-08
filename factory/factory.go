// Package factory is the local Factory, code-central-factory (ADR 0005):
// plain code that claims a Project's Ready Tickets through the HTTP API
// and runs one `claude -p` session per Claim in the Ticket's worktree.
package factory

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tomoram/code-central/cli"
	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/home"
)

// Env is everything the Factory touches outside itself, so tests can run
// it in-process.
type Env struct {
	// Dir is the working directory.
	Dir string
	// Environ is the environment: it is read for PATH, HOME and the
	// CLI's variables, and given to every child.
	Environ []string
	// UserHomeDir is the profile folder the state file falls back to when
	// HOME isn't set (home.Lookup); main passes os.UserHomeDir. nil means
	// none.
	UserHomeDir    func() (string, error)
	Stdout, Stderr io.Writer
	// After replaces time.After for the wait between polls.
	After func(time.Duration) <-chan time.Time
	// KillGrace is how long a turn being stopped has after each stop step
	// but the last (SIGINT, SIGTERM; Ctrl-Break on Windows); zero means
	// ten seconds.
	KillGrace time.Duration
	// Backoff is the first wait before resuming turns an outage cut off,
	// doubling with each outage in a row; zero means a minute.
	Backoff time.Duration
	// EventBatchEvery is how often waiting Session Events are posted;
	// zero means eventBatchEvery.
	EventBatchEvery time.Duration
	// Signals is the stop signals the Factory gets: the first Ctrl-C
	// (os.Interrupt) drains; a second, SIGTERM or SIGHUP stops the turns
	// at once. Nil means none come.
	Signals <-chan os.Signal
}

func (e Env) getenv(key string) string {
	for i := len(e.Environ) - 1; i >= 0; i-- {
		if k, v, ok := strings.Cut(e.Environ[i], "="); ok && k == key {
			return v
		}
	}
	return ""
}

// lookPath finds name on the environment's PATH.
func (e Env) lookPath(name string) (string, bool) {
	for _, d := range filepath.SplitList(e.getenv("PATH")) {
		p := filepath.Join(d, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
			return p, true
		}
	}
	return "", false
}

// Exit codes.
const (
	exitOK      = 0
	exitRefused = 1
	// exitStopped is a Factory that stopped itself, as on an expired login.
	exitStopped = 2
)

// options are the Factory's flags. There is no config file.
type options struct {
	parallel int
	poll     time.Duration
	bypass   bool
	model    string
	base     string
	project  string
	verbose  bool
}

func parseFlags(args []string, stderr io.Writer) (options, error) {
	var o options
	fs := flag.NewFlagSet("code-central-factory", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.IntVar(&o.parallel, "parallel", 3, "the most claude turns running at once (at least 1)")
	fs.DurationVar(&o.poll, "poll", 30*time.Second, "how often to look for work (at least 5s)")
	fs.BoolVar(&o.bypass, "bypass-permissions", false, "run every session in bypassPermissions mode")
	fs.StringVar(&o.model, "model", "", "the model passed to every start and resume")
	fs.StringVar(&o.base, "base", "", "the branch work starts from (default: the branch checked out)")
	fs.BoolVar(&o.verbose, "verbose", false, "print a line for each tool call the agents make")
	fs.StringVar(&o.project, "project", "", "Project Key (default $CODE_CENTRAL_PROJECT, else the nearest .mcp.json)")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if fs.NArg() > 0 {
		return o, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if o.parallel < 1 {
		return o, errors.New("--parallel must be at least 1")
	}
	if o.poll < 5*time.Second {
		return o, errors.New("--poll must be at least 5s")
	}
	return o, nil
}

// Run runs the Factory until ctx ends and returns its exit code.
func Run(ctx context.Context, args []string, e Env) int {
	if e.After == nil {
		e.After = time.After
	}
	o, err := parseFlags(args, e.Stderr)
	if err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(e.Stderr, "code-central-factory:", err)
		}
		return exitRefused
	}
	f, err := start(e, o)
	if err != nil {
		fmt.Fprintln(e.Stderr, "code-central-factory: refusing to start:", err)
		return exitRefused
	}
	defer f.lock.Close()
	f.printf("base: %s", f.base)
	if o.bypass {
		f.printf("warning: --bypass-permissions: every session runs in bypassPermissions mode, with no permission checks")
	}
	f.prune()
	ctx, f.stop = context.WithCancel(ctx)
	defer f.stop()
	go f.watchSignals(ctx)
	go f.events.run()
	f.restart(ctx)
	f.loop(ctx)
	// Stopping or draining makes one last flush.
	f.events.close()
	if reason := f.stoppedFor(); reason != "" {
		fmt.Fprintln(e.Stderr, "code-central-factory: stopped:", reason)
		return exitStopped
	}
	return exitOK
}

// stopFor stops the Factory for a reason only the human can fix: the
// loop ends and running turns are stopped. Claims are kept.
func (f *factory) stopFor(reason string) {
	f.stopMu.Lock()
	if f.stopReason == "" {
		f.stopReason = reason
		f.printf("stopping: %s", reason)
	}
	f.stopMu.Unlock()
	f.stop()
}

func (f *factory) stoppedFor() string {
	f.stopMu.Lock()
	defer f.stopMu.Unlock()
	return f.stopReason
}

// printf prints one line to stdout; turns print from their own goroutines.
func (f *factory) printf(format string, args ...any) {
	f.outMu.Lock()
	defer f.outMu.Unlock()
	fmt.Fprintf(f.e.Stdout, format+"\n", args...)
}

// factory is a started Factory.
type factory struct {
	e         Env
	o         options
	git       string
	claude    string
	repo      string // the git top level
	project   string
	base      string
	worktrees string // <repo-parent>/<repo-dir>.worktrees
	client    *client.Client
	events    *sender // posts Session Events
	state     *stateFile
	lock      *os.File // held for the Factory's whole life

	outMu     sync.Mutex
	mu        sync.Mutex           // guards turns, ended and info
	turns     map[string]*turn     // running turns by Claim ID
	ended     map[string]bool      // Claim IDs never to be written with again
	info      map[string]claimInfo // what the lines say of each held Claim, by Claim ID
	warned    map[string]bool      // dirty finished worktrees already warned of; the poll's own
	turnsDone sync.WaitGroup

	// warnedMerged is "<ticket>@<commit>" of Tickets merged in a State
	// that can't land, already warned of; the poll's own.
	warnedMerged map[string]bool

	// restarted is the saved Claims a stop cut off, to resume as cut off
	// once a slot is free; the poll's own.
	restarted []claim

	pauseMu sync.Mutex
	pause   *pause // set during an outage

	// stop ends the loop early; stopReason says why, for a Factory that
	// stops itself.
	stop       context.CancelFunc
	stopMu     sync.Mutex
	stopReason string

	// drainCh is closed by the first Ctrl-C: no more claiming, resuming
	// or landing, and the Factory exits once its running turns end.
	drainCh   chan struct{}
	drainOnce sync.Once
}

// start makes every start-up check, refusing with the reason.
func start(e Env, o options) (*factory, error) {
	f := &factory{e: e, o: o, drainCh: make(chan struct{}), turns: map[string]*turn{}, ended: map[string]bool{}, info: map[string]claimInfo{}, warned: map[string]bool{}, warnedMerged: map[string]bool{}}
	git, ok := e.lookPath("git")
	if !ok {
		return nil, errors.New("git isn't on PATH")
	}
	f.git = git
	top, err := f.gitOut(e.Dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("not in a git repository (%s)", e.Dir)
	}
	f.repo = top
	if err := f.checkGitVersion(); err != nil {
		return nil, err
	}
	project, err := cli.ResolveProject(e.Dir, o.project, e.getenv)
	if err != nil {
		return nil, err
	}
	if project == "" {
		return nil, errors.New("no Project: pass --project <key>, set CODE_CENTRAL_PROJECT, or run `code-central init` in the repository")
	}
	f.project = project
	if f.base = o.base; f.base == "" {
		b, err := f.gitOut(top, "symbolic-ref", "--short", "-q", "HEAD")
		if err != nil || b == "" {
			return nil, errors.New("HEAD is detached: check out the branch work should start from, or pass --base <branch>")
		}
		f.base = b
	} else if _, err := f.gitOut(top, "rev-parse", "--verify", "-q", f.base+"^{commit}"); err != nil {
		return nil, fmt.Errorf("--base %s isn't a commit in this repository", f.base)
	}
	if _, err := f.gitOut(top, "cat-file", "-e", f.base+":.mcp.json"); err != nil {
		return nil, fmt.Errorf(".mcp.json isn't committed on %s: every worktree starts from %s, so its sessions need it; commit it first", f.base, f.base)
	}
	if f.claude, ok = e.lookPath("claude"); !ok {
		return nil, errors.New("claude isn't on PATH")
	}
	f.worktrees = filepath.Join(filepath.Dir(top), filepath.Base(top)+".worktrees")
	if err := writable(f.worktrees); err != nil {
		return nil, fmt.Errorf("can't use the worktree folder %s: %v", f.worktrees, err)
	}
	f.client = client.New(cli.ServiceURL(e.getenv))
	every := e.EventBatchEvery
	if every <= 0 {
		every = eventBatchEvery
	}
	f.events = newSender(f.client, every)
	statePath, err := home.Lookup{Getenv: e.getenv, UserHomeDir: e.UserHomeDir}.State("code-central-factory", project+".json")
	if err != nil {
		return nil, fmt.Errorf("can't find the state folder: %w", err)
	}
	f.state = &stateFile{path: statePath}
	if err := os.MkdirAll(filepath.Dir(f.state.path), 0o755); err != nil {
		return nil, fmt.Errorf("can't make the state folder: %v", err)
	}
	if f.lock, err = f.state.lock(project); err != nil {
		return nil, err
	}
	if err := f.state.load(); err != nil {
		f.lock.Close()
		return nil, fmt.Errorf("can't read the state file: %v", err)
	}
	return f, nil
}

// minGit is the oldest git the Factory runs with: landing needs
// `git merge-tree --write-tree`.
var minGit = [2]int{2, 38}

func (f *factory) checkGitVersion() error {
	out, err := f.gitOut(f.repo, "version")
	if err != nil {
		return err
	}
	// "git version 2.39.3 (Apple Git-146)"
	fields := strings.Fields(out)
	if len(fields) < 3 {
		return fmt.Errorf("can't read the git version from %q", out)
	}
	v := fields[2]
	parts := strings.SplitN(v, ".", 3)
	var major, minor int
	if len(parts) < 2 {
		return fmt.Errorf("can't read the git version from %q", out)
	}
	if _, err := fmt.Sscan(parts[0], &major); err != nil {
		return fmt.Errorf("can't read the git version from %q", out)
	}
	if _, err := fmt.Sscan(parts[1], &minor); err != nil {
		return fmt.Errorf("can't read the git version from %q", out)
	}
	if major < minGit[0] || major == minGit[0] && minor < minGit[1] {
		return fmt.Errorf("git %s is older than %d.%d", v, minGit[0], minGit[1])
	}
	return nil
}

// writable creates dir if need be and checks a file can be made in it.
func writable(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".write-check-*")
	if err != nil {
		return err
	}
	tmp.Close()
	return os.Remove(tmp.Name())
}

// gitOut runs git in dir and returns its trimmed output.
func (f *factory) gitOut(dir string, args ...string) (string, error) {
	cmd := exec.Command(f.git, args...)
	cmd.Dir = dir
	cmd.Env = f.e.Environ
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}
