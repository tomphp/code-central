package factory

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/tomoram/code-central/core"
)

// The Factory's Heartbeats (Spec code-central-305): one on each poll, one
// on the first Ctrl-C and each drain wait, and a last stopping one on exit.
// They are posted off the poll, newest first, so a refused or hung Code
// Central never stops, slows or fails a poll.

const factoryName = "code-central-factory"

// heartbeater posts the newest waiting Heartbeat. Only the newest waits:
// an older one a slow post kept back is not worth sending.
type heartbeater struct {
	post func(ctx context.Context, in core.ReportHeartbeatInput) error
	base core.ReportHeartbeatInput // factory, host and interval
	warn func(err error)

	mu      sync.Mutex
	waiting *core.ReportHeartbeatInput
	last    core.HeartbeatState // the furthest state sent: online, draining, stopping
	failing bool
	wake    chan struct{}
	stop    chan struct{}
	done    chan struct{}
}

func newHeartbeater(post func(context.Context, core.ReportHeartbeatInput) error, base core.ReportHeartbeatInput, warn func(error)) *heartbeater {
	return &heartbeater{post: post, base: base, warn: warn, wake: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{})}
}

var heartbeatRank = map[core.HeartbeatState]int{core.HeartbeatOnline: 0, core.HeartbeatDraining: 1, core.HeartbeatStopping: 2}

// send queues a Heartbeat. A state earlier than one already queued or
// sent (online after draining) is dropped: the Factory doesn't go back.
func (h *heartbeater) send(state core.HeartbeatState, note string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if heartbeatRank[state] < heartbeatRank[h.last] {
		return
	}
	h.last = state
	in := h.base
	in.State, in.Note = state, note
	h.waiting = &in
	select {
	case h.wake <- struct{}{}:
	default:
	}
}

func (h *heartbeater) take() *core.ReportHeartbeatInput {
	h.mu.Lock()
	defer h.mu.Unlock()
	in := h.waiting
	h.waiting = nil
	return in
}

// postOne posts in, logging a failure once until a post goes through.
func (h *heartbeater) postOne(ctx context.Context, in *core.ReportHeartbeatInput) {
	if in.State == core.HeartbeatStopping {
		ctx = context.Background() // close cancels the others, never this one
	}
	ctx, cancel := context.WithTimeout(ctx, postTimeout)
	defer cancel()
	err := h.post(ctx, *in)
	h.mu.Lock()
	first := err != nil && !h.failing
	h.failing = err != nil
	h.mu.Unlock()
	if first {
		h.warn(err)
	}
}

// run posts waiting Heartbeats until close, then the last one.
func (h *heartbeater) run() {
	defer close(h.done)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-h.stop
		cancel()
	}()
	for {
		select {
		case <-h.wake:
			if in := h.take(); in != nil {
				h.postOne(ctx, in)
			}
		case <-h.stop:
			// The stopping Heartbeat has one postTimeout of its own,
			// beside the final flushes.
			if in := h.take(); in != nil && in.State == core.HeartbeatStopping {
				h.postOne(context.Background(), in)
			}
			return
		}
	}
}

// close sends the stopping Heartbeat with note and stops.
func (h *heartbeater) close(note string) {
	h.send(core.HeartbeatStopping, note)
	close(h.stop)
	<-h.done
}

// heartbeat queues a Heartbeat for the Factory's Project.
func (f *factory) heartbeat(state core.HeartbeatState, note string) {
	f.heartbeats.send(state, note)
}

// pollNote is the note of a poll's Heartbeat.
func (f *factory) pollNote() string {
	if reason, until, ok := f.pausedFor(); ok {
		return fmt.Sprintf("paused: %s; next resume at %s", reason, until.Format("15:04"))
	}
	return fmt.Sprintf("%d of %d turns running", f.running(), f.o.parallel)
}

// drainNote is the note of a draining Heartbeat.
func (f *factory) drainNote() string {
	return fmt.Sprintf("draining: waiting on %d running turn(s)", f.running())
}

// stopNote says why the Factory stopped, for its last Heartbeat.
func (f *factory) stopNote() string {
	f.stopMu.Lock()
	defer f.stopMu.Unlock()
	switch {
	case f.stopReason != "":
		return "stopped: " + f.stopReason
	case f.stopSignal != nil:
		return fmt.Sprintf("stopped on %v", f.stopSignal)
	case f.drained:
		return "stopped: drained"
	}
	return "stopped"
}

func newFactoryHeartbeater(f *factory) *heartbeater {
	host, _ := os.Hostname()
	if host == "" {
		host = "unknown"
	}
	base := core.ReportHeartbeatInput{Factory: factoryName, Host: host, IntervalSeconds: max(1, int(f.o.poll/time.Second))}
	return newHeartbeater(func(ctx context.Context, in core.ReportHeartbeatInput) error {
		_, err := f.client.ReportHeartbeat(ctx, f.project, in)
		return err
	}, base, func(err error) {
		f.printf("warning: can't send a Heartbeat to Code Central: %v", err)
	})
}
