// Package watchers is the lifetime of every kind of Watcher. On the
// service's side, it runs one of each kind per Project whose setting is
// on, as its own executable's hidden subcommand, restarts one that
// crashes, and stops them all when it stops (ADR 0018); on the Watcher's
// side, a Watcher runs until its stdin closes. The CI Watcher is the only
// kind so far; a new kind is one more Kind.
package watchers

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/events"
)

// The restart backoff: a crashed Watcher is restarted after
// FirstBackoff, doubling after each crash to at most MaxBackoff, and
// back to FirstBackoff once one has run for ResetAfter.
const (
	FirstBackoff = time.Second
	MaxBackoff   = 5 * time.Minute
	ResetAfter   = core.WatcherSteadyAfter
)

// stopLimit bounds how long a Watcher has to exit once its stdin
// closes before it is killed.
const stopLimit = 10 * time.Second

// Projects is what the service's supervision reads and reports on Code
// Central; *core.Core is one.
type Projects interface {
	// WatchedProjects is the key of every Project that should have a
	// Watcher of kind.
	WatchedProjects(ctx context.Context, kind core.WatcherKind) []string
	// ReportWatcherCrash reports that key's Watcher of kind crashed.
	ReportWatcherCrash(ctx context.Context, kind core.WatcherKind, key string, crash core.WatcherCrash)
	// Events carries project.changed, on which the Watchers are started
	// and stopped.
	Events() *events.Bus
}

// Kind is how the service runs one kind of Watcher.
type Kind struct {
	Kind core.WatcherKind
	// Args are the arguments that run the service's own executable as
	// project's Watcher of this kind, in its watch mode.
	Args func(project string) []string
}

// Supervision wires the service's supervision of its Watchers.
type Supervision struct {
	Projects Projects
	// Kinds are the kinds of Watcher to run.
	Kinds []Kind
	// Program is the service's own executable, run with each Kind's Args.
	Program string
	// Environ is the Watchers' environment: the service's, with
	// CODE_CENTRAL_URL naming the service.
	Environ []string
	Clock   clock.Clock
	// Tick, when set, checks which Watchers should run on each tick too,
	// in case a project.changed was missed.
	Tick <-chan time.Time
	// Log receives each start, stop and crash, and what the Watchers
	// write.
	Log io.Writer
}

// projectWatcher names one Project's Watcher of one kind.
type projectWatcher struct {
	kind    Kind
	project string
}

// name is how the service's log names it, such as the CI Watcher for
// acme.
func (o projectWatcher) name() string { return "the " + o.kind.Kind.Name() + " for " + o.project }

// Supervise runs a Watcher of each kind for each Project WatchedProjects
// names, starting and stopping them as it changes, until ctx ends; then
// it stops every one and returns once they have exited. Stopping one
// never holds up the others: a Project's new Watcher waits for its old
// one of the same kind to exit instead.
func Supervise(ctx context.Context, cfg Supervision) {
	notices, stop := cfg.Projects.Events().Subscribe()
	defer stop()
	running := map[watcherKey]*supervised{}
	// stopping holds each Watcher that is still exiting.
	stopping := map[watcherKey]*supervised{}
	defer func() {
		for _, w := range running {
			w.cancel()
		}
		for _, w := range running {
			<-w.done
		}
		for _, w := range stopping {
			<-w.done
		}
	}()
	for {
		for key, w := range stopping {
			select {
			case <-w.done:
				delete(stopping, key)
			default:
			}
		}
		want := map[watcherKey]bool{}
		for _, kind := range cfg.Kinds {
			for _, project := range cfg.Projects.WatchedProjects(ctx, kind.Kind) {
				key := watcherKey{kind.Kind, project}
				want[key] = true
				if running[key] == nil {
					running[key] = cfg.supervise(ctx, projectWatcher{kind, project}, stopping[key])
				}
			}
		}
		for key, w := range running {
			if !want[key] {
				w.cancel()
				stopping[key] = w
				delete(running, key)
			}
		}
	wait:
		for {
			select {
			case <-ctx.Done():
				return
			case <-cfg.Tick:
				break wait
			case e := <-notices:
				if e.Type == events.ProjectChanged {
					break wait
				}
			}
		}
	}
}

// watcherKey names one Project's Watcher of one kind in a map.
type watcherKey struct {
	kind    core.WatcherKind
	project string
}

// supervised is one Project's Watcher of one kind, restarted until
// cancelled.
type supervised struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// supervise runs o once prev, its last one if it is still exiting, has,
// restarting it with backoff each time it exits without being stopped.
func (cfg Supervision) supervise(ctx context.Context, o projectWatcher, prev *supervised) *supervised {
	ctx, cancel := context.WithCancel(ctx)
	w := &supervised{cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(w.done)
		if prev != nil {
			<-prev.done
		}
		defer cfg.logf("stopped %s", o.name())
		backoff, restarts := FirstBackoff, 0
		for {
			started := cfg.Clock.Now()
			lastLine := cfg.run(ctx, o)
			if ctx.Err() != nil {
				return
			}
			if cfg.Clock.Now().Sub(started) >= ResetAfter {
				backoff, restarts = FirstBackoff, 0
			}
			restarts++
			cfg.logf("%s crashed; restarting it in %v", o.name(), backoff)
			cfg.Projects.ReportWatcherCrash(ctx, o.kind.Kind, o.project, core.WatcherCrash{Restarts: restarts, LastLine: lastLine,
				RestartAt: cfg.Clock.Now().Add(backoff)})
			select {
			case <-ctx.Done():
				return
			case <-cfg.Clock.After(backoff):
			}
			backoff = min(2*backoff, MaxBackoff)
		}
	}()
	return w
}

// run runs o once, until it exits or ctx ends, and returns the last line
// it wrote to stderr. Ending ctx closes its stdin, which it exits on; one
// that doesn't in stopLimit is killed.
func (cfg Supervision) run(ctx context.Context, o projectWatcher) string {
	cmd := exec.Command(cfg.Program, o.kind.Args(o.project)...)
	cmd.Env = cfg.Environ
	last := &lastLine{w: cfg.Log}
	cmd.Stdout, cmd.Stderr = cfg.Log, last
	cmd.WaitDelay = stopLimit
	stdin, err := cmd.StdinPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		cfg.logf("cannot start %s: %v", o.name(), err)
		return err.Error()
	}
	cfg.logf("started %s (pid %d)", o.name(), cmd.Process.Pid)
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	select {
	case err := <-exited:
		if err != nil {
			cfg.logf("%s exited: %v", o.name(), err)
		}
	case <-ctx.Done():
		stdin.Close()
		// Real time, not cfg.Clock: a kill is a last resort that must
		// not wait on a test's fake clock.
		select {
		case <-exited:
		case <-time.After(stopLimit):
			cmd.Process.Kill()
			<-exited
		}
	}
	return last.String()
}

// logf writes one line to the service's log.
func (cfg Supervision) logf(format string, args ...any) {
	if cfg.Log != nil {
		fmt.Fprintf(cfg.Log, "code-central serve: "+format+"\n", args...)
	}
}

// lastLine passes what a Watcher writes to stderr on to w, keeping
// the last line with text on it.
type lastLine struct {
	w       io.Writer
	mu      sync.Mutex
	partial string
	last    string
}

// Write keeps p's last line and passes p on.
func (l *lastLine) Write(p []byte) (int, error) {
	l.mu.Lock()
	lines := strings.Split(l.partial+string(p), "\n")
	l.partial = lines[len(lines)-1]
	for _, line := range lines[:len(lines)-1] {
		if line = strings.TrimSpace(line); line != "" {
			l.last = line
		}
	}
	l.mu.Unlock()
	if l.w == nil {
		return len(p), nil
	}
	return l.w.Write(p)
}

// String is the last line written, a partial one included.
func (l *lastLine) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if p := strings.TrimSpace(l.partial); p != "" {
		return p
	}
	return l.last
}
