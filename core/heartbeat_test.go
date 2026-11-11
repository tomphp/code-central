package core_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/events"
)

func beat(f fixture, t *testing.T, factory, host string, state core.HeartbeatState, interval int) {
	t.Helper()
	beatNote(f, t, factory, host, state, "", interval)
}

func beatNote(f fixture, t *testing.T, factory, host string, state core.HeartbeatState, note string, interval int) {
	t.Helper()
	_, err := f.core.ReportHeartbeat(ctx, "acme", core.ReportHeartbeatInput{Factory: factory, Host: host, State: state, Note: note, IntervalSeconds: interval})
	if err != nil {
		t.Fatal(err)
	}
}

func factoryStatus(f fixture, t *testing.T) core.FactoryStatusView {
	t.Helper()
	p, err := f.core.GetProject(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	return p.FactoryStatus
}

func TestAFreshCoreHasNoFactoriesAndIsOffline(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	fs := factoryStatus(f, t)
	if fs.Status != core.FactoryOffline || fs.Factories == nil || len(fs.Factories) != 0 {
		t.Fatalf("factory_status = %+v", fs)
	}
}

func TestAFactoryIsOnlineDrainingOrOffline(t *testing.T) {
	for state, want := range map[core.HeartbeatState]core.FactoryStatusName{
		core.HeartbeatOnline: core.FactoryOnline, core.HeartbeatDraining: core.FactoryDraining, core.HeartbeatStopping: core.FactoryOffline,
	} {
		f := newFixture(t)
		f.project(t, "acme")
		beatNote(f, t, "cc", "box", state, "hello", 10)
		fs := factoryStatus(f, t)
		if fs.Status != want || len(fs.Factories) != 1 || fs.Factories[0].Status != want {
			t.Errorf("%s: %+v, want %s", state, fs, want)
		}
		got := fs.Factories[0]
		if got.Note != "hello" || !got.LastHeartbeat.Equal(t0) || got.IntervalSeconds != 10 || got.Name != "cc" || got.Host != "box" {
			t.Errorf("%s: factory = %+v", state, got)
		}
	}
}

func TestAFactoryGoesOfflineAtThreeIntervalsWithA90SecondFloor(t *testing.T) {
	for _, c := range []struct {
		interval int
		deadline time.Duration
	}{{10, 90 * time.Second}, {30, 90 * time.Second}, {60, 180 * time.Second}} {
		f := newFixture(t)
		f.project(t, "acme")
		beatNote(f, t, "cc", "box", core.HeartbeatOnline, "n", c.interval)
		f.clock.Set(t0.Add(c.deadline - time.Second))
		if s := factoryStatus(f, t).Status; s != core.FactoryOnline {
			t.Errorf("interval %d just before: %s", c.interval, s)
		}
		f.clock.Set(t0.Add(c.deadline))
		fs := factoryStatus(f, t)
		if fs.Status != core.FactoryOffline || fs.Factories[0].Note != "n" {
			t.Errorf("interval %d at deadline: %+v", c.interval, fs)
		}
	}
}

func TestADrainingFactoryAlsoGoesOffline(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	beat(f, t, "cc", "box", core.HeartbeatDraining, 10)
	f.clock.Set(t0.Add(90 * time.Second))
	if s := factoryStatus(f, t).Status; s != core.FactoryOffline {
		t.Fatalf("status %s", s)
	}
}

func TestTheCombinedStatusIsOnlineThenDrainingThenOffline(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	beat(f, t, "a", "h", core.HeartbeatStopping, 10)
	if s := factoryStatus(f, t).Status; s != core.FactoryOffline {
		t.Fatalf("stopping only: %s", s)
	}
	beat(f, t, "b", "h", core.HeartbeatDraining, 10)
	if s := factoryStatus(f, t).Status; s != core.FactoryDraining {
		t.Fatalf("draining: %s", s)
	}
	beat(f, t, "c", "h", core.HeartbeatOnline, 10)
	if s := factoryStatus(f, t).Status; s != core.FactoryOnline {
		t.Fatalf("online: %s", s)
	}
}

func TestAFactoryIsIdentifiedByNameAndHostAndTheLatestReplaces(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	beat(f, t, "b", "z", core.HeartbeatOnline, 10)
	beat(f, t, "b", "a", core.HeartbeatOnline, 10)
	beat(f, t, "a", "z", core.HeartbeatOnline, 10)
	f.clock.Advance(5 * time.Second)
	beatNote(f, t, " a ", " z ", core.HeartbeatDraining, "later", 20)
	var got []string
	for _, fv := range factoryStatus(f, t).Factories {
		got = append(got, fv.Name+"@"+fv.Host+":"+string(fv.Status))
	}
	if strings.Join(got, " ") != "a@z:draining b@a:online b@z:online" {
		t.Fatalf("factories = %v", got)
	}
}

func TestAHeartbeatsNoteIsTrimmedAndCutTo200Characters(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	beatNote(f, t, "cc", "box", core.HeartbeatOnline, "  "+strings.Repeat("é", 250), 10)
	if n := factoryStatus(f, t).Factories[0].Note; n != strings.Repeat("é", 200) {
		t.Fatalf("note has %d runes", len([]rune(n)))
	}
}

func TestAHeartbeatIsRefusedWithTheFieldNamed(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	ok := core.ReportHeartbeatInput{Factory: "f", Host: "h", State: core.HeartbeatOnline, IntervalSeconds: 10}
	for field, mutate := range map[string]func(*core.ReportHeartbeatInput){
		"factory":          func(in *core.ReportHeartbeatInput) { in.Factory = "  " },
		"host":             func(in *core.ReportHeartbeatInput) { in.Host = "" },
		"state":            func(in *core.ReportHeartbeatInput) { in.State = "asleep" },
		"interval_seconds": func(in *core.ReportHeartbeatInput) { in.IntervalSeconds = 0 },
	} {
		in := ok
		mutate(&in)
		_, err := f.core.ReportHeartbeat(ctx, "acme", in)
		wantCode(t, err, core.Invalid, field)
	}
	in := ok
	in.IntervalSeconds = 86401
	_, err := f.core.ReportHeartbeat(ctx, "acme", in)
	wantCode(t, err, core.Invalid, "interval_seconds")
	_, err = f.core.ReportHeartbeat(ctx, "nope", ok)
	wantCode(t, err, core.UnknownProject, "")
}

func TestAHeartbeatSendsTheProjectsNoticeOnlyWhenWhatTheHumanSeesChanges(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	notices, stop := f.core.Events().Subscribe()
	defer stop()
	want := events.Event{Type: events.ProjectChanged, Project: "acme"}
	expect := func(what string, send bool) {
		t.Helper()
		if send {
			if got := <-notices; got != want {
				t.Fatalf("%s: got %+v", what, got)
			}
		}
		noNotice(t, notices)
	}

	beatNote(f, t, "cc", "box", core.HeartbeatOnline, "a", 10)
	expect("new Factory", true)
	f.clock.Advance(10 * time.Second)
	beatNote(f, t, "cc", "box", core.HeartbeatOnline, "a", 10)
	expect("repeat", false)
	beatNote(f, t, "cc", "box", core.HeartbeatOnline, "b", 10)
	expect("new note", true)
	beatNote(f, t, "cc", "box", core.HeartbeatDraining, "b", 10)
	expect("new status", true)
	beatNote(f, t, "cc", "other", core.HeartbeatOnline, "b", 10)
	expect("another host", true)
	f.clock.Advance(5 * time.Minute) // both are Offline by now
	beatNote(f, t, "cc", "other", core.HeartbeatOnline, "b", 10)
	expect("back from Offline", true)
}

func TestAHeartbeatAcknowledgesWithoutFactoryStatusNoise(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	w, err := f.core.ReportHeartbeat(ctx, "acme", core.ReportHeartbeatInput{Factory: "f", Host: "h", State: core.HeartbeatOnline, IntervalSeconds: 10})
	if err != nil {
		t.Fatal(err)
	}
	if w.Record != nil || w.ID != "acme" || len(w.Changed) != 0 {
		t.Fatalf("acknowledgement = %+v", w)
	}
	full, err := f.core.ReportHeartbeat(ctx, "acme", core.ReportHeartbeatInput{Factory: "f", Host: "h", State: core.HeartbeatOnline, IntervalSeconds: 10, Full: true})
	if err != nil || full.Record == nil || full.Record.FactoryStatus.Status != core.FactoryOnline {
		t.Fatalf("full = %+v, %v", full, err)
	}
}

// watchFactories runs f.core.WatchFactoryStatus until the test ends and
// returns tick: one tick, returning the Projects it reported.
func watchFactories(t *testing.T, f fixture) (tick func() []string) {
	t.Helper()
	ticks := make(chan time.Time)
	changes := make(chan string, 10)
	watchCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		f.core.WatchFactoryStatus(watchCtx, ticks, func(project string) { changes <- project })
		close(done)
	}()
	t.Cleanup(func() { stop(); <-done })
	return func() []string {
		ticks <- f.clock.Now()
		ticks <- f.clock.Now() // the second send returns once the first tick is handled
		var got []string
		for {
			select {
			case p := <-changes:
				got = append(got, p)
			default:
				return got
			}
		}
	}
}

func TestTheWatchReportsAFactoryGoingOfflineOnceAndAnUnchangedProjectNever(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.project(t, "other")
	tick := watchFactories(t, f)
	if got := tick(); len(got) != 0 {
		t.Fatalf("first tick reported %v", got)
	}
	beat(f, t, "cc", "box", core.HeartbeatOnline, 10)
	if got := tick(); len(got) != 1 || got[0] != "acme" {
		t.Fatalf("after a new Factory reported %v, want acme", got)
	}
	f.clock.Advance(89 * time.Second)
	if got := tick(); len(got) != 0 {
		t.Fatalf("before the deadline reported %v", got)
	}
	f.clock.Advance(time.Second)
	if got := tick(); len(got) != 1 || got[0] != "acme" {
		t.Fatalf("at the deadline reported %v, want acme", got)
	}
	f.clock.Advance(time.Second)
	if got := tick(); len(got) != 0 {
		t.Fatalf("next tick reported %v", got)
	}
}
