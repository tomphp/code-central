// The service's supervision of its Watchers, as a test runs it.

package ciwatchertest

import (
	"context"
	"strings"
	"time"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/watchers"
)

// Supervising is the service's supervision of srv's Watchers of some
// kinds, each run as a program the harness installed, with the fake gh
// on PATH, until the test ends or Stop.
type Supervising struct {
	h      *Harness
	srv    *codecentraltest.Server
	cancel context.CancelFunc
	// Log is the service's log.
	Log *codecentraltest.SyncBuffer
	// Done closes once every Watcher has stopped.
	Done chan struct{}
}

// Supervise starts supervising srv's Watchers of kinds, with program,
// such as CodeCentral or Crashing, as the service's executable.
func (h *Harness) Supervise(srv *codecentraltest.Server, program string, kinds ...watchers.Kind) *Supervising {
	h.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	s := &Supervising{h: h, srv: srv, cancel: cancel, Log: &codecentraltest.SyncBuffer{}, Done: make(chan struct{})}
	go func() {
		defer close(s.Done)
		watchers.Supervise(ctx, watchers.Supervision{
			Projects: srv.Core, Kinds: kinds, Program: program, Clock: srv.Clock, Log: s.Log,
			Environ: append(h.Environ(h.Bin), "CODE_CENTRAL_URL="+srv.URL),
		})
	}()
	h.t.Cleanup(s.Stop)
	return s
}

// stopTimeout bounds Stop's wait for every Watcher to exit.
const stopTimeout = time.Minute

// Stop stops the supervision and returns once every Watcher has stopped.
// It keeps moving srv's clock past the stop limit meanwhile, so a Watcher
// that doesn't exit when its stdin closes is killed.
func (s *Supervising) Stop() {
	s.cancel()
	timeout := time.After(stopTimeout)
	for {
		select {
		case <-s.Done:
			return
		case <-timeout:
			s.h.t.Fatalf("the supervision didn't stop; the service's log:\n%s", s.Log)
		case <-time.After(10 * time.Millisecond):
			s.srv.Clock.Advance(watchers.StopLimit)
		}
	}
}

// Logged waits for the service's log to say line n times.
func (s *Supervising) Logged(line string, n int) {
	s.h.t.Helper()
	codecentraltest.Eventually(s.h.t, func() bool { return strings.Count(s.Log.String(), line) >= n },
		"the service's log said %q fewer than %d times:\n%s", line, n, s.Log)
}
