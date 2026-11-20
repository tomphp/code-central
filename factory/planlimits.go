// Plan Limits (Spec code-central-861, ADR 0051). The Factory reads the
// Claude account's 5-hour and weekly limits from each Turn's stream-json
// rate_limit_event, Claim and Chat Turns alike, keeps the latest, and
// sends it on every Heartbeat from then on.

package factory

import (
	"encoding/json"
	"math"
	"sync"
	"time"

	"github.com/tomoram/code-central/api"
)

// rateLimitInfo is the part of a rate_limit_event's rate_limit_info the
// Factory reads: the five_hour and seven_day windows of its
// unifiedWindows. Model-specific windows, status and overage are left.
type rateLimitInfo struct {
	UnifiedWindows struct {
		FiveHour *rateLimitWindow `json:"five_hour"`
		SevenDay *rateLimitWindow `json:"seven_day"`
	} `json:"unifiedWindows"`
}

// rateLimitWindow is one window: Utilization the share used, from 0 to
// 1, and ResetsAt when it resets, in Unix seconds. A window missing
// either isn't read.
type rateLimitWindow struct {
	Utilization *float64 `json:"utilization"`
	ResetsAt    *float64 `json:"resetsAt"`
}

// readPlanLimits gives msg's Plan Limits, observed at now, if msg is a
// rate_limit_event with a five_hour or seven_day window.
func readPlanLimits(msg streamMessage, now time.Time) (api.PlanLimits, bool) {
	var info rateLimitInfo
	if msg.Type != "rate_limit_event" || json.Unmarshal(msg.RateLimitInfo, &info) != nil {
		return api.PlanLimits{}, false
	}
	windows := info.UnifiedWindows
	pl := api.PlanLimits{ObservedAt: now, FiveHour: windows.FiveHour.planLimitWindow(), Weekly: windows.SevenDay.planLimitWindow()}
	return pl, pl.FiveHour != nil || pl.Weekly != nil
}

// planLimitWindow is w as a Plan Limits window, or nil when w is missing
// or lacks either figure.
func (w *rateLimitWindow) planLimitWindow() *api.PlanLimitWindow {
	if w == nil || w.Utilization == nil || w.ResetsAt == nil {
		return nil
	}
	return &api.PlanLimitWindow{UsedPercent: usedPercent(*w.Utilization), ResetsAt: time.Unix(int64(*w.ResetsAt), 0).UTC()}
}

// usedPercent is utilization as a percentage to a hundredth, clamped to
// 0–100 so a figure claude reports above 1 never gets the Heartbeat
// refused.
func usedPercent(utilization float64) float64 {
	return min(max(math.Round(utilization*10000)/100, 0), 100)
}

// latestPlanLimits holds the Factory's latest Plan Limits, set by Turns
// running in parallel and read by each Heartbeat. Each set replaces them
// whole; nothing clears them while the Factory runs.
type latestPlanLimits struct {
	mu     sync.Mutex
	limits *api.PlanLimits
}

func (l *latestPlanLimits) set(pl api.PlanLimits) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.limits = &pl
}

// get is the latest Plan Limits, or nil before the first.
func (l *latestPlanLimits) get() *api.PlanLimits {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.limits
}
