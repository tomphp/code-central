package core_test

import (
	"errors"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/core/coretest"
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
	laptop = fixtureOf(coretest.On(s, clk))
	desktop = fixtureOf(coretest.On(s, clk))
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

func (f fixture) ciWatcher(t *testing.T) api.WatcherView {
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
	if _, err := laptop.core.ReportCIWatcherStatus(ctx, "acme", api.ReportWatcherStatusInput{State: api.WatcherWatching}); err != nil {
		t.Fatal(err)
	}

	if w := laptop.ciWatcher(t); w.State != api.WatcherWatching || w.Host != "laptop" {
		t.Fatalf("on the laptop, the CI Watcher is %+v, want watching on laptop", w)
	}
	if w := desktop.ciWatcher(t); w.State != api.WatcherElsewhere || w.Host != "laptop" {
		t.Fatalf("on the desktop, the CI Watcher is %+v, want elsewhere, on laptop", w)
	}
}

// A Lease no live service renews lapses, and the status waits for a
// service to take it; the next service to try takes it over, and the
// one that held it sees it gone and forgets its own Watcher's status.
func TestALapsedWatcherLeaseWaitsUntilAnotherServiceTakesItOver(t *testing.T) {
	laptop, desktop := laptopAndDesktop(t)
	laptop.holdLease(t, "acme")
	if _, err := laptop.core.ReportCIWatcherStatus(ctx, "acme", api.ReportWatcherStatusInput{State: api.WatcherWatching}); err != nil {
		t.Fatal(err)
	}
	desktop.holdLease(t, "acme")

	laptop.clock.Advance(store.ServiceLife)
	if w := desktop.ciWatcher(t); w.State != api.WatcherWaiting || w.Message != "Waiting for a service to take the Watchers." {
		t.Fatalf("with the Lease lapsed, the CI Watcher is %+v, want waiting", w)
	}

	if !desktop.holdLease(t, "acme") {
		t.Fatal("the desktop didn't take over the lapsed Lease")
	}
	if w := desktop.ciWatcher(t); w.State != api.WatcherNotReported || w.Host != "desktop" {
		t.Fatalf("on the desktop, the CI Watcher is %+v, want not reported yet, on desktop", w)
	}
	if laptop.holdLease(t, "acme") {
		t.Fatal("the laptop took back the Lease the desktop holds")
	}
	if w := laptop.ciWatcher(t); w.State != api.WatcherElsewhere || w.Host != "desktop" {
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
	if w := laptop.ciWatcher(t); w.State != api.WatcherNotReported || w.Host != "laptop" {
		t.Fatalf("on the laptop again, the CI Watcher is %+v, want not reported, on laptop", w)
	}
}

// A service that holds no Lease yet shows its own Watcher's status alone,
// as a service always did.
func TestAWatcherStatusWithNoLeaseKnownShowsNoHost(t *testing.T) {
	laptop, _ := laptopAndDesktop(t)
	if w := laptop.ciWatcher(t); w.State != api.WatcherNotReported || w.Host != "" {
		t.Fatalf("the CI Watcher is %+v, want not reported, with no host", w)
	}
}

// A Lease the Store can't renew is still held until it lapses, as last
// known, so a moment's outage doesn't stop the Watchers.
func TestAWatcherLeaseTheStoreCannotRenewIsHeldUntilItLapses(t *testing.T) {
	s := &coretest.FailingServices{Store: store.NewMemory()}
	clk := clock.NewFake(t0)
	f := fixtureOf(coretest.On(s, clk))
	f.core.SetService(core.Service{ID: "a", Host: "laptop"})
	f.project(t, "acme")
	f.holdLease(t, "acme")

	s.Err = errors.New("connection refused")
	clk.Advance(store.ServiceRenewal)
	if held, err := f.core.HoldWatcherLease(ctx, "acme"); !held || err == nil {
		t.Fatalf("held %v, %v: want still held, and the error to log", held, err)
	}
	clk.Advance(store.ServiceLife)
	if held, _ := f.core.HoldWatcherLease(ctx, "acme"); held {
		t.Fatal("held a Lease that has lapsed")
	}
}
