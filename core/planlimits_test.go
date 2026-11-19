package core_test

import (
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/events"
)

// limitsBeat is an Online Heartbeat from factory on box with limits.
func limitsBeat(factory string, limits *api.PlanLimits) api.ReportHeartbeatInput {
	return api.ReportHeartbeatInput{Factory: factory, Host: "box", State: api.HeartbeatOnline, IntervalSeconds: 10, PlanLimits: limits}
}

// sendBeat sends in to acme.
func sendBeat(f fixture, t *testing.T, in api.ReportHeartbeatInput) {
	t.Helper()
	if _, err := f.core.ReportHeartbeat(ctx, "acme", in); err != nil {
		t.Fatal(err)
	}
}

// beatLimits sends limitsBeat(factory, limits).
func beatLimits(f fixture, t *testing.T, factory string, limits *api.PlanLimits) {
	t.Helper()
	sendBeat(f, t, limitsBeat(factory, limits))
}

// sameWindow says whether got is want.
func sameWindow(got, want *api.PlanLimitWindow) bool {
	return got != nil && got.UsedPercent == want.UsedPercent && got.ResetsAt.Equal(want.ResetsAt)
}

// limitsAt is Plan Limits observed at, with both windows at five and week
// percent used.
func limitsAt(at time.Time, five, week float64) *api.PlanLimits {
	return &api.PlanLimits{
		ObservedAt: at,
		FiveHour:   &api.PlanLimitWindow{UsedPercent: five, ResetsAt: at.Add(3 * time.Hour)},
		Weekly:     &api.PlanLimitWindow{UsedPercent: week, ResetsAt: at.Add(72 * time.Hour)},
	}
}

func TestAProjectHasNoPlanLimitsUntilAFactoryReportsThem(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	beat(f, t, "cc", "box", api.HeartbeatOnline, 10)
	if pl := factoryStatus(f, t).PlanLimits; pl != nil {
		t.Fatalf("plan_limits = %+v", pl)
	}
}

func TestAHeartbeatsPlanLimitsShowOnTheFactoryStatus(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	sent := limitsAt(t0.Add(-time.Minute), 42.5, 7)
	beatLimits(f, t, "cc", sent)
	got := factoryStatus(f, t).PlanLimits
	if got == nil || !got.ObservedAt.Equal(sent.ObservedAt) || !sameWindow(got.FiveHour, sent.FiveHour) || !sameWindow(got.Weekly, sent.Weekly) {
		t.Fatalf("plan_limits = %+v, want %+v", got, sent)
	}
}

func TestOneWindowAloneIsKeptAndNeitherIsNone(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	only := &api.PlanLimits{ObservedAt: t0, Weekly: &api.PlanLimitWindow{UsedPercent: 100, ResetsAt: t0.Add(time.Hour)}}
	beatLimits(f, t, "cc", only)
	if got := factoryStatus(f, t).PlanLimits; got == nil || got.FiveHour != nil || got.Weekly == nil || got.Weekly.UsedPercent != 100 {
		t.Fatalf("weekly only: %+v", got)
	}
	beatLimits(f, t, "cc", &api.PlanLimits{ObservedAt: t0})
	if got := factoryStatus(f, t).PlanLimits; got != nil {
		t.Fatalf("neither window: %+v", got)
	}
}

func TestPlanLimitsAreRefusedWithTheFieldAndItsFix(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	for field, mutate := range map[string]func(*api.PlanLimits){
		"plan_limits.observed_at":            func(pl *api.PlanLimits) { pl.ObservedAt = time.Time{} },
		"plan_limits.five_hour.used_percent": func(pl *api.PlanLimits) { pl.FiveHour.UsedPercent = 100.5 },
		"plan_limits.weekly.used_percent":    func(pl *api.PlanLimits) { pl.Weekly.UsedPercent = -1 },
		"plan_limits.five_hour.resets_at":    func(pl *api.PlanLimits) { pl.FiveHour.ResetsAt = time.Time{} },
		"plan_limits.weekly.resets_at":       func(pl *api.PlanLimits) { pl.Weekly.ResetsAt = time.Time{} },
	} {
		pl := limitsAt(t0, 10, 20)
		mutate(pl)
		_, err := f.core.ReportHeartbeat(ctx, "acme", limitsBeat("cc", pl))
		wantCode(t, err, api.Invalid, field)
	}
	if fs := factoryStatus(f, t); len(fs.Factories) != 0 {
		t.Fatalf("a refused Heartbeat was kept: %+v", fs)
	}
}

func TestAnObservationLaterThanReceiptIsCappedAtReceipt(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	beatLimits(f, t, "cc", limitsAt(t0.Add(time.Hour), 10, 20))
	if got := factoryStatus(f, t).PlanLimits; got == nil || !got.ObservedAt.Equal(t0) {
		t.Fatalf("plan_limits = %+v, want observed at %s", got, t0)
	}
	// The same skewed figures again keep their first receipt time.
	f.clock.Advance(10 * time.Second)
	beatLimits(f, t, "cc", limitsAt(t0.Add(time.Hour), 10, 20))
	if got := factoryStatus(f, t).PlanLimits; !got.ObservedAt.Equal(t0) {
		t.Fatalf("resent: observed at %s, want %s", got.ObservedAt, t0)
	}
}

func TestTheMostRecentlyObservedPlanLimitsAmongFactoriesWin(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	beatLimits(f, t, "a", limitsAt(t0.Add(-time.Minute), 30, 30))
	beatLimits(f, t, "b", limitsAt(t0.Add(-2*time.Minute), 60, 60))
	if got := factoryStatus(f, t).PlanLimits; got == nil || got.FiveHour.UsedPercent != 30 {
		t.Fatalf("plan_limits = %+v, want a's", got)
	}
	f.clock.Advance(5 * time.Second)
	beatLimits(f, t, "b", limitsAt(t0, 70, 70))
	if got := factoryStatus(f, t).PlanLimits; got == nil || got.FiveHour.UsedPercent != 70 {
		t.Fatalf("plan_limits = %+v, want b's newer ones", got)
	}
}

func TestAnOfflineFactorysPlanLimitsDropOutAtOnce(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	beatLimits(f, t, "a", limitsAt(t0, 30, 30))
	f.clock.Advance(60 * time.Second)
	beatLimits(f, t, "b", limitsAt(t0.Add(-time.Hour), 60, 60))
	f.clock.Advance(30 * time.Second) // a is Offline, b isn't
	if got := factoryStatus(f, t).PlanLimits; got == nil || got.FiveHour.UsedPercent != 60 {
		t.Fatalf("plan_limits = %+v, want b's", got)
	}
	stopping := limitsBeat("b", limitsAt(t0, 60, 60))
	stopping.State = api.HeartbeatStopping
	sendBeat(f, t, stopping)
	if got := factoryStatus(f, t).PlanLimits; got != nil {
		t.Fatalf("every Factory Offline: plan_limits = %+v", got)
	}
}

func TestAHeartbeatWithoutPlanLimitsClearsTheFactorys(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	beatLimits(f, t, "cc", limitsAt(t0, 30, 30))
	beatLimits(f, t, "cc", nil)
	if got := factoryStatus(f, t).PlanLimits; got != nil {
		t.Fatalf("plan_limits = %+v", got)
	}
}

func TestChangedPlanLimitsSendTheProjectsNoticeAndUnchangedOnesDont(t *testing.T) {
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
	beatLimits(f, t, "cc", nil)
	expect("new Factory", true)
	beatLimits(f, t, "cc", limitsAt(t0, 10, 20))
	expect("first figures", true)
	f.clock.Advance(10 * time.Second)
	beatLimits(f, t, "cc", limitsAt(t0, 10, 20))
	expect("same figures", false)
	beatLimits(f, t, "cc", limitsAt(t0, 11, 20))
	expect("new percent", true)
	beatLimits(f, t, "cc", limitsAt(t0.Add(time.Second), 11, 20))
	expect("newer observation", true)
	beatLimits(f, t, "cc", nil)
	expect("figures cleared", true)
	beatLimits(f, t, "cc", nil)
	expect("still none", false)
}

func TestTheWatchReportsPlanLimitsLapsing(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	tick := watchFactories(t, f)
	tick()
	// A long interval keeps the Factory Online while a second one lapses.
	slow := limitsBeat("slow", nil)
	slow.IntervalSeconds = 600
	sendBeat(f, t, slow)
	beatLimits(f, t, "fast", limitsAt(t0, 10, 20))
	tick()
	f.clock.Advance(90 * time.Second)
	if got := tick(); len(got) != 1 || got[0] != "acme" {
		t.Fatalf("lapse reported %v, want acme", got)
	}
	if pl := factoryStatus(f, t).PlanLimits; pl != nil {
		t.Fatalf("plan_limits = %+v", pl)
	}
}
