package api

// TriageAction is a triage action the human may take on an Incoming Issue
// (Spec code-central-279), named for its operation.
type TriageAction string

// The triage actions.
const (
	OptionKeepAsDraft   TriageAction = "keep_as_draft"
	OptionToRequirement TriageAction = "to_requirement"
	OptionWontFix       TriageAction = "wont_fix"
)

// TriageActions are the triage actions in the order they are offered.
func TriageActions() []TriageAction {
	return []TriageAction{OptionKeepAsDraft, OptionToRequirement, OptionWontFix}
}

// ToRequirementInput is the human's to_requirement(ticket, context?).
type ToRequirementInput struct {
	Ticket  Ref    `json:"ticket" path:"n"`
	Context string `json:"context,omitempty"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// WontFixInput is the human's wont_fix(ticket, reason?).
type WontFixInput struct {
	Ticket Ref    `json:"ticket" path:"n"`
	Reason string `json:"reason,omitempty"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}
