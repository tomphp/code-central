// The Factory's wake (Spec code-central-743): a goroutine apart from the
// poll loop keeps one next_factory_wake call open while the Factory runs,
// and each call that returns woken has the poll loop poll at once instead
// of on its next --poll tick. --poll stays the fallback and the Heartbeat
// cadence.

package factory

import (
	"context"
	"errors"
	"time"

	"github.com/tomoram/code-central/core"
)

// wakeRetry is how long the wake loop waits after a next_factory_wake
// that failed before it calls again: the Chat loop's pause.
const wakeRetry = chatRetry

// wakeLoop keeps one next_factory_wake call open until ctx ends or the
// Factory drains, and wakes the poll loop each time a call returns woken.
// Against a Code Central without the operation it stops at once, leaving
// the timed poll. Its pauses run on real time, never on the injected
// After, so they never take a poll's tick.
func (f *factory) wakeLoop(ctx context.Context) {
	pollCtx, cancel := f.untilDraining(ctx)
	defer cancel()
	failing := false
	for pollCtx.Err() == nil {
		w, err := f.client.NextFactoryWake(pollCtx, f.project, core.NextFactoryWakeInput{Factory: factoryName, Host: f.host})
		if err != nil {
			if pollCtx.Err() != nil {
				return
			}
			if ce := (*core.Error)(nil); errors.As(err, &ce) && ce.Code == core.NotFound {
				f.printf("warning: Code Central has no next_factory_wake: polling every %s only", f.o.poll)
				return
			}
			if !failing {
				f.printf("warning: can't wait for a wake from Code Central: %v", err)
			}
			failing = true
			select {
			case <-pollCtx.Done():
			case <-time.After(wakeRetry):
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
// one more poll.
func (f *factory) wake() {
	select {
	case f.wakeCh <- struct{}{}:
	default: // a poll is due already
	}
}

// untilDraining is ctx, ended also by the first Ctrl-C: a long-poll's
// context, as draining closes the long-polls.
func (f *factory) untilDraining(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-f.drainCh:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}
