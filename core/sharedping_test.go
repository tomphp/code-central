package core_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/store"
)

// Pings shared across services (Spec code-central-1107): a Claim pinging
// one service is active on every service sharing the Store once that
// service's Factory Status watch has ticked, so Silent Claims and last
// activity agree.

// claimedOnBoth is two services on s with Claim acme-2 for session s1 and
// the desktop's tick.
func claimedOnBoth(t *testing.T, s store.Store) (laptop, desktop fixture, tick func() []string) {
	t.Helper()
	laptop, desktop = twoServices(t, s)
	laptop.longLived(t, laptop.spec(t, "acme", "Login"))
	laptop.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Ta", State: domain.Ready})
	laptop.clock.Advance(time.Minute)
	laptop.claim(t, core.ClaimInput{SessionID: "s1"})
	return laptop, desktop, watchFactories(t, desktop)
}

// silentOn is whether f shows acme-2 as a Silent Claim.
func silentOn(t *testing.T, f fixture) bool {
	t.Helper()
	return f.get(t, "acme-2").SilentSince != nil
}

func TestAClaimPingingOneServiceIsNotSilentOnAnotherAndOneQuietOnBothIsSilentOnBoth(t *testing.T) {
	laptop, desktop, tick := claimedOnBoth(t, store.NewMemory())

	laptop.clock.Advance(25 * time.Minute)
	laptop.core.Ping(ctx, "s1")
	pinged := laptop.clock.Now()
	laptop.clock.Advance(20 * time.Minute) // 45 minutes since the claim, 20 since the ping
	tick()
	for name, f := range map[string]fixture{"laptop": laptop, "desktop": desktop} {
		if silentOn(t, f) {
			t.Errorf("the Claim pinged on the laptop is Silent on the %s", name)
		}
		if la := f.get(t, "acme-2").Claims[0].LastActivity; !la.Equal(pinged) {
			t.Errorf("on the %s the Claim's last activity is %v, want the ping at %v", name, la, pinged)
		}
	}

	laptop.clock.Advance(10 * time.Minute) // 30 minutes since the ping
	tick()
	for name, f := range map[string]fixture{"laptop": laptop, "desktop": desktop} {
		if !silentOn(t, f) {
			t.Errorf("the Claim quiet for 30 minutes is not Silent on the %s", name)
		}
	}
}

// A service started in the last 30 minutes keeps its own start as the
// floor of last activity, so it lags the others in flagging a Claim Silent
// but never flags one falsely. This is the one exception to "Silent on
// every service or on none".
func TestAServiceStartedRecentlyNeverFlagsAClaimSilentFalsely(t *testing.T) {
	s := store.NewMemory()
	laptop, _, _ := claimedOnBoth(t, s)
	laptop.clock.Advance(time.Hour)
	late := core.New(s, laptop.clock, &ids.Sequence{})
	late.SetService(core.Service{ID: "c", Host: "phone"})
	laptop.core.Ping(ctx, "s1")
	laptop.clock.Advance(20 * time.Minute)

	v, err := late.GetTicket(ctx, "acme", whole("2"))
	if err != nil {
		t.Fatal(err)
	}
	if v.SilentSince != nil {
		t.Fatalf("a service started 20 minutes ago flags the Claim Silent since %v", *v.SilentSince)
	}
}

// The throttle is per service: a session that moves to another service
// writes from its first ping there, even a few seconds after the last one.
func TestASessionMovingToAnotherServiceIsCountedFromItsFirstPing(t *testing.T) {
	s := store.NewMemory()
	laptop, desktop, tick := claimedOnBoth(t, s)
	laptop.core.Ping(ctx, "s1")
	tick()
	laptop.clock.Advance(10 * time.Second)
	desktop.core.Ping(ctx, "s1")

	pings, _ := s.Services().Pings()
	if len(pings) != 1 || pings[0].ServiceHost != "desktop" || !pings[0].At.Equal(laptop.clock.Now()) {
		t.Fatalf("pings %+v, want the desktop's ping 10s later to replace the laptop's", pings)
	}
}

func TestPingRowsOlderThanAWeekArePruned(t *testing.T) {
	s := store.NewMemory()
	laptop, _, tick := claimedOnBoth(t, s)
	laptop.core.Ping(ctx, "s1")
	laptop.clock.Advance(6 * 24 * time.Hour)
	tick()
	if pings, _ := s.Services().Pings(); len(pings) != 1 {
		t.Fatalf("pings %+v after 6 days, want the one", pings)
	}
	laptop.clock.Advance(2 * 24 * time.Hour)
	tick()
	if pings, _ := s.Services().Pings(); len(pings) != 0 {
		t.Fatalf("pings %+v after 8 days, want none", pings)
	}
}

func TestOnlyCountedPingsAreWritten(t *testing.T) {
	s := store.NewMemory()
	laptop, _, _ := claimedOnBoth(t, s)
	laptop.core.Ping(ctx, "s1")
	laptop.clock.Advance(30 * time.Second)
	laptop.core.Ping(ctx, "s1") // throttled
	laptop.core.Ping(ctx, "")
	pings, _ := s.Services().Pings()
	if len(pings) != 1 || pings[0].Session != "s1" || !pings[0].At.Equal(t0.Add(time.Minute)) || pings[0].Service != "a" || pings[0].ServiceHost != "laptop" {
		t.Fatalf("pings %+v, want one for s1 at the counted time, reaching laptop", pings)
	}
}

func TestAPingTheStoreCannotRecordStillCounts(t *testing.T) {
	s := &failingServices{Store: store.NewMemory(), err: errors.New("connection refused")}
	clk := clock.NewFake(t0)
	f := fixture{core: core.New(s, clk, &ids.Sequence{}), clock: clk, store: s}
	var log strings.Builder
	f.core.SetLog(&log)
	f.work(t, domain.Ready)
	f.claim(t, core.ClaimInput{SessionID: "s1"})
	tick := watchFactories(t, f)

	f.clock.Advance(25 * time.Minute)
	f.core.Ping(ctx, "s1")
	pinged := f.clock.Now()
	f.clock.Advance(20 * time.Minute)
	tick()
	v := f.get(t, "acme-2")
	if v.SilentSince != nil || !v.Claims[0].LastActivity.Equal(pinged) {
		t.Fatalf("Silent %v, last activity %v: want the ping at %v to count, though the Store failed", v.SilentSince, v.Claims[0].LastActivity, pinged)
	}
	if !strings.Contains(log.String(), "connection refused") {
		t.Fatalf("logged %q, want the Store's error", log.String())
	}
}
