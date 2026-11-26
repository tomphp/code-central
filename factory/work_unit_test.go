package factory

import (
	"testing"
	"time"

	"github.com/tomoram/code-central/clock"
)

// movingClock moves the fake clock right after each Now, as a test
// rig might between a caller's reading the time and making its timer.
type movingClock struct {
	*clock.Fake
	by time.Duration
}

func (c movingClock) Now() time.Time {
	now := c.Fake.Now()
	c.Fake.Advance(c.by)
	return now
}

type restHook struct{ until []time.Time }

func (h *restHook) Busy(string) func() { return func() {} }
func (h *restHook) Rest(_ string, until time.Time) func() {
	h.until = append(h.until, until)
	return func() {}
}
func (h *restHook) PollWait(until time.Time) func() {
	h.until = append(h.until, until)
	return func() {}
}

// A wait the clock moves under while it starts is told as due no earlier
// than its timer: told earlier, a rig would call it due while the
// timer hadn't fired, and wait for it without moving the clock.
func TestAWaitIsNeverToldDueBeforeItsTimer(t *testing.T) {
	for name, wait := range map[string]func(work, clock.Clock){
		"after":    func(w work, c clock.Clock) { w.after(c, 30*time.Second, "kill grace") },
		"pollWait": func(w work, c clock.Clock) { w.pollWait(c, 30*time.Second) },
	} {
		t.Run(name, func(t *testing.T) {
			fake := clock.NewFake(time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC))
			h := &restHook{}
			wait(work{h}, movingClock{fake, 10 * time.Second})
			timer, ok := fake.Next()
			if !ok || len(h.until) != 1 {
				t.Fatalf("timer %v, %v; told %v", timer, ok, h.until)
			}
			if h.until[0].Before(timer) {
				t.Errorf("told due at %v, before its timer's %v", h.until[0], timer)
			}
		})
	}
}
