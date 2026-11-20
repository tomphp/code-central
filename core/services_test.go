package core_test

import (
	"errors"
	"testing"
	"time"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/store"
)

// The services running on one Store (ADR 0068): each registers itself,
// and each Project's Watchers run on the one service holding its Watcher
// Lease, whose host its Watcher status shows.

// laptopAndDesktop is two services, one on laptop and one on desktop, on one
// Store and one fake clock, with acme's CI Tracking on.
func laptopAndDesktop(t *testing.T) (laptop, desktop fixture) {
	t.Helper()
	s := store.NewMemory()
	clk := clock.NewFake(t0)
	laptop = fixture{core: core.New(s, clk, &ids.Sequence{}), clock: clk, store: s}
	desktop = fixture{core: core.New(s, clk, &ids.Sequence{}), clock: clk, store: s}
	laptop.core.SetService(core.Service{ID: "a", Host: "laptop", Version: "v1.2.0"})
	desktop.core.SetService(core.Service{ID: "b", Host: "desktop", Version: "v1.2.0"})
	for _, f := range []fixture{laptop, desktop} {
		if err := f.core.Register(ctx); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	laptop.project(t, "acme")
	laptop.trackCI(t)
	return laptop, desktop
}

func (f fixture) holdLease(t *testing.T, key string) bool {
	t.Helper()
	held, err := f.core.HoldWatcherLease(ctx, key)
	if err != nil {
		t.Fatalf("hold %s's Watcher Lease: %v", key, err)
	}
	return held
}

func (f fixture) ciWatcher(t *testing.T) core.WatcherView {
	t.Helper()
	return f.projectView(t, "acme").CI.Watcher
}

func TestAProjectsWatcherStatusShowsTheHostHoldingItsLease(t *testing.T) {
	laptop, desktop := laptopAndDesktop(t)
	if !laptop.holdLease(t, "acme") {
		t.Fatal("the laptop didn't take acme's free Watcher Lease")
	}
	if desktop.holdLease(t, "acme") {
		t.Fatal("the desktop took the Watcher Lease the laptop holds")
	}
	if _, err := laptop.core.ReportCIWatcherStatus(ctx, "acme", core.ReportWatcherStatusInput{State: core.WatcherWatching}); err != nil {
		t.Fatal(err)
	}

	if w := laptop.ciWatcher(t); w.State != core.WatcherWatching || w.Host != "laptop" {
		t.Fatalf("on the laptop, the CI Watcher is %+v, want watching on laptop", w)
	}
	if w := desktop.ciWatcher(t); w.State != core.WatcherElsewhere || w.Host != "laptop" {
		t.Fatalf("on the desktop, the CI Watcher is %+v, want elsewhere, on laptop", w)
	}
}

// A Lease no live service renews lapses, and the status waits for a
// service to take it; the next service to try takes it over, and the
// one that held it sees it gone and forgets its own Watcher's status.
func TestALapsedWatcherLeaseWaitsUntilAnotherServiceTakesItOver(t *testing.T) {
	laptop, desktop := laptopAndDesktop(t)
	laptop.holdLease(t, "acme")
	if _, err := laptop.core.ReportCIWatcherStatus(ctx, "acme", core.ReportWatcherStatusInput{State: core.WatcherWatching}); err != nil {
		t.Fatal(err)
	}
	desktop.holdLease(t, "acme")

	laptop.clock.Advance(store.ServiceLife)
	if w := desktop.ciWatcher(t); w.State != core.WatcherWaiting || w.Message != "Waiting for a service to take the Watchers." {
		t.Fatalf("with the Lease lapsed, the CI Watcher is %+v, want waiting", w)
	}

	if !desktop.holdLease(t, "acme") {
		t.Fatal("the desktop didn't take over the lapsed Lease")
	}
	if w := desktop.ciWatcher(t); w.State != core.WatcherNotReported || w.Host != "desktop" {
		t.Fatalf("on the desktop, the CI Watcher is %+v, want not reported yet, on desktop", w)
	}
	if laptop.holdLease(t, "acme") {
		t.Fatal("the laptop took back the Lease the desktop holds")
	}
	if w := laptop.ciWatcher(t); w.State != core.WatcherElsewhere || w.Host != "desktop" {
		t.Fatalf("on the laptop, the CI Watcher is %+v, want elsewhere, on desktop", w)
	}

	// The desktop stops; the laptop takes the Lease back, and its Watcher
	// hasn't reported since.
	if err := desktop.core.Deregister(ctx); err != nil {
		t.Fatal(err)
	}
	if !laptop.holdLease(t, "acme") {
		t.Fatal("the laptop didn't take the Lease the desktop let go of")
	}
	if w := laptop.ciWatcher(t); w.State != core.WatcherNotReported || w.Host != "laptop" {
		t.Fatalf("on the laptop again, the CI Watcher is %+v, want not reported, on laptop", w)
	}
}

// A service that holds no Lease yet shows its own Watcher's status alone,
// as a service always did.
func TestAWatcherStatusWithNoLeaseKnownShowsNoHost(t *testing.T) {
	laptop, _ := laptopAndDesktop(t)
	if w := laptop.ciWatcher(t); w.State != core.WatcherNotReported || w.Host != "" {
		t.Fatalf("the CI Watcher is %+v, want not reported, with no host", w)
	}
}

// A Lease the Store can't renew is still held until it lapses, as last
// known, so a moment's outage doesn't stop the Watchers.
func TestAWatcherLeaseTheStoreCannotRenewIsHeldUntilItLapses(t *testing.T) {
	s := &failingServices{Store: store.NewMemory()}
	clk := clock.NewFake(t0)
	f := fixture{core: core.New(s, clk, &ids.Sequence{}), clock: clk, store: s}
	f.core.SetService(core.Service{ID: "a", Host: "laptop"})
	f.project(t, "acme")
	f.holdLease(t, "acme")

	s.err = errors.New("connection refused")
	clk.Advance(store.ServiceRenewal)
	if held, err := f.core.HoldWatcherLease(ctx, "acme"); !held || err == nil {
		t.Fatalf("held %v, %v: want still held, and the error to log", held, err)
	}
	clk.Advance(store.ServiceLife)
	if held, _ := f.core.HoldWatcherLease(ctx, "acme"); held {
		t.Fatal("held a Lease that has lapsed")
	}
}

// failingServices is a Store whose Services fail with err, once it is
// set.
type failingServices struct {
	store.Store
	err error
}

func (s *failingServices) Services() store.Services {
	return failing{s.Store.Services(), s}
}

type failing struct {
	store.Services
	s *failingServices
}

func (f failing) HoldLease(project, id string, now time.Time) (store.Lease, error) {
	if f.s.err != nil {
		return store.Lease{}, f.s.err
	}
	return f.Services.HoldLease(project, id, now)
}

func (f failing) PutHeartbeat(h store.Heartbeat) error {
	if f.s.err != nil {
		return f.s.err
	}
	return f.Services.PutHeartbeat(h)
}

func (f failing) Heartbeats() ([]store.Heartbeat, error) {
	if f.s.err != nil {
		return nil, f.s.err
	}
	return f.Services.Heartbeats()
}

func (f failing) PutFactoryReports(r store.FactoryReports) error {
	if f.s.err != nil {
		return f.s.err
	}
	return f.Services.PutFactoryReports(r)
}

func (f failing) FactoryReports() ([]store.FactoryReports, error) {
	if f.s.err != nil {
		return nil, f.s.err
	}
	return f.Services.FactoryReports()
}

func (f failing) PutTranscriptHolder(h store.TranscriptHolder) error {
	if f.s.err != nil {
		return f.s.err
	}
	return f.Services.PutTranscriptHolder(h)
}

func (f failing) TranscriptHolder(project string, ticket, claim int) (store.TranscriptHolder, bool, error) {
	if f.s.err != nil {
		return store.TranscriptHolder{}, false, f.s.err
	}
	return f.Services.TranscriptHolder(project, ticket, claim)
}

func (f failing) PutPing(p store.Ping) error {
	if f.s.err != nil {
		return f.s.err
	}
	return f.Services.PutPing(p)
}

func (f failing) Pings() ([]store.Ping, error) {
	if f.s.err != nil {
		return nil, f.s.err
	}
	return f.Services.Pings()
}

func (f failing) PrunePings(before time.Time) error {
	if f.s.err != nil {
		return f.s.err
	}
	return f.Services.PrunePings(before)
}
