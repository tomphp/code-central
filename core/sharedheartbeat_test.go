package core_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/store"
)

// Heartbeats shared across services (Spec code-central-1107): a Factory
// reporting to one service shows, with its Plan Limits and its Base and
// clone reports, on every service sharing the Store, once that service's
// Factory Status watch has ticked.

// twoServices is two services, one on laptop and one on desktop, on s and
// one fake clock, with Project acme.
func twoServices(t *testing.T, s store.Store) (laptop, desktop fixture) {
	t.Helper()
	clk := clock.NewFake(t0)
	laptop = fixture{core: core.New(s, clk, &ids.Sequence{}), clock: clk, store: s}
	desktop = fixture{core: core.New(s, clk, &ids.Sequence{}), clock: clk, store: s}
	laptop.core.SetService(core.Service{ID: "a", Host: "laptop"})
	desktop.core.SetService(core.Service{ID: "b", Host: "desktop"})
	laptop.project(t, "acme")
	return laptop, desktop
}

// onlyFactory is the one Factory f lists for acme.
func onlyFactory(t *testing.T, f fixture) core.FactoryView {
	t.Helper()
	fs := factoryStatus(f, t).Factories
	if len(fs) != 1 {
		t.Fatalf("Factories %+v, want one", fs)
	}
	return fs[0]
}

func TestAFactoryReportingToOneServiceShowsOnAnotherAfterItsTick(t *testing.T) {
	laptop, desktop := twoServices(t, store.NewMemory())
	tick := watchFactories(t, desktop)
	sendBeat(laptop, t, core.ReportHeartbeatInput{Factory: "cc", Host: "box", State: core.HeartbeatDraining, Note: "winding down", IntervalSeconds: 20, Chat: true})

	if got := tick(); !slices.Equal(got, []string{"acme"}) {
		t.Fatalf("the desktop's tick reported %v, want acme", got)
	}
	here, there := onlyFactory(t, laptop), onlyFactory(t, desktop)
	if here.ReportsTo != "" {
		t.Errorf("on the laptop the Factory reports to %q, want this service", here.ReportsTo)
	}
	if there.ReportsTo != "laptop" {
		t.Errorf("on the desktop the Factory reports to %q, want laptop", there.ReportsTo)
	}
	there.ReportsTo = ""
	if here != there || there.Status != core.FactoryDraining || there.Note != "winding down" || there.IntervalSeconds != 20 || !there.Chat || !there.LastHeartbeat.Equal(t0) {
		t.Fatalf("the desktop lists %+v, the laptop %+v: want the same Draining Factory", there, here)
	}
	if s := factoryStatus(desktop, t).Status; s != core.FactoryDraining {
		t.Fatalf("the desktop's combined status is %s, want draining", s)
	}

	// A steady Factory sends no notice on the desktop's tick either.
	laptop.clock.Advance(20 * time.Second)
	sendBeat(laptop, t, core.ReportHeartbeatInput{Factory: "cc", Host: "box", State: core.HeartbeatDraining, Note: "winding down", IntervalSeconds: 20, Chat: true})
	if got := tick(); len(got) != 0 {
		t.Fatalf("a steady Factory's tick reported %v", got)
	}
	// What the human sees changing sends one.
	sendBeat(laptop, t, core.ReportHeartbeatInput{Factory: "cc", Host: "box", State: core.HeartbeatDraining, Note: "nearly done", IntervalSeconds: 20, Chat: true})
	if got := tick(); !slices.Equal(got, []string{"acme"}) {
		t.Fatalf("a new note's tick reported %v, want acme", got)
	}

	laptop.clock.Advance(90 * time.Second)
	if got := tick(); !slices.Equal(got, []string{"acme"}) {
		t.Fatalf("the deadline's tick reported %v, want acme", got)
	}
	for name, f := range map[string]fixture{"laptop": laptop, "desktop": desktop} {
		if fs := factoryStatus(f, t); fs.Status != core.FactoryOffline || fs.Factories[0].Status != core.FactoryOffline {
			t.Errorf("past its deadline, on the %s, %+v, want Offline", name, fs)
		}
	}
}

func TestAStoppingFactoryIsOfflineOnEveryService(t *testing.T) {
	laptop, desktop := twoServices(t, store.NewMemory())
	tick := watchFactories(t, desktop)
	beat(laptop, t, "cc", "box", core.HeartbeatOnline, 10)
	tick()
	beat(laptop, t, "cc", "box", core.HeartbeatStopping, 10)
	if got := tick(); !slices.Equal(got, []string{"acme"}) {
		t.Fatalf("the tick reported %v, want acme", got)
	}
	if s := onlyFactory(t, desktop).Status; s != core.FactoryOffline {
		t.Fatalf("a stopping Factory is %s on the desktop, want offline", s)
	}
}

func TestPlanLimitsAndBaseAndCloneReportsShowOnEveryService(t *testing.T) {
	laptop, desktop := twoServices(t, store.NewMemory())
	tick := watchFactories(t, desktop)
	sent := limitsAt(t0.Add(-time.Minute), 42.5, 7)
	beatLimits(laptop, t, "cc", sent)
	laptop.reportBase(t, core.ReportBaseInput{State: core.BaseDiverged, Local: "aaa111", Remote: "bbb222"})
	laptop.reportRepositories(t, homeReady)
	tick()

	fs := factoryStatus(desktop, t)
	if pl := fs.PlanLimits; pl == nil || !pl.ObservedAt.Equal(sent.ObservedAt) || !sameWindow(pl.FiveHour, sent.FiveHour) || !sameWindow(pl.Weekly, sent.Weekly) {
		t.Errorf("the desktop's Plan Limits are %+v, want %+v", pl, sent)
	}
	if b := fs.Base; b == nil || b.State != core.BaseDiverged || b.Local != "aaa111" || b.Remote != "bbb222" {
		t.Errorf("the desktop's base is %+v, want diverged", b)
	}
	if c := repository(t, desktop.projectView(t, "acme"), "acme").Clone; c == nil || c.State != core.CloneReady || c.Path != "/src/acme" {
		t.Errorf("the desktop's clone is %+v, want ready at /src/acme", c)
	}

	// A Heartbeat with no Plan Limits clears them, and one back from
	// stopping drops the reports, on both.
	beat(laptop, t, "cc", "box", core.HeartbeatStopping, 10)
	laptop.clock.Advance(time.Second)
	beat(laptop, t, "cc", "box", core.HeartbeatOnline, 10)
	tick()
	for name, f := range map[string]fixture{"laptop": laptop, "desktop": desktop} {
		fs := factoryStatus(f, t)
		if fs.Status != core.FactoryOnline || fs.PlanLimits != nil || fs.Base != nil {
			t.Errorf("on the %s, %+v: want Online, with no Plan Limits or base", name, fs)
		}
		if c := repository(t, f.projectView(t, "acme"), "acme").Clone; c != nil {
			t.Errorf("on the %s, the clone is %+v, want none", name, c)
		}
	}
}

func TestAReportOnAnotherServiceSendsTheProjectsNoticeOnTheTick(t *testing.T) {
	laptop, desktop := twoServices(t, store.NewMemory())
	tick := watchFactories(t, desktop)
	beat(laptop, t, "cc", "box", core.HeartbeatOnline, 10)
	tick()
	laptop.reportRepositories(t, core.RepositoryReport{Name: "acme", Path: "/src/acme", State: core.CloneMissing, Problem: "not a clone"})
	if got := tick(); !slices.Equal(got, []string{"acme"}) {
		t.Fatalf("a new clone report's tick reported %v, want acme", got)
	}
	laptop.reportRepositories(t, core.RepositoryReport{Name: "acme", Path: "/src/acme", State: core.CloneMissing, Problem: "not a clone"})
	if got := tick(); len(got) != 0 {
		t.Fatalf("an unchanged clone report's tick reported %v", got)
	}
}

// A Factory that moves to another service is overwritten there: the
// newest Heartbeat wins on both, whichever ticks first.
func TestAFactorySwitchingServicesShowsItsNewestHeartbeat(t *testing.T) {
	laptop, desktop := twoServices(t, store.NewMemory())
	laptopTick, desktopTick := watchFactories(t, laptop), watchFactories(t, desktop)
	beatNote(laptop, t, "cc", "box", core.HeartbeatOnline, "on the laptop", 10)
	desktopTick()
	laptop.clock.Advance(5 * time.Second)
	beatNote(desktop, t, "cc", "box", core.HeartbeatOnline, "on the desktop", 10)
	laptopTick()
	desktopTick()

	for name, f := range map[string]fixture{"laptop": laptop, "desktop": desktop} {
		got := onlyFactory(t, f)
		if got.Note != "on the desktop" || !got.LastHeartbeat.Equal(t0.Add(5*time.Second)) {
			t.Errorf("on the %s, %+v, want the desktop's Heartbeat", name, got)
		}
	}
	if r := onlyFactory(t, laptop).ReportsTo; r != "desktop" {
		t.Errorf("on the laptop the Factory reports to %q, want desktop", r)
	}
	if r := onlyFactory(t, desktop).ReportsTo; r != "" {
		t.Errorf("on the desktop the Factory reports to %q, want this service", r)
	}
}

// A Factory Offline for a day leaves every service and the Store.
func TestAFactoryOfflineForADayIsPruned(t *testing.T) {
	for _, c := range []struct {
		name    string
		state   core.HeartbeatState
		offline time.Duration // after its last Heartbeat
	}{{"stopping", core.HeartbeatStopping, 0}, {"lapsed", core.HeartbeatOnline, 90 * time.Second}} {
		t.Run(c.name, func(t *testing.T) {
			s := store.NewMemory()
			laptop, desktop := twoServices(t, s)
			laptopTick, desktopTick := watchFactories(t, laptop), watchFactories(t, desktop)
			beat(laptop, t, "cc", "box", c.state, 10)

			laptop.clock.Advance(c.offline + 24*time.Hour - time.Second)
			laptopTick()
			desktopTick()
			if fs := factoryStatus(desktop, t).Factories; len(fs) != 1 {
				t.Fatalf("a second short of a day Offline, the desktop lists %+v", fs)
			}
			laptop.clock.Advance(time.Second)
			laptopTick()
			desktopTick()
			for name, f := range map[string]fixture{"laptop": laptop, "desktop": desktop} {
				if fs := factoryStatus(f, t).Factories; len(fs) != 0 {
					t.Errorf("a day Offline, the %s lists %+v", name, fs)
				}
			}
			if hs, err := s.Services().Heartbeats(); err != nil || len(hs) != 0 {
				t.Errorf("the Store keeps %+v, %v", hs, err)
			}
		})
	}
}

// A Store that can't be reached doesn't fail report_heartbeat: the
// Heartbeat counts here, and the error is logged.
func TestAHeartbeatTheStoreCannotRecordStillCounts(t *testing.T) {
	s := &failingServices{Store: store.NewMemory(), err: errors.New("connection refused")}
	clk := clock.NewFake(t0)
	f := fixture{core: core.New(s, clk, &ids.Sequence{}), clock: clk, store: s}
	var log strings.Builder
	f.core.SetLog(&log)
	f.core.SetService(core.Service{ID: "a", Host: "laptop"})
	f.project(t, "acme")
	tick := watchFactories(t, f)

	beat(f, t, "cc", "box", core.HeartbeatOnline, 10)
	f.reportBase(t, core.ReportBaseInput{State: core.BaseDiverged, Local: "aaa111", Remote: "bbb222"})
	tick()
	fs := factoryStatus(f, t)
	if fs.Status != core.FactoryOnline || fs.Base == nil {
		t.Fatalf("%+v, want Online with the base, though the Store failed", fs)
	}
	if !strings.Contains(log.String(), "connection refused") {
		t.Fatalf("logged %q, want the Store's error", log.String())
	}
}

// A service starting on a Store shows the Factories reporting to another
// as soon as it has loaded, before its first tick, so a Heartbeat reaching
// it then keeps the Factory's reports.
func TestAServiceShowsTheFactoriesOnceItHasLoaded(t *testing.T) {
	laptop, desktop := twoServices(t, store.NewMemory())
	beat(laptop, t, "cc", "box", core.HeartbeatOnline, 10)
	laptop.reportBase(t, core.ReportBaseInput{State: core.BaseDiverged, Local: "aaa111", Remote: "bbb222"})

	desktop.core.LoadFactories()
	if got := onlyFactory(t, desktop); got.Status != core.FactoryOnline || got.ReportsTo != "laptop" {
		t.Fatalf("once loaded, the desktop lists %+v, want Online, reporting to laptop", got)
	}
	laptop.clock.Advance(10 * time.Second)
	beat(desktop, t, "cc", "box", core.HeartbeatOnline, 10)
	if b := factoryStatus(desktop, t).Base; b == nil || b.State != core.BaseDiverged {
		t.Fatalf("the Factory moved to the desktop, its base is %+v, want diverged", b)
	}
}
