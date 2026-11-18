// Package factory is the local Factory, code-central-factory (ADR 0005):
// plain code that claims a Project's Ready Tickets through the HTTP API
// and runs one `claude -p` session per Claim in the Ticket's worktree.
//
// # Parts
//
// The Factory is two parts over one shared registry (Spec
// code-central-934), all unexported types in this package:
//
//   - repositories (repositories.go) is the registry: the Home
//     Repository, the Project's Repositories and its base, and the
//     worktree set-up. Both parts read it; only the lander writes it
//     (replace after a sync, setBase on a Project read).
//   - lander (lander.go) is the landing part: each poll it reads the
//     Project and its Landing Policy, keeps the clones and bases current,
//     lands Accepted work, detects hand merges and merged pull requests,
//     and sweeps the worktrees and Spec Branches it is done with. It owns
//     every "already said" map and all base state, and starts no session.
//   - *factory is the session part: the poll loop, slots and turns,
//     Claims, Chat, Heartbeats, outages, Stage set-up, and the Changed
//     Repositories report and finish check. It holds the registry and the
//     lander as fields.
//
// # The interface between the parts
//
// The session part gives the lander landerHooks, the only things landing
// asks of sessions:
//
//   - stopped: the Factory is halted, draining or paused, so nothing more
//     lands this poll. The landing loops ask it between steps.
//   - turnRunningOn: a turn is running on the Ticket, so the sweep keeps
//     its worktrees.
//
// newLander builds the lander once, in start, over the registry, the
// Client, the Project Key, the shared work hook, the shared output (the
// factory's printf, under its one output mutex) and the hooks. The
// session part then calls only two methods on it:
//
//   - poll, once per poll, after reconciling and before claiming. It
//     returns the Project and whether it was read; when it was, the
//     session part makes the report of what claude accepts (reportModels)
//     until Code Central takes it. Landing never makes that report.
//   - claimingHeld, wherever claiming is gated: a base is missing or has
//     diverged, .mcp.json is missing from the base, or a Repository is
//     missing.
//
// The rules that keep the split from changing behaviour:
//
//   - One call to poll per poll, in the place the landing step ran
//     before the split.
//   - Both parts write lines through the same output and report work in
//     flight through the same work value, so a test that settles on
//     Env.Work (ADR 0045) hears landing's git and post steps too. The
//     lander has no timer of its own and takes no clock; every wait stays
//     on Env.Clock in the session part.
//   - The base name is written into the registry by the lander on a
//     Project read, and read by turns there under the registry's mutex.
//   - ADR 0005 is untouched: the Factory is plain code that starts one
//     session per Claim, and the lander starts none.
package factory

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/home"
	"github.com/tomoram/code-central/store"
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
	// Clock is every timer and timestamp of the Factory's: the wait
	// between polls, the retry pauses, the batch posts, the kill grace,
	// an outage's backoff, Session Events' times and the dates of the
	// model responses posted to Usage. Nil means clock.System. Only the
	// deadlines that bound a call to Code Central or a process stay on
	// real time.
	Clock clock.Clock
	// KillGrace is how long a turn being stopped has after each stop step
	// but the last (SIGINT, SIGTERM; Ctrl-Break on Windows); zero means
	// ten seconds.
	KillGrace time.Duration
	// Backoff is the first wait before resuming turns an outage cut off,
	// doubling with each outage in a row; zero means a minute.
	Backoff time.Duration
	// EventBatchEvery is how often waiting Session Events and token usage
	// are posted; zero means postEvery.
	EventBatchEvery time.Duration
	// EventPostTimeout bounds each post of Session Events and token
	// usage; zero means postTimeout. It is on real time, as postTimeout
	// is.
	EventPostTimeout time.Duration
	// Signals is the stop signals the Factory gets: the first Ctrl-C
	// (os.Interrupt) drains; a second, SIGTERM or SIGHUP stops the turns
	// at once. Nil means none come.
	Signals <-chan os.Signal
	// Work, if set, hears of the Factory's work in flight, so that a test
	// knows when it has settled. The binary sets none.
	Work Work
}

func (e Env) getenv(key string) string {
	for i := len(e.Environ) - 1; i >= 0; i-- {
		if k, v, ok := strings.Cut(e.Environ[i], "="); ok && k == key {
			return v
		}
	}
	return ""
}

// killGrace is how long a turn being stopped has after each stop step
// but the last (stopSteps).
func (e Env) killGrace() time.Duration {
	if e.KillGrace > 0 {
		return e.KillGrace
	}
	return 10 * time.Second
}

// home is where the state file is found.
func (e Env) home() home.Lookup {
	return home.Lookup{Getenv: e.getenv, UserHomeDir: e.UserHomeDir}
}

// lookPath finds name on the environment's PATH.
func (e Env) lookPath(name string) (string, bool) {
	for _, d := range filepath.SplitList(e.getenv("PATH")) {
		for _, ext := range e.pathExts() {
			p := filepath.Join(d, name+ext)
			if fi, err := os.Stat(p); err == nil && !fi.IsDir() && executable(fi) {
				return p, true
			}
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
	noChat   bool
	model    string
	effort   string
	project  string
	verbose  bool
	// claudeArgs are what follows -- on the command line: claude flags
	// every start and resume passes as given.
	claudeArgs []string
}

func parseFlags(args []string, stderr io.Writer) (options, error) {
	var o options
	fs := flag.NewFlagSet("code-central-factory", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.IntVar(&o.parallel, "parallel", 3, "the most claude turns running at once (at least 1)")
	fs.DurationVar(&o.poll, "poll", 30*time.Second, "how often to look for work when no wake from Code Central comes sooner (at least 5s)")
	fs.BoolVar(&o.bypass, "bypass-permissions", false, "run every session in bypassPermissions mode")
	fs.BoolVar(&o.noChat, "no-chat", false, "don't report Chat support: the Project's Chat is unavailable through this Factory")
	fs.StringVar(&o.model, "model", "", "the model for every start and resume a Ticket's model Tag doesn't set")
	fs.StringVar(&o.effort, "effort", "", "the effort for every start and resume a Ticket's effort Tag doesn't set")
	fs.String("base", "", "removed: the base is the Project's Base branch setting")
	fs.BoolVar(&o.verbose, "verbose", false, "print a line for each tool call the agents make")
	fs.StringVar(&o.project, "project", "", "Project Key (default $CODE_CENTRAL_PROJECT, else the nearest .mcp.json)")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: code-central-factory [flags] [-- <Claude Args>]")
		fmt.Fprintln(stderr, "\n<Claude Args> are claude flags, such as --chrome, passed as given to every start and resume.")
		fmt.Fprintln(stderr, "\nFlags:")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	refused := false
	fs.Visit(func(fl *flag.Flag) { refused = refused || fl.Name == "base" })
	if refused {
		return o, errors.New("--base is gone: the base is the Project's setting now; set it in Project Options' Base branch field")
	}
	if rest := fs.Args(); len(rest) > 0 {
		if i := len(args) - len(rest) - 1; i < 0 || args[i] != "--" {
			return o, fmt.Errorf("unexpected argument %q", rest[0])
		}
		if err := checkClaudeArgs(rest); err != nil {
			return o, err
		}
		o.claudeArgs = rest
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
	if e.Clock == nil {
		e.Clock = clock.System{}
	}
	// Busy from the first line until the first poll takes over.
	startingUp := work{e.Work}.busy("starting up")
	o, err := parseFlags(args, e.Stderr)
	if err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(e.Stderr, "code-central-factory:", err)
		}
		startingUp()
		return exitRefused
	}
	f, err := start(e, o)
	if err != nil {
		fmt.Fprintln(e.Stderr, "code-central-factory: refusing to start:", err)
		startingUp()
		return exitRefused
	}
	f.polling.end = startingUp
	defer f.lock.Close()
	f.printf("base: %s", f.repos.baseBranch())
	if len(o.claudeArgs) > 0 {
		f.printf("passing to every claude turn: %s", joinArgs(o.claudeArgs))
	}
	if o.bypass {
		f.printf("warning: --bypass-permissions: every session runs in bypassPermissions mode, with no permission checks")
	}
	f.repos.prune(f.repos.home)
	ctx, f.stop = context.WithCancel(ctx)
	defer f.stop()
	go f.watchSignals(ctx, f.starting("the signal watch"))
	f.events.start()
	f.usage.start()
	go f.heartbeats.run()
	chatDone := make(chan struct{})
	chatStarting := f.starting("the Chat loop")
	go func() {
		defer close(chatDone)
		f.chatLoop(ctx, chatStarting)
	}()
	wakeDone := make(chan struct{})
	wakeStarting := f.starting("the wake loop")
	go func() {
		defer close(wakeDone)
		f.wakeLoop(ctx, wakeStarting)
	}()
	f.restart(ctx)
	f.loop(ctx)
	// A running Chat reply ends before the last flushes.
	<-chatDone
	<-wakeDone
	// Stopping or draining makes one last flush of each, side by side, so
	// a hung Code Central holds up the stop one postTimeout at most.
	var flushed sync.WaitGroup
	// The stopping Heartbeat goes with them, under the same timeout.
	for _, close := range []func(){f.events.close, f.usage.close, func() { f.heartbeats.close(f.stopNote()) }} {
		flushed.Go(close)
	}
	flushed.Wait()
	if reason := f.stoppedFor(); reason != "" {
		fmt.Fprintln(e.Stderr, "code-central-factory: stopped:", reason)
		return exitStopped
	}
	return exitOK
}

// starting is the line of work of a goroutine about to start, busy
// starting what (its parts, joined) until it first rests.
func (f *factory) starting(what ...string) *stint {
	s := &stint{w: f.work}
	if f.work.hook != nil {
		s.end = f.work.busy("starting ", strings.Join(what, ""))
	}
	return s
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
	f.halt()
}

// halt stops the Factory, handing the stop's busy work to the poll loop,
// which takes it up once it has started stopping the turns, unless it
// has ended already.
func (f *factory) halt() {
	f.wakeMu.Lock()
	if !f.loopDone {
		f.stopWork.give(f.work.busy("stopping"))
	}
	f.wakeMu.Unlock()
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
	e      Env
	o      options
	claude string
	// repos is the registry of the Home Repository, the Project's
	// Repositories and its base, and of their worktrees.
	repos *repositories
	// lander is the landing part, which the poll calls once per poll and
	// asks whether claiming is held; start builds it.
	lander     *lander
	project    string
	host       string // the machine, as the Heartbeats name it
	client     *client.Client
	events     *sender[core.SessionEvent]  // posts Session Events
	usage      *sender[core.UsageResponse] // posts model responses' token usage
	planLimits latestPlanLimits            // the latest Plan Limits, sent on each Heartbeat
	state      *stateFile
	lock       *os.File // held for the Factory's whole life

	outMu sync.Mutex
	mu    sync.Mutex           // guards turns, ended, info and backInReady
	turns map[string]*turn     // running turns by Claim ID
	ended map[string]bool      // Claim IDs never to be written with again
	info  map[string]claimInfo // what the lines say of each held Claim, by Claim ID
	// backInReady is when each Ticket's turn ended with it back in Ready,
	// as when the agent gave up, by Ticket ID: it isn't claimed again
	// until the next timed poll, so the wake at its turn's end doesn't
	// claim it again at once, and again, without end.
	backInReady map[string]time.Time
	turnsDone   sync.WaitGroup

	// reportedChanged is the Changed Repositories last reported for each
	// held Claim, by Claim ID, so a report goes only when they change;
	// the poll's own.
	reportedChanged map[string]string
	// accepts is what claude --help said claude accepts, read at startup.
	accepts claudeAccepts
	// runner runs the claude Turns, built at startup once accepts is read.
	runner *turnRunner
	// modelsReported is whether Code Central has taken the report of
	// accepts; until it has, each poll that reads the Project tries.
	modelsReported bool

	// restarted is the saved Claims a stop cut off, to resume as cut off
	// once a slot is free; the poll's own.
	restarted []claim
	// lostClaims are the claims whose answer was lost on every try, oldest
	// first, sent again at each poll until Code Central answers; each
	// holds a slot meanwhile. They last this run only; the poll's own.
	lostClaims []core.ClaimInput

	pauseMu sync.Mutex
	pause   *pause // set during an outage

	// stop ends the loop early; stopReason says why, for a Factory that
	// stops itself.
	stop       context.CancelFunc
	stopMu     sync.Mutex
	stopReason string
	stopSignal os.Signal // the signal that stopped the Factory, if one did
	drained    bool      // the drain finished with every turn ended
	heartbeats *heartbeater

	// drainCh is closed by the first Ctrl-C: no more claiming, resuming
	// or landing, and the Factory exits once its running turns end.
	drainCh   chan struct{}
	drainOnce sync.Once
	// wakeCh holds a wake for the poll loop, as the end of its busy work:
	// one at most, so any number of wakes during a poll make one more
	// poll. pollsOver is set once the poll loop drains or stops, and
	// takes no more, and loopDone once it has ended, and takes up no
	// stop; wakeMu guards both, ordering a wake or a stop against them.
	wakeCh    chan func()
	wakeMu    sync.Mutex
	pollsOver bool
	loopDone  bool

	// work hears of the Factory's work in flight (Env.Work). polling is
	// the poll loop's line of it, and polls the number of polls so far;
	// both the poll's own. drainWork and stopWork hand the busy work of a
	// drain or a stop to the poll loop, which takes it up.
	work      work
	polling   stint
	polls     int
	drainWork handoff
	stopWork  handoff
}

// newFactory is a factory with nothing found out yet: start fills in the
// rest.
func newFactory(e Env, o options) *factory {
	w := work{e.Work}
	f := &factory{e: e, o: o, drainCh: make(chan struct{}), wakeCh: make(chan func(), 1), work: w, polling: stint{w: w}, drainWork: newHandoff(), stopWork: newHandoff(), turns: map[string]*turn{}, ended: map[string]bool{}, info: map[string]claimInfo{}, backInReady: map[string]time.Time{}, reportedChanged: map[string]string{}}
	f.repos = newRepositories(e.Environ, f.printf)
	return f
}

// hooks is what the landing part asks of the sessions: whether the
// Factory is halted, draining or paused, so nothing more lands this poll,
// and whether a turn is running on a Ticket, whose worktrees the sweep
// keeps.
func (f *factory) hooks() landerHooks {
	return landerHooks{
		stopped:       func(ctx context.Context) bool { return f.halted(ctx) || f.paused() },
		turnRunningOn: f.turnRunningOn,
	}
}

// start makes every start-up check, refusing with the reason.
func start(e Env, o options) (*factory, error) {
	f := newFactory(e, o)
	git, ok := e.lookPath("git")
	if !ok {
		return nil, errors.New("git isn't on PATH")
	}
	f.repos.git = git
	top, err := f.gitOut(e.Dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("not in a git repository (%s)", e.Dir)
	}
	// git writes C:/... on Windows; the Factory's paths are C:\...
	home := f.repos.newRepository("", filepath.FromSlash(top))
	home.home = true
	f.repos.home = home
	if err := f.checkGitVersion(); err != nil {
		return nil, err
	}
	project, err := client.ResolveProject(e.Dir, o.project, e.getenv)
	if err != nil {
		return nil, err
	}
	if project == "" {
		return nil, errors.New("no Project: pass --project <key>, set CODE_CENTRAL_PROJECT, or run `code-central init` in the repository")
	}
	f.project = project
	// The Home Repository is named by the Project Key.
	home.name = project
	f.repos.replace(map[string]*repository{project: home}, []*repository{home})
	if f.claude, ok = e.lookPath("claude"); !ok {
		return nil, errors.New("claude isn't on PATH")
	}
	if err := refuseClaudeShim(f.claude); err != nil {
		return nil, err
	}
	if err := writable(home.worktrees); err != nil {
		return nil, fmt.Errorf("can't use the worktree folder %s: %v", home.worktrees, err)
	}
	f.client = client.New(client.ServiceURL(e.getenv))
	every := e.EventBatchEvery
	if every <= 0 {
		every = postEvery
	}
	f.host = hostname()
	f.heartbeats = newFactoryHeartbeater(f)
	timeout := e.EventPostTimeout
	if timeout <= 0 {
		timeout = postTimeout
	}
	f.events = newSender(f.client.PostSessionEvents, e.Clock, every, timeout, f.work, "Session Events")
	f.usage = newSender(f.client.PostUsage, e.Clock, every, timeout, f.work, "token usage")
	f.lander = newLander(f.repos, f.client, project, f.work, f.printf, f.hooks())
	// A Project that can't be read yet is checked on every poll instead.
	p, readErr := f.client.GetProject(context.Background(), project)
	// The base is the Project's, read again on every poll; until it can be
	// read, it is the default.
	base := store.DefaultBase
	if readErr == nil {
		base = p.Landing.Base
	}
	f.repos.setBase(base)
	if err := f.lander.checkMCPAtStart(); err != nil {
		return nil, err
	}
	f.accepts = f.readClaudeHelp()
	if err := f.checkEffortFlag(); err != nil {
		return nil, err
	}
	f.runner = newTurnRunner(f.claude, e, f.accepts, o.bypass, o.claudeArgs, o.verbose)
	if err := f.checkClaudeArgsAccepted(); err != nil {
		return nil, err
	}
	if readErr == nil {
		f.lander.reportPushRemote(context.Background(), p)
		f.modelsReported = f.reportModels(context.Background())
	}
	statePath, err := e.home().State("code-central-factory", project+".json")
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
	out, err := f.gitOut(f.repos.home.dir, "version")
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
	return f.repos.gitOut(dir, args...)
}
