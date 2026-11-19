package api

import (
	"time"
)

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
