package factorytest

import (
	"runtime"
	"testing"
	"time"
)

// The harness's tests run the fake claude, as the Factory's do.
func TestMain(m *testing.M) { Main(m) }

func TestTickMovesTheClockToTheNextPollsDeadline(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		args []string
		poll time.Duration
	}{
		{"the default --poll", nil, 30 * time.Second},
		{"--poll 20s", []string{"--poll", "20s"}, 20 * time.Second},
		{"--poll=45s", []string{"--poll=45s"}, 45 * time.Second},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			h := New(t)
			r := h.Start(c.args...)
			started := h.Clock.Now()
			r.Poll()
			if got := h.Clock.Now().Sub(started); got != c.poll {
				t.Fatalf("the clock moved %v for one poll, want %v", got, c.poll)
			}
			r.Poll()
			if got := h.Clock.Now().Sub(started); got != 2*c.poll {
				t.Fatalf("the clock moved %v for two polls, want %v", got, 2*c.poll)
			}
		})
	}
}

// A Factory stopping a turn that ignores its first stop step waits out
// the kill grace on its clock, which Stop moves.
func TestStopEndsAFactoryInAKillGrace(t *testing.T) {
	t.Parallel()
	h := New(t)
	id := h.ReadyTicket("Long job")
	h.Script(id, Wait())
	r := h.Start()
	h.WaitFor("the turn", func() bool { return len(h.Calls()) == 1 })
	r.Interrupt()
	r.Interrupt()
	h.WaitFor("the turn's kill grace", r.clock.inKillGrace)

	if code := r.Stop(); code != 0 {
		t.Errorf("exit code %d, want 0", code)
	}
	if got := h.Signals(id); len(got) == 0 {
		t.Error("the turn got no stop signal")
	}
}

func TestPassKillGraceTakesAStoppingTurnToItsNextStopStep(t *testing.T) {
	t.Parallel()
	h := New(t)
	id := h.ReadyTicket("Long job")
	h.Script(id, Wait())
	r := h.Start()
	h.WaitFor("the turn", func() bool { return len(h.Calls()) == 1 })
	r.Interrupt()
	r.Interrupt()

	started := h.Clock.Now()
	r.PassKillGrace()
	if got := h.Clock.Now().Sub(started); got != KillGrace {
		t.Errorf("the clock moved %v, want the kill grace, %v", got, KillGrace)
	}
	if code := r.Exited(); code != 0 {
		t.Errorf("exit code %d, want 0", code)
	}
	if got := h.Signals(id); len(got) < 2 && runtime.GOOS != "windows" {
		t.Errorf("the turn got %q, want a second stop step after the kill grace", got)
	}
}
