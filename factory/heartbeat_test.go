package factory_test

import (
	"context"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// factories is the Project's Factory Status, read as the human.
func factories(t *testing.T, h *factorytest.Harness) core.FactoryStatusView {
	t.Helper()
	p, err := h.Human().GetProject(context.Background(), h.Project)
	if err != nil {
		t.Fatal(err)
	}
	return p.FactoryStatus
}

// wantFactory fails the test unless the Factory shows status with a note
// containing note, and returns its Factory Status. A test settles the
// Factory first: a Heartbeat it has queued is busy work until posted.
func wantFactory(t *testing.T, h *factorytest.Harness, status core.FactoryStatusName, note string) core.FactoryStatusView {
	t.Helper()
	fs := factories(t, h)
	if fs.Status != status || len(fs.Factories) != 1 || !strings.Contains(fs.Factories[0].Note, note) {
		t.Fatalf("Factory Status %+v, want %s with %q in its note", fs, status, note)
	}
	return fs
}

// TEMPORARY: waitForFactory is wantFactory for chatstop_test.go, which
// isn't migrated to Settle yet; it goes with its last use.
func waitForFactory(t *testing.T, h *factorytest.Harness, status core.FactoryStatusName, note string) core.FactoryStatusView {
	t.Helper()
	h.WaitFor("the Factory to be "+string(status)+" with "+note, func() bool {
		fs := factories(t, h)
		return fs.Status == status && len(fs.Factories) == 1 && strings.Contains(fs.Factories[0].Note, note)
	})
	return factories(t, h)
}

func TestEachPollSendsAnOnlineHeartbeatWithTheTurnsRunning(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Long job")
	h.Script(id, factorytest.Wait(), factorytest.Finish("Done."), factorytest.Result("success"))
	r := h.Start("--parallel", "2", "--poll", "20s")
	fs := wantFactory(t, h, core.FactoryOnline, "1 of 2 turns running")
	f := fs.Factories[0]
	host, _ := os.Hostname()
	if f.Name != "code-central-factory" || f.Host != host || f.IntervalSeconds != 20 {
		t.Errorf("Factory %+v, want code-central-factory on %q with a 20s interval", f, host)
	}
	if !f.Chat {
		t.Errorf("Factory %+v, want Chat support on by default", f)
	}
	h.Continue(id)
	r.Settle()
	h.WantState(id, store.InReview)
	r.Poll()
	wantFactory(t, h, core.FactoryOnline, "0 of 2 turns running")
}

func TestNoChatReportsNoChatSupport(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.Start("--no-chat")
	fs := wantFactory(t, h, core.FactoryOnline, "turns running")
	if fs.Factories[0].Chat {
		t.Errorf("Factory %+v, want no Chat support with --no-chat", fs.Factories[0])
	}
}

func TestAnOutagePauseStaysOnlineAndSaysWhenItResumes(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	cut := h.ReadyTicket("Cut off")
	h.Script(cut, factorytest.APIError("rate_limit"), factorytest.Result("success"), factorytest.Exit(1))
	r := h.Start()
	r.WantOutput("waiting on rate_limit")
	r.Poll()
	fs := wantFactory(t, h, core.FactoryOnline, "paused: ")
	if n := fs.Factories[0].Note; !strings.Contains(n, "rate_limit") || !strings.Contains(n, "; next resume at ") {
		t.Errorf("note %q, want the reason and when it resumes", n)
	}
}

func TestTheFirstCtrlCSendsDrainingAndTheDrainKeepsSendingIt(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Nearly done")
	h.Script(id, factorytest.Wait(), factorytest.Finish("Done."), factorytest.Result("success"))
	r := h.Start("--parallel", "1")
	h.WantCalls(1)

	r.Interrupt()
	r.Settle()
	first := wantFactory(t, h, core.FactoryDraining, "draining: waiting on 1 running turn(s)").Factories[0].LastHeartbeat
	h.Server.Clock.Advance(time.Second)
	r.Tick()
	r.Settle()
	if last := factories(t, h).Factories[0].LastHeartbeat; !last.After(first) {
		t.Errorf("the last Heartbeat is at %v, want another after %v while it drains", last, first)
	}

	h.Continue(id)
	r.Exited()
	wantFactory(t, h, core.FactoryOffline, "stopped: drained")
}

func TestASecondCtrlCSendsAStoppingHeartbeatNamingTheSignal(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Long job")
	h.Script(id, factorytest.Wait())
	r := h.Start()
	h.WantCalls(1)
	r.Interrupt()
	r.Settle()
	wantFactory(t, h, core.FactoryDraining, "draining")
	r.Interrupt()
	r.PassKillGrace()
	r.Exited()
	wantFactory(t, h, core.FactoryOffline, "stopped on interrupt")
}

func TestSIGTERMSendsAStoppingHeartbeatNamingTheSignal(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Long job")
	h.Script(id, factorytest.Wait())
	r := h.Start()
	h.WantCalls(1)
	r.Signal(syscall.SIGTERM)
	r.PassKillGrace()
	r.Exited()
	wantFactory(t, h, core.FactoryOffline, "stopped on terminated")
}

func TestAStopForSendsAStoppingHeartbeatWithItsReason(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	expired := h.ReadyTicket("Expired")
	h.Script(expired, factorytest.APIError("authentication_failed"), factorytest.Result("success"), factorytest.Exit(1))
	r := h.Start()
	r.Exited()
	wantFactory(t, h, core.FactoryOffline, "stopped: claude's login has expired")
}

func TestAFactoryThatRefusesToStartSendsNoHeartbeat(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	refuses(t, h, h.Repo, "isn't an effort level", "--effort", "extreme")
	if fs := factories(t, h); len(fs.Factories) != 0 {
		t.Errorf("a refused Factory sent %+v", fs)
	}
}

func TestARefusedHeartbeatIsWarnedOnceAndNeverStopsThePoll(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.SetFailing("/report_heartbeat")
	r := h.Start()
	r.Poll()
	r.Poll()
	id := h.ReadyTicket("Still claimed")
	h.Script(id, factorytest.Finish("Done."), factorytest.Result("success"))
	r.Poll()
	h.WantState(id, store.InReview)
	r.WantOutput("warning: can't send a Heartbeat")
	if n := strings.Count(r.Stdout(), "warning: can't send a Heartbeat"); n != 1 {
		t.Errorf("warned %d times, want once: %s", n, r.Stdout())
	}

	h.SetFailing("")
	r.Poll()
	wantFactory(t, h, core.FactoryOnline, "turns running")
}
