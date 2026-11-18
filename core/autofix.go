package core

import (
	"context"
	"strings"

	"github.com/tomoram/code-central/domain"
)

// Auto-fix (Spec code-central-1022, ADR 0066): the Project's setting that
// opens fix Tickets for CI failed items on the human's behalf. This file
// holds the setting, which only the human sets, over HTTP.

// AutoFixView is a Project's Auto-fix setting as every Project read
// returns it: the defaults while the human never set them.
type AutoFixView struct {
	On bool `json:"on"`
	// FixInstructions are appended to every fix Ticket's description.
	FixInstructions string `json:"fix_instructions"`
	// DefaultFixInstructions is the text FixInstructions starts from, for
	// the UI's way back to it.
	DefaultFixInstructions string `json:"default_fix_instructions"`
	// Limit is the Auto-fix limit: the most Auto-fix attempts per failure
	// chain, 1 to 10.
	Limit int `json:"limit"`
}

func autoFixView(a domain.AutoFix) AutoFixView {
	return AutoFixView{On: a.On, FixInstructions: a.FixInstructions(), DefaultFixInstructions: domain.DefaultFixInstructions, Limit: a.AttemptLimit()}
}

// SetAutoFixInput is the human's set_auto_fix(on, fix_instructions,
// limit, full?).
type SetAutoFixInput struct {
	On bool `json:"on"`
	// FixInstructions is the text appended to every fix Ticket's
	// description; it may not be empty.
	FixInstructions string `json:"fix_instructions"`
	// Limit is the most Auto-fix attempts per failure chain, 1 to 10.
	Limit int `json:"limit"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// SetAutoFix is the human's set_auto_fix: Auto-fix on or off, with its
// Fix instructions and limit. It refuses a limit outside 1 to 10, empty
// Fix instructions, and turning Auto-fix on while CI Tracking is off.
// Turning CI Tracking off later leaves the entry as it is.
func (c *Core) SetAutoFix(ctx context.Context, key string, in SetAutoFixInput) (Written[ProjectView], error) {
	if err := humanOnly(ctx, "set_auto_fix"); err != nil {
		return Written[ProjectView]{}, err
	}
	if in.Limit < domain.MinAutoFixLimit || in.Limit > domain.MaxAutoFixLimit {
		return Written[ProjectView]{}, invalid("limit", "the Auto-fix limit is %d to %d, not %d", domain.MinAutoFixLimit, domain.MaxAutoFixLimit, in.Limit)
	}
	if strings.TrimSpace(in.FixInstructions) == "" {
		return Written[ProjectView]{}, invalid("fix_instructions", "Fix instructions can't be empty; the default text is %q", domain.DefaultFixInstructions)
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
		return Written[ProjectView]{}, err
	}
	return v, nil
}
