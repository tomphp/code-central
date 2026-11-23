package factories_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/core/coretest"
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
	laptop.LongLived(t, laptop.Spec(t, "acme", "Login"))
	laptop.Ticket(t, "acme", api.CreateTicketInput{Spec: "1", Title: "Ta", State: domain.Ready})
	laptop.Clock.Advance(time.Minute)
	laptop.Claim(t, api.ClaimInput{SessionID: "s1"})
	return laptop, desktop, desktop.WatchFactories(t)
}

// silentOn is whether f shows acme-2 as a Silent Claim.
func silentOn(t *testing.T, f fixture) bool {
	t.Helper()
	return f.Get(t, "acme-2").SilentSince != nil
}

func TestAClaimPingingOneServiceIsNotSilentOnAnotherAndOneQuietOnBothIsSilentOnBoth(t *testing.T) {
	laptop, desktop, tick := claimedOnBoth(t, store.NewMemory())

	laptop.Clock.Advance(25 * time.Minute)
	laptop.Core.Factories().Ping(ctx, "s1")
	pinged := laptop.Clock.Now()
	laptop.Clock.Advance(20 * time.Minute) // 45 minutes since the claim, 20 since the ping
	tick()
	for name, f := range map[string]fixture{"laptop": laptop, "desktop": desktop} {
		if silentOn(t, f) {
			t.Errorf("the Claim pinged on the laptop is Silent on the %s", name)
		}
		if la := f.Get(t, "acme-2").Claims[0].LastActivity; !la.Equal(pinged) {
			t.Errorf("on the %s the Claim's last activity is %v, want the ping at %v", name, la, pinged)
		}
	}

	laptop.Clock.Advance(10 * time.Minute) // 30 minutes since the ping
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
	laptop.Clock.Advance(time.Hour)
	late := core.New(s, laptop.Clock, &ids.Sequence{})
	late.SetService(core.Service{ID: "c", Host: "phone"})
	laptop.Core.Factories().Ping(ctx, "s1")
	laptop.Clock.Advance(20 * time.Minute)

	v, err := late.GetTicket(ctx, "acme", api.GetTicketInput{Ticket: "2", Parts: api.TicketParts()})
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
	laptop.Core.Factories().Ping(ctx, "s1")
	tick()
	laptop.Clock.Advance(10 * time.Second)
	desktop.Core.Factories().Ping(ctx, "s1")

	pings, _ := s.Services().Pings()
	if len(pings) != 1 || pings[0].ServiceHost != "desktop" || !pings[0].At.Equal(laptop.Clock.Now()) {
		t.Fatalf("pings %+v, want the desktop's ping 10s later to replace the laptop's", pings)
	}
}

func TestPingRowsOlderThanAWeekArePruned(t *testing.T) {
	s := store.NewMemory()
	laptop, _, tick := claimedOnBoth(t, s)
	laptop.Core.Factories().Ping(ctx, "s1")
	laptop.Clock.Advance(6 * 24 * time.Hour)
	tick()
	if pings, _ := s.Services().Pings(); len(pings) != 1 {
		t.Fatalf("pings %+v after 6 days, want the one", pings)
	}
	laptop.Clock.Advance(2 * 24 * time.Hour)
	tick()
	if pings, _ := s.Services().Pings(); len(pings) != 0 {
		t.Fatalf("pings %+v after 8 days, want none", pings)
	}
}

func TestOnlyCountedPingsAreWritten(t *testing.T) {
	s := store.NewMemory()
	laptop, _, _ := claimedOnBoth(t, s)
	laptop.Core.Factories().Ping(ctx, "s1")
	laptop.Clock.Advance(30 * time.Second)
	laptop.Core.Factories().Ping(ctx, "s1") // throttled
	laptop.Core.Factories().Ping(ctx, "")
	pings, _ := s.Services().Pings()
	if len(pings) != 1 || pings[0].Session != "s1" || !pings[0].At.Equal(t0.Add(time.Minute)) || pings[0].Service != "a" || pings[0].ServiceHost != "laptop" {
		t.Fatalf("pings %+v, want one for s1 at the counted time, reaching laptop", pings)
	}
}

func TestAPingTheStoreCannotRecordStillCounts(t *testing.T) {
	s := &coretest.FailingServices{Store: store.NewMemory(), Err: errors.New("connection refused")}
	clk := clock.NewFake(t0)
	f := coretest.On(s, clk)
	var log strings.Builder
	f.Core.SetLog(&log)
	f.Work(t, domain.Ready)
	f.Claim(t, api.ClaimInput{SessionID: "s1"})
	tick := f.WatchFactories(t)

	f.Clock.Advance(25 * time.Minute)
	f.Core.Factories().Ping(ctx, "s1")
	pinged := f.Clock.Now()
	f.Clock.Advance(20 * time.Minute)
	tick()
	v := f.Get(t, "acme-2")
	if v.SilentSince != nil || !v.Claims[0].LastActivity.Equal(pinged) {
		t.Fatalf("Silent %v, last activity %v: want the ping at %v to count, though the Store failed", v.SilentSince, v.Claims[0].LastActivity, pinged)
	}
	if !strings.Contains(log.String(), "connection refused") {
		t.Fatalf("logged %q, want the Store's error", log.String())
	}
}
