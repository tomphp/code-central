package core

import (
	"strings"
	"time"
)

// Plan Limits (Spec code-central-861): the Claude account's 5-hour and
// weekly limits, as a Factory last read them from its Turns and reports
// them on each Heartbeat. Code Central keeps them with the Heartbeat, in
// memory only, and the Project's Factory Status shows the most recently
// observed among its Factories that aren't Offline.

// PlanLimits are a Factory's Plan Limits, as it reports them and as the
// Project's Factory Status shows them.
type PlanLimits struct {
	// ObservedAt is when the Factory read the figures; one later than
	// Code Central's receipt is kept as the receipt time.
	ObservedAt time.Time `json:"observed_at"`
	// FiveHour and Weekly are each left out when the Factory has no
	// figures for that window.
	FiveHour *PlanLimitWindow `json:"five_hour,omitempty"`
	Weekly   *PlanLimitWindow `json:"weekly,omitempty"`
}

// PlanLimitWindow is one window of the Plan Limits.
type PlanLimitWindow struct {
	// UsedPercent is the share of the window's limit used, 0 to 100.
	UsedPercent float64   `json:"used_percent"`
	ResetsAt    time.Time `json:"resets_at"`
}

// checkPlanLimits checks the Plan Limits a Heartbeat reports and returns
// a copy of them, or nil when it reports none: no plan_limits, or neither
// window.
func checkPlanLimits(in *PlanLimits) (*PlanLimits, error) {
	if in == nil || in.FiveHour == nil && in.Weekly == nil {
		return nil, nil
	}
	if in.ObservedAt.IsZero() {
		return nil, invalid("plan_limits.observed_at", "plan_limits.observed_at is required: when the Factory read the figures, as an RFC 3339 time")
	}
	out := &PlanLimits{ObservedAt: in.ObservedAt.UTC()}
	for _, w := range []struct {
		name string
		in   *PlanLimitWindow
		out  **PlanLimitWindow
	}{{"five_hour", in.FiveHour, &out.FiveHour}, {"weekly", in.Weekly, &out.Weekly}} {
		if w.in == nil {
			continue
		}
		field := "plan_limits." + w.name
		if p := w.in.UsedPercent; !(p >= 0 && p <= 100) {
			return nil, invalid(field+".used_percent", "%s.used_percent is from 0 to 100, not %g", field, p)
		}
		if w.in.ResetsAt.IsZero() {
			return nil, invalid(field+".resets_at", "%s.resets_at is required: when the window resets, as an RFC 3339 time", field)
		}
		*w.out = &PlanLimitWindow{UsedPercent: w.in.UsedPercent, ResetsAt: w.in.ResetsAt.UTC()}
	}
	return out, nil
}

// keptLimits are the Plan Limits to keep for a Heartbeat reporting
// reported at now, after prev: reported with observed_at capped at now.
// Figures the Factory sent before keep their first capped time, so a
// Factory whose clock runs ahead doesn't look newer on every Heartbeat.
func keptLimits(prev heartbeat, reported *PlanLimits, now time.Time) *PlanLimits {
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
func sameLimits(a, b *PlanLimits) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.ObservedAt.Equal(b.ObservedAt) && sameWindow(a.FiveHour, b.FiveHour) && sameWindow(a.Weekly, b.Weekly)
}

func sameWindow(a, b *PlanLimitWindow) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.UsedPercent == b.UsedPercent && a.ResetsAt.Equal(b.ResetsAt)
}

// newerLimits says whether a, from the Factory ak, wins over b, from bk:
// observed later, or at the same time from the Factory first by name and
// host, so the choice doesn't hang on map order.
func newerLimits(a PlanLimits, ak heartbeatKey, b PlanLimits, bk heartbeatKey) bool {
	if !a.ObservedAt.Equal(b.ObservedAt) {
		return a.ObservedAt.After(b.ObservedAt)
	}
	if c := strings.Compare(ak.name, bk.name); c != 0 {
		return c < 0
	}
	return ak.host < bk.host
}
