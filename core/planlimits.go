package core

import (
	"strings"
	"time"

	"github.com/tomoram/code-central/api"
)

// Plan Limits (Spec code-central-861): the Claude account's 5-hour and
// weekly limits, as a Factory last read them from its Turns and reports
// them on each Heartbeat. Code Central keeps them with the Heartbeat, in
// memory only, and the Project's Factory Status shows the most recently
// observed among its Factories that aren't Offline.

// checkPlanLimits checks the Plan Limits a Heartbeat reports and returns
// a copy of them, or nil when it reports none: no plan_limits, or neither
// window.
func checkPlanLimits(in *api.PlanLimits) (*api.PlanLimits, error) {
	if in == nil || in.FiveHour == nil && in.Weekly == nil {
		return nil, nil
	}
	if in.ObservedAt.IsZero() {
		return nil, invalid("plan_limits.observed_at", "plan_limits.observed_at is required: when the Factory read the figures, as an RFC 3339 time")
	}
	fiveHour, err := checkPlanLimitWindow("plan_limits.five_hour", in.FiveHour)
	if err != nil {
		return nil, err
	}
	weekly, err := checkPlanLimitWindow("plan_limits.weekly", in.Weekly)
	if err != nil {
		return nil, err
	}
	return &api.PlanLimits{ObservedAt: in.ObservedAt.UTC(), FiveHour: fiveHour, Weekly: weekly}, nil
}

// checkPlanLimitWindow checks the window at field and returns a copy of
// it, or nil when it is left out.
func checkPlanLimitWindow(field string, in *api.PlanLimitWindow) (*api.PlanLimitWindow, error) {
	if in == nil {
		return nil, nil
	}
	// Written this way round, the check refuses NaN too.
	if p := in.UsedPercent; !(p >= 0 && p <= 100) {
		return nil, invalid(field+".used_percent", "%s.used_percent is from 0 to 100, not %g", field, p)
	}
	if in.ResetsAt.IsZero() {
		return nil, invalid(field+".resets_at", "%s.resets_at is required: when the window resets, as an RFC 3339 time", field)
	}
	return &api.PlanLimitWindow{UsedPercent: in.UsedPercent, ResetsAt: in.ResetsAt.UTC()}, nil
}

// keptLimits are the Plan Limits to keep for a Heartbeat reporting
// reported at now, after prev: reported with observed_at capped at now.
// Figures the Factory sent before keep their first capped time, so a
// Factory whose clock runs ahead doesn't look newer on every Heartbeat.
func keptLimits(prev heartbeat, reported *api.PlanLimits, now time.Time) *api.PlanLimits {
	if reported == nil {
		return nil
	}
	if prev.reported != nil && sameLimits(prev.reported, reported) {
		return prev.limits
	}
	kept := *reported
	if kept.ObservedAt.After(now) {
		kept.ObservedAt = now
	}
	return &kept
}

// sameLimits says whether a and b are the same Plan Limits, or both none.
func sameLimits(a, b *api.PlanLimits) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.ObservedAt.Equal(b.ObservedAt) && sameWindow(a.FiveHour, b.FiveHour) && sameWindow(a.Weekly, b.Weekly)
}

// sameWindow says whether a and b are the same window, or both left out.
func sameWindow(a, b *api.PlanLimitWindow) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.UsedPercent == b.UsedPercent && a.ResetsAt.Equal(b.ResetsAt)
}

// newerLimits says whether a, from the Factory ak, wins over b, from bk:
// observed later, or at the same time from the Factory first by name and
// host, so the choice doesn't hang on map order.
func newerLimits(a api.PlanLimits, ak heartbeatKey, b api.PlanLimits, bk heartbeatKey) bool {
	if !a.ObservedAt.Equal(b.ObservedAt) {
		return a.ObservedAt.After(b.ObservedAt)
	}
	if c := strings.Compare(ak.name, bk.name); c != 0 {
		return c < 0
	}
	return ak.host < bk.host
}
