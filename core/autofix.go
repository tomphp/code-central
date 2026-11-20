package core

import (
	"context"
	"strings"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

// Auto-fix (Spec code-central-1022, ADR 0066): the Project's setting that
// opens fix Tickets for CI failed items on the human's behalf. This file
// holds the setting, which only the human sets, over HTTP.

func autoFixView(a domain.AutoFix) api.AutoFixView {
	return api.AutoFixView{On: a.On, FixInstructions: a.FixInstructions(), DefaultFixInstructions: domain.DefaultFixInstructions, Limit: a.AttemptLimit()}
}

// SetAutoFix is the human's set_auto_fix: Auto-fix on or off, with its
// Fix instructions and limit. It refuses a limit outside 1 to 10, empty
// Fix instructions, and turning Auto-fix on while CI Tracking is off.
// Turning CI Tracking off later leaves the entry as it is.
func (c *Core) SetAutoFix(ctx context.Context, key string, in api.SetAutoFixInput) (api.Written[api.ProjectView], error) {
	if err := humanOnly(ctx, "set_auto_fix"); err != nil {
		return api.Written[api.ProjectView]{}, err
	}
	if in.Limit < domain.MinAutoFixLimit || in.Limit > domain.MaxAutoFixLimit {
		return api.Written[api.ProjectView]{}, invalid("limit", "the Auto-fix limit is %d to %d, not %d", domain.MinAutoFixLimit, domain.MaxAutoFixLimit, in.Limit)
	}
	if strings.TrimSpace(in.FixInstructions) == "" {
		return api.Written[api.ProjectView]{}, invalid("fix_instructions", "Fix instructions can't be empty; the default text is %q", domain.DefaultFixInstructions)
	}
	v, err := c.updateCIIn(key, in.Full, func(p domain.Project, ci *domain.CI) error {
		if in.On && !ciTrackingOn(*ci) {
			return invalid("on", "Auto-fix needs CI Tracking on; turn CI Tracking on first")
		}
		instructions := in.FixInstructions
		if instructions == domain.DefaultFixInstructions {
			instructions = ""
		}
		ci.AutoFix = domain.AutoFix{On: in.On, Instructions: instructions, Limit: in.Limit}
		return nil
	})
	if err != nil {
		return v, err
	}
	// Level-triggered: turning it on acts on the items already waiting.
	if err := c.sweepAutoFix(ctx, key); err != nil {
		return api.Written[api.ProjectView]{}, err
	}
	return v, nil
}
