// The Factory's work in flight, told to Env.Work (ADR 0045): a test's
// hook hears of each piece of work as it starts and ends, so that the
// test harness, factorytest, knows when the Factory has settled. The
// binary gives no hook, and then the Factory tells nothing and pays for
// nothing.

package factory

import (
	"strings"
	"time"

	"github.com/tomoram/code-central/clock"
)

// Work hears of the Factory's work in flight, so that a test can know
// when the Factory has settled: when nothing it is doing will change
// anything without an outside event. The outside events are the clock
// moving, a long-poll's answer, a Turn's claude printing or exiting, and
// a signal.
//
// Each piece of work is busy, changing something by itself (a poll, a
// post, git work, handling a Turn's exit), or resting, waiting on an
// outside event (a timer, a long-poll, a Turn's claude running, a signal).
// Work that follows other work starts before the other ends, so no moment
// between them looks settled.
//
// A test acts on the outside events it makes, so it tells apart the
// resting work they end by name: "turn <key> running" is a Turn's claude
// running, <key> being its Ticket ID or chat for a Chat turn; "kill grace
// of turn <key>" is the wait after a stop step sent to it; and
// WakeLongPoll, ChatLongPoll and SignalWait are the long-polls and the
// wait for a signal.
type Work interface {
	// Busy is told that busy work called name has started, and returns
	// the func told that it has ended.
	Busy(name string) (end func())
	// Rest is told that resting work called name has started, and
	// returns the func told that it has ended. until, when it isn't zero,
	// is the deadline on the Factory's clock of the timer it waits on.
	Rest(name string, until time.Time) (end func())
	// PollWait is Rest for the poll loop's wait for its next poll, due at
	// deadline.
	PollWait(deadline time.Time) (end func())
}

// The names of the resting work that the outside events a test makes
// end (Work).
const (
	// WakeLongPoll is the open next_factory_wake call.
	WakeLongPoll = "wake long-poll"
	// ChatLongPoll is the open next_chat_work call.
	ChatLongPoll = "Chat long-poll"
	// SignalWait is the wait for a stop signal on Env.Signals.
	SignalWait = "waiting for a signal"
)

// work is Env.Work, safe to use without one: with no hook it tells
// nothing, builds no name and costs nothing.
type work struct{ hook Work }

func noop() {}

// busy tells of busy work named by name's parts, joined; end ends it.
func (w work) busy(name ...string) (end func()) {
	if w.hook == nil {
		return noop
	}
	return w.hook.Busy(strings.Join(name, ""))
}

// rest tells of resting work named by name's parts that waits on no
// timer.
func (w work) rest(name ...string) (end func()) {
	if w.hook == nil {
		return noop
	}
	return w.hook.Rest(strings.Join(name, ""), time.Time{})
}

// after is clk.After(d), telling of the wait as resting work named by
// name's parts until the timer fires; end ends the wait.
func (w work) after(clk clock.Clock, d time.Duration, name ...string) (fired <-chan time.Time, end func()) {
	if w.hook == nil {
		return clk.After(d), noop
	}
	at := clk.Now().Add(d)
	return clk.After(d), w.hook.Rest(strings.Join(name, ""), at)
}

// pollWait is the poll loop's after: its wait for its next poll.
func (w work) pollWait(clk clock.Clock, d time.Duration) (fired <-chan time.Time, end func()) {
	if w.hook == nil {
		return clk.After(d), noop
	}
	at := clk.Now().Add(d)
	return clk.After(d), w.hook.PollWait(at)
}

// stint is one goroutine's line of work, told piece by piece: each piece
// starts before the one it follows ends. The zero stint tells nothing.
type stint struct {
	w   work
	end func()
}

// busy goes on with busy work named by name's parts.
func (s *stint) busy(name ...string) { s.then(s.w.busy(name...)) }

// rest goes on with resting work named by name's parts that waits on no
// timer.
func (s *stint) rest(name ...string) { s.then(s.w.rest(name...)) }

// after goes on resting on clk.After(d), named by name's parts, and
// returns the timer.
func (s *stint) after(clk clock.Clock, d time.Duration, name ...string) <-chan time.Time {
	fired, end := s.w.after(clk, d, name...)
	s.then(end)
	return fired
}

// then goes on with the work end ends, ending the piece before it.
func (s *stint) then(end func()) {
	before := s.end
	s.end = end
	if before != nil {
		before()
	}
}

// stop ends the stint's work.
func (s *stint) stop() { s.then(nil) }

// handoff carries busy work from the goroutine that ends another's wait
// to the goroutine waiting, which ends it once its own work has started:
// so the hand-over leaves no moment that looks settled. It holds one at
// most.
type handoff chan func()

func newHandoff() handoff { return make(handoff, 1) }

// give hands busy work's end over, or ends it at once when a hand-over is
// waiting already: one is enough.
func (h handoff) give(end func()) {
	select {
	case h <- end:
	default:
		end()
	}
}

// take ends the work handed over, if any. A nil handoff holds none.
func (h handoff) take() {
	select {
	case end := <-h:
		end()
	default:
	}
}
