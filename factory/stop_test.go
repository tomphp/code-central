package factory_test

import (
	"os"
	"runtime"
	"slices"
	"syscall"
	"testing"

	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

func TestTheFirstCtrlCDrainsWaitingForRunningTurnsAndClaimingNoMore(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Nearly done")
	h.Script(id, factorytest.Wait(), factorytest.Finish("Done."), factorytest.Result("success"))
	later := h.ReadyTicket("Later")
	r := h.Start("--parallel", "1")
	h.WaitFor("the turn", func() bool { return len(h.Calls()) == 1 })

	r.Interrupt()
	r.WaitForOutput("waiting on 1 running turn(s): " + id)

	h.Continue(id)
	if code := r.Exited(); code != 0 {
		t.Errorf("exit code %d, want 0", code)
	}
	h.WaitForState(id, store.InReview)
	if got := h.Signals(id); len(got) != 0 {
		t.Errorf("the draining turn got %q, want no signals", got)
	}
	if s := h.Ticket(later).State; s != store.Ready || len(h.Calls()) != 1 {
		t.Errorf("%s is %s after %d runs; a draining Factory claims nothing", later, s, len(h.Calls()))
	}
}

// stopSignals is what the fake claude notes when the Factory stops a
// turn. On Unix it gets SIGINT, which it ignores, then SIGTERM, which ends
// it. On Windows it gets Ctrl-Break, which it ignores, so the Factory
// kills it.
func stopSignals() []string {
	if runtime.GOOS == "windows" {
		return []string{"BREAK"}
	}
	return []string{"INT", "TERM"}
}

// stoppedExit is the exit code of a turn the Factory stopped: 143 from
// SIGTERM on Unix, 1 from the kill on Windows.
func stoppedExit() int {
	if runtime.GOOS == "windows" {
		return 1
	}
	return 143
}

// wantStopped checks the turn for id got stopSignals.
func wantStopped(t *testing.T, h *factorytest.Harness, id string) {
	t.Helper()
	if got, want := h.Signals(id), stopSignals(); !slices.Equal(got, want) {
		t.Errorf("the turn got %q, want %q", got, want)
	}
}

// killed checks a turn the Factory stopped on its way out: it got
// stopSignals, and its Claim is kept, In Progress and in the state
// file, with no Failure Report.
func killed(t *testing.T, h *factorytest.Harness, r *factorytest.Running, id string) {
	t.Helper()
	if code := r.Exited(); code != 0 {
		t.Errorf("exit code %d, want 0", code)
	}
	wantStopped(t, h, id)
	tk := h.Ticket(id)
	if tk.State != store.InProgress || len(tk.Claims) != 1 || tk.Claims[0].Ended != nil || len(tk.FailureReports) != 0 {
		t.Errorf("%s is %s with Claims %+v and %d Failure Reports; want its one Claim kept, In Progress, never failed", id, tk.State, tk.Claims, len(tk.FailureReports))
	}
	if got := stateClaims(t, h); !slices.Equal(got, []string{h.Calls()[0].Claim}) {
		t.Errorf("state file holds %q, want the kept Claim", got)
	}
}

func TestASecondCtrlCStopsTheRunningTurnsAndKeepsTheirClaims(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Long job")
	h.Script(id, factorytest.Wait())
	r := h.Start()
	h.WaitFor("the turn", func() bool { return len(h.Calls()) == 1 })

	r.Interrupt()
	r.WaitForOutput("waiting on 1 running turn(s)")
	r.Interrupt()
	killed(t, h, r, id)
}

func TestSIGTERMOrSIGHUPStopsTheRunningTurnsAtOnce(t *testing.T) {
	t.Parallel()
	for _, sig := range []os.Signal{syscall.SIGTERM, syscall.SIGHUP} {
		t.Run(sig.String(), func(t *testing.T) {
			t.Parallel()
			h := factorytest.New(t)
			id := h.ReadyTicket("Long job")
			h.Script(id, factorytest.Wait())
			r := h.Start()
			h.WaitFor("the turn", func() bool { return len(h.Calls()) == 1 })

			r.Signal(sig)
			killed(t, h, r, id)
		})
	}
}
