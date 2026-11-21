package factory_test

import (
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"

	"github.com/tomoram/code-central/factory/factorytest"
)

// Plan Limits (Spec code-central-861): the Factory reads them from each
// Turn's rate_limit_event and sends the latest on every Heartbeat.

// resets is a window's reset time, in Unix seconds as claude gives it,
// hours after the harness's midday start.
func resets(h *factorytest.Harness, hours int) int64 {
	return h.Clock.Now().Add(time.Duration(hours) * time.Hour).Unix()
}

// planLimits is the Project's Plan Limits, read as the human after the
// next poll's Heartbeat.
func planLimits(t *testing.T, h *factorytest.Harness, r *factorytest.Running) *api.PlanLimits {
	t.Helper()
	r.Poll()
	return wantFactory(t, h, api.FactoryOnline, "turns running").PlanLimits
}

// wantWindow fails the test unless w is used percent and resets at
// resetsAt, in Unix seconds.
func wantWindow(t *testing.T, name string, w *api.PlanLimitWindow, percent float64, resetsAt int64) {
	t.Helper()
	if w == nil {
		t.Fatalf("%s window missing, want %g%% resetting at %v", name, percent, time.Unix(resetsAt, 0).UTC())
	}
	if w.UsedPercent != percent || !w.ResetsAt.Equal(time.Unix(resetsAt, 0)) {
		t.Errorf("%s window %g%% resetting at %v, want %g%% resetting at %v", name, w.UsedPercent, w.ResetsAt, percent, time.Unix(resetsAt, 0).UTC())
	}
}

func TestAFactorySendsNoPlanLimitsBeforeItsFirstTurn(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	r := h.Start()
	if pl := planLimits(t, h, r); pl != nil {
		t.Errorf("Plan Limits %+v before any Turn, want none", pl)
	}
}

func TestATurnsRateLimitEventReachesTheNextHeartbeat(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("Spends the limit")
	fiveHour, weekly := resets(h, 3), resets(h, 100)
	h.Script(id, factorytest.RateLimitEvent(map[string]factorytest.RateLimitWindow{
		"five_hour":      {Utilization: 0.42, ResetsAt: fiveHour},
		"seven_day":      {Utilization: 0.07, ResetsAt: weekly},
		"seven_day_opus": {Utilization: 0.9, ResetsAt: resets(h, 50)},
	}), factorytest.Finish("Done."), factorytest.Result("success"))
	r := h.Start()
	h.WantState(id, domain.InReview)

	pl := planLimits(t, h, r)
	if pl == nil {
		t.Fatal("no Plan Limits after a Turn's rate_limit_event")
	}
	wantWindow(t, "5-hour", pl.FiveHour, 42, fiveHour)
	wantWindow(t, "weekly", pl.Weekly, 7, weekly)
	if pl.ObservedAt.IsZero() {
		t.Error("Plan Limits with no observed_at")
	}
}

func TestADrainingHeartbeatCarriesTheRunningTurnsPlanLimits(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("Still running")
	fiveHour := resets(h, 3)
	h.Script(id, factorytest.RateLimitEvent(map[string]factorytest.RateLimitWindow{
		"five_hour": {Utilization: 0.25, ResetsAt: fiveHour},
	}), factorytest.Wait(), factorytest.Finish("Done."), factorytest.Result("success"))
	r := h.Start()
	r.PollUntil("the running Turn's Plan Limits", func() bool { return factories(t, h).PlanLimits != nil })

	// The draining Heartbeat replaces the online one whole, so the
	// figures stay only if it carries them too.
	r.Interrupt()
	r.Settle()
	pl := wantFactory(t, h, api.FactoryDraining, "draining").PlanLimits
	if pl == nil {
		t.Fatal("no Plan Limits on the draining Heartbeat")
	}
	wantWindow(t, "5-hour", pl.FiveHour, 25, fiveHour)
	h.Continue(id)
	r.Exited()
}

func TestAMissingWindowIsntReported(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("Opus only")
	fiveHour := resets(h, 2)
	h.Script(id, factorytest.RateLimitEvent(map[string]factorytest.RateLimitWindow{
		"five_hour":      {Utilization: 0.5, ResetsAt: fiveHour},
		"seven_day_opus": {Utilization: 0.9, ResetsAt: resets(h, 50)},
	}), factorytest.Finish("Done."), factorytest.Result("success"))
	r := h.Start()
	h.WantState(id, domain.InReview)

	pl := planLimits(t, h, r)
	if pl == nil {
		t.Fatal("no Plan Limits after a Turn's rate_limit_event")
	}
	wantWindow(t, "5-hour", pl.FiveHour, 50, fiveHour)
	if pl.Weekly != nil {
		t.Errorf("weekly window %+v from seven_day_opus, want none", pl.Weekly)
	}
}

func TestTheLatestRateLimitEventWinsAndStaysAfterTheTurn(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	first := h.ReadyTicket("First")
	h.Script(first, factorytest.RateLimitEvent(map[string]factorytest.RateLimitWindow{
		"five_hour": {Utilization: 0.1, ResetsAt: resets(h, 4)},
		"seven_day": {Utilization: 0.2, ResetsAt: resets(h, 100)},
	}), factorytest.Finish("Done."), factorytest.Result("success"))
	r := h.Start()
	h.WantState(first, domain.InReview)

	// An event with neither window changes nothing; the next replaces
	// the figures whole.
	second := h.ReadyTicket("Second")
	fiveHour := resets(h, 3)
	h.Script(second,
		factorytest.RateLimitEvent(map[string]factorytest.RateLimitWindow{"seven_day_opus": {Utilization: 0.9, ResetsAt: resets(h, 50)}}),
		factorytest.RateLimitEvent(map[string]factorytest.RateLimitWindow{"five_hour": {Utilization: 0.3, ResetsAt: fiveHour}}),
		factorytest.RateLimitEvent(map[string]factorytest.RateLimitWindow{}),
		factorytest.Finish("Done."), factorytest.Result("success"))
	r.Poll()
	h.WantState(second, domain.InReview)

	for range 3 {
		pl := planLimits(t, h, r)
		if pl == nil {
			t.Fatal("no Plan Limits after the Turns ended")
		}
		wantWindow(t, "5-hour", pl.FiveHour, 30, fiveHour)
		if pl.Weekly != nil {
			t.Errorf("weekly window %+v, want the latest event's, none", pl.Weekly)
		}
		r.Tick()
	}
}

func TestAChatTurnsRateLimitEventReachesTheNextHeartbeat(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	weekly := resets(h, 100)
	h.Script(factorytest.Chat, factorytest.RateLimitEvent(map[string]factorytest.RateLimitWindow{
		"seven_day": {Utilization: 0.61, ResetsAt: weekly},
	}), factorytest.Say(factorytest.ChatReply), factorytest.Result("success"))
	r := h.Start()
	h.SendChat("How much is left?")
	h.WaitForReply()

	pl := planLimits(t, h, r)
	if pl == nil {
		t.Fatal("no Plan Limits after a Chat Turn's rate_limit_event")
	}
	wantWindow(t, "weekly", pl.Weekly, 61, weekly)
	if pl.FiveHour != nil {
		t.Errorf("5-hour window %+v, want none", pl.FiveHour)
	}
}

func TestUtilizationAboveOneIsSentAsAHundredPercent(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("Over the limit")
	fiveHour := resets(h, 1)
	h.Script(id, factorytest.RateLimitEvent(map[string]factorytest.RateLimitWindow{
		"five_hour": {Utilization: 1.3, ResetsAt: fiveHour},
	}), factorytest.Finish("Done."), factorytest.Result("success"))
	r := h.Start()
	h.WantState(id, domain.InReview)

	pl := planLimits(t, h, r)
	if pl == nil {
		t.Fatal("no Plan Limits after a Turn's rate_limit_event")
	}
	wantWindow(t, "5-hour", pl.FiveHour, 100, fiveHour)
}
