package watchers_test

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/watchers"
)

// leasing is Code Central as the supervision reads it, watching acme and
// beta with every kind, with each Project's Watcher Lease held or not as
// the test sets it.
type leasing struct {
	bus *events.Bus
	mu  sync.Mutex
	// held is whether this service holds each Project's Lease, and err
	// what renewing one fails with; holds counts the renewals.
	held  map[string]bool
	err   error
	holds map[string]int
}

func (l *leasing) WatchedProjects(context.Context, core.WatcherKind) []string {
	return []string{"acme", "beta"}
}

func (l *leasing) ReportWatcherCrash(context.Context, core.WatcherKind, string, core.WatcherCrash) {}

func (l *leasing) Events() *events.Bus { return l.bus }

func (l *leasing) HoldWatcherLease(_ context.Context, key string) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.holds[key]++
	return l.held[key], l.err
}

func (l *leasing) set(held map[string]bool, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.held, l.err = held, err
}

func (l *leasing) renewals(key string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.holds[key]
}

// syncLog is the service's log, written by the supervision's goroutines.
type syncLog struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncLog) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncLog) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func eventually(t *testing.T, ok func() bool, format string, args ...any) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); !ok(); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf(format, args...)
		}
	}
}

// The supervision runs a Project's Watchers only while this service holds
// its Watcher Lease, renewing each Lease once a tick whatever the kinds,
// and stops them once another service has it.
func TestTheSupervisionRunsOnlyTheWatchersOfProjectsWhoseLeaseItHolds(t *testing.T) {
	cat, err := exec.LookPath("cat") // runs until its stdin closes, as a Watcher does
	if err != nil {
		t.Skip("no cat")
	}
	projects := &leasing{bus: events.New(), held: map[string]bool{"acme": true}, holds: map[string]int{}}
	leaseTick := make(chan time.Time)
	log := &syncLog{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	kind := func(k core.WatcherKind) watchers.Kind {
		return watchers.Kind{Kind: k, Args: func(string) []string { return nil }}
	}
	go func() {
		defer close(done)
		watchers.Supervise(ctx, watchers.Supervision{Projects: projects, Kinds: []watchers.Kind{kind(core.WatcherCI), kind(core.WatcherIssues)},
			Program: cat, Clock: clock.NewFake(time.Now()), LeaseTick: leaseTick, Log: log})
	}()
	t.Cleanup(func() { cancel(); <-done })
	logged := func(line string) {
		t.Helper()
		eventually(t, func() bool { return strings.Contains(log.String(), line) }, "the log never said %q:\n%s", line, log)
	}

	logged("started the CI Watcher for acme")
	logged("started the Issue Watcher for acme")
	if got := projects.renewals("acme"); got != 1 {
		t.Fatalf("acme's Lease renewed %d times for two kinds of Watcher, want once", got)
	}
	if strings.Contains(log.String(), "for beta") {
		t.Fatalf("ran beta's Watchers without its Lease:\n%s", log)
	}

	// Another service has taken acme's Lease, and beta's lapsed to this one.
	projects.set(map[string]bool{"beta": true}, nil)
	leaseTick <- time.Now()
	logged("stopped the CI Watcher for acme")
	logged("stopped the Issue Watcher for acme")
	logged("started the CI Watcher for beta")
	if got := projects.renewals("acme"); got != 2 {
		t.Fatalf("acme's Lease renewed %d times over two ticks, want twice", got)
	}

	// The Store can't be reached: what it says goes to the log.
	projects.set(map[string]bool{"beta": true}, errors.New("connection refused"))
	leaseTick <- time.Now()
	logged("cannot renew beta's Watcher Lease: connection refused")
}
