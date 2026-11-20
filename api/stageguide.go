package api

// StageGuidesView is a Project's Stage Guides settings as every Project
// read returns them.
type StageGuidesView struct {
	// SubagentExploration is whether the implementation guide tells the
	// agent to send reading across several files to a subagent.
	SubagentExploration bool `json:"subagent_exploration"`
	// ModelAssignment is whether the tickets Stage sets each Ticket's
	// model and effort.
	ModelAssignment bool `json:"model_assignment"`
}

// StageGuideInput is get_stage_guide's input.
type StageGuideInput struct {
	Stage string `json:"stage" path:"stage" desc:"The guide's name: the Ticket's Stage Label without stage:, map-<type> for a Wayfinder Label wayfinder:<type>, prototype, retro, or implementation when it has none of these." required:"true"`
}

// StageGuideView is one Stage Guide: get_stage_guide's result, and the
// guide in claim's.
type StageGuideView struct {
	Stage string `json:"stage"`
	Guide string `json:"guide"`
}

// SetSubagentExplorationInput is the human's
// set_subagent_exploration(on, full?).
type SetSubagentExplorationInput struct {
	On bool `json:"on"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// SetModelAssignmentInput is the human's set_model_assignment(on, full?).
type SetModelAssignmentInput struct {
	On bool `json:"on"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}
