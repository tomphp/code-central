package api

import (
	"time"
)

// HeartbeatState is the state a Factory reports in a Heartbeat.
type HeartbeatState string

// FactoryStatusName is a Factory's, or a Project's combined, Factory Status.
type FactoryStatusName string

// FactoryStatusNames are the Factory Statuses.
func FactoryStatusNames() []FactoryStatusName {
	return []FactoryStatusName{FactoryOnline, FactoryDraining, FactoryOffline}
}

// FactoryStatusView is a Project's Factory Status as every Project read
// returns it, always filled in.
type FactoryStatusView struct {
	// Status is the combined status: Online if any Factory is, else
	// Draining if any is, else Offline (also with no Factories).
	Status    FactoryStatusName `json:"status"`
	Factories []FactoryView     `json:"factories"`
	// Base is the base the Factory last reported under Pull Request
	// (code-central-394); left out while none is reporting.
	Base *BaseView `json:"base,omitempty"`
	// PlanLimits are the most recently observed Plan Limits among the
	// Factories that aren't Offline (Spec code-central-861); left out
	// while none has any.
	PlanLimits *PlanLimits `json:"plan_limits,omitempty"`
}

// FactoryView is one Factory as the Project view lists it, from its last
// Heartbeat.
type FactoryView struct {
	Name   string            `json:"name"`
	Host   string            `json:"host"`
	Status FactoryStatusName `json:"status"`
	// Note is empty when none; a stopped Factory keeps its last one.
	Note            string    `json:"note"`
	LastHeartbeat   time.Time `json:"last_heartbeat"`
	IntervalSeconds int       `json:"interval_seconds"`
	// Chat is Chat support: the Factory can run Conversations. Chat is
	// available for the Project while an Online Factory has it.
	Chat bool `json:"chat"`
	// ReportsTo is the host of the Factory's Reporting Service, the
	// service its Heartbeats reach, when that is another service sharing
	// the Store; empty when it is this one.
	ReportsTo string `json:"reports_to,omitempty"`
}

// ReportHeartbeatInput is the Factory's report_heartbeat(factory, host,
// state, note?, interval_seconds, full?).
type ReportHeartbeatInput struct {
	Factory string         `json:"factory"`
	Host    string         `json:"host"`
	State   HeartbeatState `json:"state"`
	// Note is free text, cut to 200 characters; Code Central never
	// interprets it.
	Note string `json:"note"`
	// IntervalSeconds is the Factory's poll interval, 1 to 86400.
	IntervalSeconds int `json:"interval_seconds"`
	// Chat is Chat support: the Factory can run Conversations. It
	// defaults to false, so a Factory that doesn't know Chat has none.
	Chat bool `json:"chat"`
	// PlanLimits are the Plan Limits the Factory last read from its
	// Turns; left out when it has none. Plan Limits with neither window
	// are none.
	PlanLimits *PlanLimits `json:"plan_limits,omitempty"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

const (
	HeartbeatOnline   HeartbeatState = "online"
	HeartbeatDraining HeartbeatState = "draining"
	HeartbeatStopping HeartbeatState = "stopping"
)

const (
	FactoryOnline   FactoryStatusName = "online"
	FactoryDraining FactoryStatusName = "draining"
	FactoryOffline  FactoryStatusName = "offline"
)
