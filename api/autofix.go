package api

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
