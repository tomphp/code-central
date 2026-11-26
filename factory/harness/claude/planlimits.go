package claude

import (
	"encoding/json"
	"math"
	"time"

	"github.com/tomoram/code-central/api"
)

// Plan Limits (Spec code-central-861, ADR 0051) are the Claude account's
// 5-hour and weekly limits, read from each Turn's rate_limit_event.

// rateLimitInfo is the part of a rate_limit_event's rate_limit_info the
// Harness reads: the five_hour and seven_day windows of its
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
