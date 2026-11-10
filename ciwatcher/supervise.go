// The service's side of the watch mode: it runs one CI Watcher per
// Project with CI Tracking on, as its own executable's hidden
// subcommand, and restarts one that crashes.

package ciwatcher

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/events"
)

// The restart backoff: a crashed CI Watcher is restarted after
// FirstBackoff, doubling after each crash to at most MaxBackoff, and
// back to FirstBackoff once one has run for ResetAfter.
const (
	FirstBackoff = time.Second
	MaxBackoff   = 5 * time.Minute
	ResetAfter   = 10 * time.Minute
)

// stopLimit bounds how long a CI Watcher has to exit once its stdin
// closes before it is killed.
const stopLimit = 10 * time.Second

// Projects is what the service's supervision reads and reports on Code
// Central; *core.Core is one.
type Projects interface {
	// CIWatchedProjects is the key of every Project that should have a
	// CI Watcher.
	CIWatchedProjects(ctx context.Context) []string
	// ReportCIWatcherCrash reports that key's CI Watcher crashed and is
	// being restarted for the restarts-th time.
	ReportCIWatcherCrash(ctx context.Context, key string, restarts int, lastLine string)
	// Events carries project.changed, on which the CI Watchers are
	// started and stopped.
	Events() *events.Bus
}

// Supervision wires the service's supervision of its CI Watchers.
type Supervision struct {
	Projects Projects
	// Program is the service's own executable, run with WatchArgs.
	Program string
	// Environ is the CI Watchers' environment: the service's, with
	// CODE_CENTRAL_URL naming the service.
	Environ []string
	Clock   clock.Clock
	// Tick, when set, checks which CI Watchers should run on each tick
	// too, in case a project.changed was missed.
	Tick <-chan time.Time
	// Log receives each start, stop and crash, and what the CI Watchers
	// write.
	Log io.Writer
}

// Supervise runs a CI Watcher for each Project CIWatchedProjects names,
// starting and stopping them as it changes, until ctx ends; then it
// stops every one and returns once they have exited.
func Supervise(ctx context.Context, cfg Supervision) {
	notices, stop := cfg.Projects.Events().Subscribe()
	defer stop()
	running := map[string]*supervised{}
	defer func() {
		for _, w := range running {
			w.cancel()
		}
		for _, w := range running {
			<-w.done
		}
	}()
	for {
		want := map[string]bool{}
		for _, key := range cfg.Projects.CIWatchedProjects(ctx) {
			want[key] = true
			if running[key] == nil {
				running[key] = cfg.supervise(ctx, key)
			}
		}
		for key, w := range running {
			if !want[key] {
				w.cancel()
				<-w.done
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

// supervised is one Project's CI Watcher, restarted until cancelled.
type supervised struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// supervise runs key's CI Watcher, restarting it with backoff each time
// it exits without being stopped.
func (cfg Supervision) supervise(ctx context.Context, key string) *supervised {
	ctx, cancel := context.WithCancel(ctx)
	w := &supervised{cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(w.done)
		defer cfg.logf("stopped the CI Watcher for %s", key)
		backoff, restarts := FirstBackoff, 0
		for {
			started := cfg.Clock.Now()
			lastLine := cfg.run(ctx, key)
			if ctx.Err() != nil {
				return
			}
			if cfg.Clock.Now().Sub(started) >= ResetAfter {
				backoff, restarts = FirstBackoff, 0
			}
			restarts++
			cfg.logf("the CI Watcher for %s crashed; restarting it in %v", key, backoff)
			cfg.Projects.ReportCIWatcherCrash(ctx, key, restarts, lastLine)
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

// run runs key's CI Watcher once, until it exits or ctx ends, and
// returns the last line it wrote to stderr. Ending ctx closes its stdin,
// which it exits on; one that doesn't in stopLimit is killed.
func (cfg Supervision) run(ctx context.Context, key string) string {
	cmd := exec.Command(cfg.Program, WatchArgs(key)...)
	cmd.Env = cfg.Environ
	last := &lastLine{w: cfg.Log}
	cmd.Stdout, cmd.Stderr = cfg.Log, last
	cmd.WaitDelay = stopLimit
	stdin, err := cmd.StdinPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		cfg.logf("cannot start the CI Watcher for %s: %v", key, err)
		return err.Error()
	}
	cfg.logf("started the CI Watcher for %s (pid %d)", key, cmd.Process.Pid)
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	select {
	case err := <-exited:
		if err != nil {
			cfg.logf("the CI Watcher for %s exited: %v", key, err)
		}
	case <-ctx.Done():
		stdin.Close()
		select {
		case <-exited:
		case <-time.After(stopLimit):
			cmd.Process.Kill()
			<-exited
		}
	}
	return last.String()
}

func (cfg Supervision) logf(format string, args ...any) {
	if cfg.Log != nil {
		fmt.Fprintf(cfg.Log, "code-central serve: "+format+"\n", args...)
	}
}

// lastLine passes what a CI Watcher writes to stderr on to w, keeping
// the last line with text on it.
type lastLine struct {
	w       io.Writer
	mu      sync.Mutex
	partial string
	last    string
}

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
