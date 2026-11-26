// Plan Limits (Spec code-central-861, ADR 0051). The Factory keeps the
// latest Plan Limits its Harness reads from a Turn, Claim and Chat Turns
// alike, and sends it on every Heartbeat from then on.

package factory

import (
	"sync"

	"github.com/tomoram/code-central/api"
)

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
