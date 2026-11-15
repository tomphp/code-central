package factory

import (
	"io"
	"testing"
)

// Any number of wakes during a poll make exactly one more poll: the wake
// waits in a one-slot channel, and the rest find it full.
func TestWakesDuringAPollMakeOneMorePoll(t *testing.T) {
	f := newFactory(Env{Stdout: io.Discard}, options{})
	for range 3 {
		f.wake()
	}
	polls := 0
	for {
		select {
		case <-f.wakeCh:
			polls++
			continue
		default:
		}
		break
	}
	if polls != 1 {
		t.Errorf("three wakes during a poll made %d more polls, want one", polls)
	}
}
