// The service's supervision of its Watchers, as a test runs it.

package ciwatchertest

import (
	"context"
	"strings"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/watchers"
)

// Supervising is the service's supervision of srv's Watchers of some
// kinds, each run as a program the harness installed, with the fake gh
// on PATH, until the test ends or Stop.
type Supervising struct {
	h *Harness
	// Log is the service's log.
	Log *codecentraltest.SyncBuffer
	// Stop stops the supervision, and Done closes once every Watcher has
	// stopped.
	Stop func()
	Done chan struct{}
}

// Supervise starts supervising srv's Watchers of kinds, with program,
// such as CodeCentral or Crashing, as the service's executable.
func (h *Harness) Supervise(srv *codecentraltest.Server, program string, kinds ...watchers.Kind) *Supervising {
	h.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	s := &Supervising{h: h, Log: &codecentraltest.SyncBuffer{}, Stop: cancel, Done: make(chan struct{})}
	go func() {
		defer close(s.Done)
		watchers.Supervise(ctx, watchers.Supervision{
			Projects: srv.Core, Kinds: kinds, Program: program, Clock: srv.Clock, Log: s.Log,
			Environ: append(h.Environ(h.Bin), "CODE_CENTRAL_URL="+srv.URL),
		})
	}()
	h.t.Cleanup(func() {
		cancel()
		<-s.Done
	})
	return s
}

// Logged waits for the service's log to say line n times.
func (s *Supervising) Logged(line string, n int) {
	s.h.t.Helper()
	codecentraltest.Eventually(s.h.t, func() bool { return strings.Count(s.Log.String(), line) >= n },
		"the service's log said %q fewer than %d times:\n%s", line, n, s.Log)
}
