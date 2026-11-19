// The Factory's wake (Spec code-central-743): a goroutine apart from the
// poll loop keeps one next_factory_wake call open while the Factory runs,
// and each call that returns woken has the poll loop poll at once instead
// of on its next --poll tick. --poll stays the fallback and the Heartbeat
// cadence.

package factory

import (
	"context"
	"errors"

	"github.com/tomoram/code-central/api"
)

// wakeRetry is how long the wake loop waits after a next_factory_wake
// that failed before it calls again: the Chat loop's pause.
const wakeRetry = chatRetry

// wakeLoop keeps one next_factory_wake call open until ctx ends or the
// Factory drains, and wakes the poll loop each time a call returns woken.
// Against a Code Central without the operation it stops at once, leaving
// the timed poll. Its pauses are on the Factory's clock, as every
// other timer is. s is its line of work, busy starting it.
func (f *factory) wakeLoop(ctx context.Context, s *stint) {
	defer s.stop()
	pollCtx, cancel := f.untilDraining(ctx)
	defer cancel()
	failing := false
	for pollCtx.Err() == nil {
		s.rest(WakeLongPoll)
		w, err := f.client.NextFactoryWake(pollCtx, f.project, api.NextFactoryWakeInput{Factory: factoryName, Host: f.host})
		if err != nil {
			if pollCtx.Err() != nil {
				return
			}
			if ce := (*api.Error)(nil); errors.As(err, &ce) && ce.Code == api.NotFound {
				f.printf("warning: Code Central has no next_factory_wake: polling every %s only", f.o.poll)
				return
			}
			if !failing {
				f.printf("warning: can't wait for a wake from Code Central: %v", err)
			}
			failing = true
			retry := s.after(f.e.Clock, wakeRetry, "wake retry pause")
			select {
			case <-pollCtx.Done():
			case <-retry:
			}
			continue
		}
		failing = false
		if w.Woken {
			f.wake()
		}
	}
}

// wake has the poll loop poll once more, at once if it is waiting, or
// else as soon as its poll ends: any number of wakes during a poll make
// one more poll. The wake is busy work until that poll has started. Once
// the poll loop polls no more, draining or stopped, it is ignored.
func (f *factory) wake() {
	f.wakeMu.Lock()
	defer f.wakeMu.Unlock()
	if f.pollsOver {
		return
	}
	end := f.work.busy("a wake")
	select {
	case f.wakeCh <- end:
	default: // a poll is due already
		end()
	}
}

// endPolls has the poll loop take no more wakes, as it drains or has
// stopped, dropping one waiting.
func (f *factory) endPolls() {
	f.wakeMu.Lock()
	defer f.wakeMu.Unlock()
	f.pollsOver = true
	select {
	case end := <-f.wakeCh:
		end()
	default:
	}
}
