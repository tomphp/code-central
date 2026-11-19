package api

// FocusKindsView is a Project's Focus Kinds as every Project read returns
// them: the Needs Me reasons that are off, in group order, empty while
// every reason is on.
type FocusKindsView struct {
	Off []NeedsMeReason `json:"off"`
}

// SetFocusKindsInput is the human's set_focus_kinds(off, full?).
type SetFocusKindsInput struct {
	// Off is the whole list of Needs Me reasons to turn off; every other
	// reason is on.
	Off []NeedsMeReason `json:"off"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}
