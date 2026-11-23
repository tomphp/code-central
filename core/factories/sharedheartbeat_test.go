package factories_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/core/coretest"
	"github.com/tomoram/code-central/store"
)

// Heartbeats shared across services (Spec code-central-1107): a Factory
// reporting to one service shows, with its Plan Limits and its Base and
// clone reports, on every service sharing the Store, once that service's
// Factory Status watch has ticked.

func TestAFactoryReportingToOneServiceShowsOnAnotherAfterItsTick(t *testing.T) {
	laptop, desktop := twoServices(t, store.NewMemory())
	tick := desktop.WatchFactories(t)
	sendBeat(laptop, t, api.ReportHeartbeatInput{Factory: "cc", Host: "box", State: api.HeartbeatDraining, Note: "winding down", IntervalSeconds: 20, Chat: true})

	if got := tick(); !slices.Equal(got, []string{"acme"}) {
		t.Fatalf("the desktop's tick reported %v, want acme", got)
	}
	here, there := laptop.OnlyFactory(t), desktop.OnlyFactory(t)
	if here.ReportsTo != "" {
		t.Errorf("on the laptop the Factory reports to %q, want this service", here.ReportsTo)
	}
	if there.ReportsTo != "laptop" {
		t.Errorf("on the desktop the Factory reports to %q, want laptop", there.ReportsTo)
	}
	there.ReportsTo = ""
	if here != there || there.Status != api.FactoryDraining || there.Note != "winding down" || there.IntervalSeconds != 20 || !there.Chat || !there.LastHeartbeat.Equal(t0) {
		t.Fatalf("the desktop lists %+v, the laptop %+v: want the same Draining Factory", there, here)
	}
	if s := desktop.FactoryStatus(t).Status; s != api.FactoryDraining {
		t.Fatalf("the desktop's combined status is %s, want draining", s)
	}

	// A steady Factory sends no notice on the desktop's tick either.
	laptop.Clock.Advance(20 * time.Second)
	sendBeat(laptop, t, api.ReportHeartbeatInput{Factory: "cc", Host: "box", State: api.HeartbeatDraining, Note: "winding down", IntervalSeconds: 20, Chat: true})
	if got := tick(); len(got) != 0 {
		t.Fatalf("a steady Factory's tick reported %v", got)
	}
	// What the human sees changing sends one.
	sendBeat(laptop, t, api.ReportHeartbeatInput{Factory: "cc", Host: "box", State: api.HeartbeatDraining, Note: "nearly done", IntervalSeconds: 20, Chat: true})
	if got := tick(); !slices.Equal(got, []string{"acme"}) {
		t.Fatalf("a new note's tick reported %v, want acme", got)
	}

	laptop.Clock.Advance(90 * time.Second)
	if got := tick(); !slices.Equal(got, []string{"acme"}) {
		t.Fatalf("the deadline's tick reported %v, want acme", got)
	}
	for name, f := range map[string]fixture{"laptop": laptop, "desktop": desktop} {
		if fs := f.FactoryStatus(t); fs.Status != api.FactoryOffline || fs.Factories[0].Status != api.FactoryOffline {
			t.Errorf("past its deadline, on the %s, %+v, want Offline", name, fs)
		}
	}
}

func TestAStoppingFactoryIsOfflineOnEveryService(t *testing.T) {
	laptop, desktop := twoServices(t, store.NewMemory())
	tick := desktop.WatchFactories(t)
	laptop.Beat(t, "cc", "box", api.HeartbeatOnline, 10)
	tick()
	laptop.Beat(t, "cc", "box", api.HeartbeatStopping, 10)
	if got := tick(); !slices.Equal(got, []string{"acme"}) {
		t.Fatalf("the tick reported %v, want acme", got)
	}
	if s := desktop.OnlyFactory(t).Status; s != api.FactoryOffline {
		t.Fatalf("a stopping Factory is %s on the desktop, want offline", s)
	}
}

func TestPlanLimitsAndBaseAndCloneReportsShowOnEveryService(t *testing.T) {
	laptop, desktop := twoServices(t, store.NewMemory())
	tick := desktop.WatchFactories(t)
	sent := limitsAt(t0.Add(-time.Minute), 42.5, 7)
	beatLimits(laptop, t, "cc", sent)
	laptop.ReportBase(t, api.ReportBaseInput{State: api.BaseDiverged, Local: "aaa111", Remote: "bbb222"})
	laptop.ReportRepositories(t, api.RepositoryReport{Name: "acme", Path: "/src/acme", State: api.CloneReady})
	tick()

	fs := desktop.FactoryStatus(t)
	if pl := fs.PlanLimits; pl == nil || !pl.ObservedAt.Equal(sent.ObservedAt) || !sameWindow(pl.FiveHour, sent.FiveHour) || !sameWindow(pl.Weekly, sent.Weekly) {
		t.Errorf("the desktop's Plan Limits are %+v, want %+v", pl, sent)
	}
	if b := fs.Base; b == nil || b.State != api.BaseDiverged || b.Local != "aaa111" || b.Remote != "bbb222" {
		t.Errorf("the desktop's base is %+v, want diverged", b)
	}
	if c := desktop.Clone(t); c == nil || c.State != api.CloneReady || c.Path != "/src/acme" {
		t.Errorf("the desktop's clone is %+v, want ready at /src/acme", c)
	}

	// A Heartbeat with no Plan Limits clears them, and one back from
	// stopping drops the reports, on both.
	laptop.Beat(t, "cc", "box", api.HeartbeatStopping, 10)
	laptop.Clock.Advance(time.Second)
	laptop.Beat(t, "cc", "box", api.HeartbeatOnline, 10)
	tick()
	for name, f := range map[string]fixture{"laptop": laptop, "desktop": desktop} {
		fs := f.FactoryStatus(t)
		if fs.Status != api.FactoryOnline || fs.PlanLimits != nil || fs.Base != nil {
			t.Errorf("on the %s, %+v: want Online, with no Plan Limits or base", name, fs)
		}
		if c := f.Clone(t); c != nil {
			t.Errorf("on the %s, the clone is %+v, want none", name, c)
		}
	}
}

func TestAReportOnAnotherServiceSendsTheProjectsNoticeOnTheTick(t *testing.T) {
	laptop, desktop := twoServices(t, store.NewMemory())
	tick := desktop.WatchFactories(t)
	laptop.Beat(t, "cc", "box", api.HeartbeatOnline, 10)
	tick()
	laptop.ReportRepositories(t, api.RepositoryReport{Name: "acme", Path: "/src/acme", State: api.CloneMissing, Problem: "not a clone"})
	if got := tick(); !slices.Equal(got, []string{"acme"}) {
		t.Fatalf("a new clone report's tick reported %v, want acme", got)
	}
	laptop.ReportRepositories(t, api.RepositoryReport{Name: "acme", Path: "/src/acme", State: api.CloneMissing, Problem: "not a clone"})
	if got := tick(); len(got) != 0 {
		t.Fatalf("an unchanged clone report's tick reported %v", got)
	}
}

// A Factory that moves to another service is overwritten there: the
// newest Heartbeat wins on both, whichever ticks first.
func TestAFactorySwitchingServicesShowsItsNewestHeartbeat(t *testing.T) {
	laptop, desktop := twoServices(t, store.NewMemory())
	laptopTick, desktopTick := laptop.WatchFactories(t), desktop.WatchFactories(t)
	laptop.BeatNote(t, "cc", "box", api.HeartbeatOnline, "on the laptop", 10)
	desktopTick()
	laptop.Clock.Advance(5 * time.Second)
	desktop.BeatNote(t, "cc", "box", api.HeartbeatOnline, "on the desktop", 10)
	laptopTick()
	desktopTick()

	for name, f := range map[string]fixture{"laptop": laptop, "desktop": desktop} {
		got := f.OnlyFactory(t)
		if got.Note != "on the desktop" || !got.LastHeartbeat.Equal(t0.Add(5*time.Second)) {
			t.Errorf("on the %s, %+v, want the desktop's Heartbeat", name, got)
		}
	}
	if r := laptop.OnlyFactory(t).ReportsTo; r != "desktop" {
		t.Errorf("on the laptop the Factory reports to %q, want desktop", r)
	}
	if r := desktop.OnlyFactory(t).ReportsTo; r != "" {
		t.Errorf("on the desktop the Factory reports to %q, want this service", r)
	}
}

// A Factory Offline for a day leaves every service and the Store.
func TestAFactoryOfflineForADayIsPruned(t *testing.T) {
	for _, c := range []struct {
		name    string
		state   api.HeartbeatState
		offline time.Duration // after its last Heartbeat
	}{{"stopping", api.HeartbeatStopping, 0}, {"lapsed", api.HeartbeatOnline, 90 * time.Second}} {
		t.Run(c.name, func(t *testing.T) {
			s := store.NewMemory()
			laptop, desktop := twoServices(t, s)
			laptopTick, desktopTick := laptop.WatchFactories(t), desktop.WatchFactories(t)
			laptop.Beat(t, "cc", "box", c.state, 10)

			laptop.Clock.Advance(c.offline + 24*time.Hour - time.Second)
			laptopTick()
			desktopTick()
			if fs := desktop.FactoryStatus(t).Factories; len(fs) != 1 {
				t.Fatalf("a second short of a day Offline, the desktop lists %+v", fs)
			}
			laptop.Clock.Advance(time.Second)
			laptopTick()
			desktopTick()
			for name, f := range map[string]fixture{"laptop": laptop, "desktop": desktop} {
				if fs := f.FactoryStatus(t).Factories; len(fs) != 0 {
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
	s := &coretest.FailingServices{Store: store.NewMemory(), Err: errors.New("connection refused")}
	clk := clock.NewFake(t0)
	f := coretest.On(s, clk)
	var log strings.Builder
	f.Core.SetLog(&log)
	f.Core.SetService(core.Service{ID: "a", Host: "laptop"})
	f.Project(t, "acme")
	tick := f.WatchFactories(t)

	f.Beat(t, "cc", "box", api.HeartbeatOnline, 10)
	f.ReportBase(t, api.ReportBaseInput{State: api.BaseDiverged, Local: "aaa111", Remote: "bbb222"})
	tick()
	fs := f.FactoryStatus(t)
	if fs.Status != api.FactoryOnline || fs.Base == nil {
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
	laptop.Beat(t, "cc", "box", api.HeartbeatOnline, 10)
	laptop.ReportBase(t, api.ReportBaseInput{State: api.BaseDiverged, Local: "aaa111", Remote: "bbb222"})

	desktop.Core.Factories().LoadFactories()
	if got := desktop.OnlyFactory(t); got.Status != api.FactoryOnline || got.ReportsTo != "laptop" {
		t.Fatalf("once loaded, the desktop lists %+v, want Online, reporting to laptop", got)
	}
	laptop.Clock.Advance(10 * time.Second)
	desktop.Beat(t, "cc", "box", api.HeartbeatOnline, 10)
	if b := desktop.FactoryStatus(t).Base; b == nil || b.State != api.BaseDiverged {
		t.Fatalf("the Factory moved to the desktop, its base is %+v, want diverged", b)
	}
}
