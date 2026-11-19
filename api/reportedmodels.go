package api

import (
	"time"
)

// ReportedModelsView is a Project's Reported Models as every Project read
// returns them: what its Factory last reported, plus the model IDs its
// sessions' usage has shown.
type ReportedModelsView struct {
	// EffortLevels are the valid effort levels: the reported ones, or the
	// seeded ones until a report has given some.
	EffortLevels []string `json:"effort_levels"`
	Aliases      []string `json:"aliases"`
	ModelIDs     []string `json:"model_ids"`
	// ModelFlag and EffortFlag are the Factory's --model and --effort,
	// empty when it reported them unset or hasn't reported.
	ModelFlag  string `json:"model_flag"`
	EffortFlag string `json:"effort_flag"`
	// Reported is when the Factory last reported, left out before any
	// report.
	Reported *time.Time `json:"reported,omitempty"`
}

// ReportModelsInput is the Factory's report_models(effort_levels,
// aliases, model_flag, effort_flag, full?).
type ReportModelsInput struct {
	// EffortLevels and Aliases are what the Factory read from claude
	// --help; left empty, the kept ones stay.
	EffortLevels []string `json:"effort_levels"`
	Aliases      []string `json:"aliases"`
	// ModelFlag and EffortFlag are the Factory's own flags, empty when
	// unset; they always replace the kept ones.
	ModelFlag  string `json:"model_flag"`
	EffortFlag string `json:"effort_flag"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}
