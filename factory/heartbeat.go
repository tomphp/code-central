package factory

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/tomoram/code-central/api"
)

// The Factory's Heartbeats (Spec code-central-305): one on each poll, one
// on the first Ctrl-C and each drain wait, and a last stopping one on exit.
// They are posted off the poll, newest first, so a refused or hung Code
// Central never stops, slows or fails a poll.

const factoryName = "code-central-factory"

// heartbeater posts the newest waiting Heartbeat. Only the newest waits:
// an older one a slow post kept back is not worth sending.
type heartbeater struct {
	post func(ctx context.Context, in api.ReportHeartbeatInput) error
	base api.ReportHeartbeatInput // factory, host and interval
	// limits are the Factory's latest Plan Limits as a Heartbeat is
	// queued, nil before its first Turn reads any.
	limits func() *api.PlanLimits
	warn   func(err error)
	work   work

	mu      sync.Mutex
	waiting *api.ReportHeartbeatInput
	posted  func()             // ends the waiting Heartbeat's busy work once it is posted
	last    api.HeartbeatState // the furthest state sent: online, draining, stopping
	failing bool
	wake    chan struct{}
	stop    chan struct{}
	done    chan struct{}
}

func newHeartbeater(post func(context.Context, api.ReportHeartbeatInput) error, base api.ReportHeartbeatInput, limits func() *api.PlanLimits, warn func(error), w work) *heartbeater {
	return &heartbeater{post: post, base: base, limits: limits, warn: warn, work: w, wake: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{})}
}

var heartbeatRank = map[api.HeartbeatState]int{api.HeartbeatOnline: 0, api.HeartbeatDraining: 1, api.HeartbeatStopping: 2}

// send queues a Heartbeat. A state earlier than one already queued or
// sent (online after draining) is dropped: the Factory doesn't go back.
func (h *heartbeater) send(state api.HeartbeatState, note string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if heartbeatRank[state] < heartbeatRank[h.last] {
		return
	}
	h.last = state
	in := h.base
	in.State, in.Note, in.PlanLimits = state, note, h.limits()
	h.waiting = &in
	if h.posted == nil {
		h.posted = h.work.busy("posting a Heartbeat")
	}
	select {
	case h.wake <- struct{}{}:
	default:
	}
}

// take takes the waiting Heartbeat, if any, and the func that ends its
// busy work once it is posted.
func (h *heartbeater) take() (*api.ReportHeartbeatInput, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	in, posted := h.waiting, h.posted
	h.waiting, h.posted = nil, nil
	if posted == nil {
		posted = noop
	}
	return in, posted
}

// postOne posts in, logging a failure once until a post goes through.
func (h *heartbeater) postOne(ctx context.Context, in *api.ReportHeartbeatInput) {
	if in.State == api.HeartbeatStopping {
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
			in, posted := h.take()
			if in != nil {
				h.postOne(ctx, in)
			}
			posted()
		case <-h.stop:
			// The stopping Heartbeat has one postTimeout of its own,
			// beside the final flushes.
			in, posted := h.take()
			if in != nil && in.State == api.HeartbeatStopping {
				h.postOne(context.Background(), in)
			}
			posted()
			return
		}
	}
}

// close sends the stopping Heartbeat with note and stops.
func (h *heartbeater) close(note string) {
	h.send(api.HeartbeatStopping, note)
	close(h.stop)
	<-h.done
}

// heartbeat queues a Heartbeat for the Factory's Project.
func (f *factory) heartbeat(state api.HeartbeatState, note string) {
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

// hostname is the machine the Factory runs on, as its Heartbeats and its
// next_chat_work calls name it.
func hostname() string {
	host, _ := os.Hostname()
	if host == "" {
		host = "unknown"
	}
	return host
}

func newFactoryHeartbeater(f *factory) *heartbeater {
	base := api.ReportHeartbeatInput{Factory: factoryName, Host: f.host, IntervalSeconds: max(1, int(f.o.poll/time.Second)), Chat: !f.o.noChat}
	return newHeartbeater(func(ctx context.Context, in api.ReportHeartbeatInput) error {
		_, err := f.client.ReportHeartbeat(ctx, f.project, in)
		return err
	}, base, f.planLimits.get, func(err error) {
		f.printf("warning: can't send a Heartbeat to Code Central: %v", err)
	}, f.work)
}
