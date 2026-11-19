package api

// ChoiceSource says which tier set a Model Choice value.
type ChoiceSource string

// ChoiceSources are the sources.
func ChoiceSources() []ChoiceSource {
	return []ChoiceSource{SourceTicketTag, SourceProjectKind, SourceProjectDefault, SourceNone}
}

// ChoiceValue is one of a Model Choice's values with its source; Value is
// empty when the source is none. Kind names the kind of work whose
// Models row set it, only when the source is project_kind.
type ChoiceValue struct {
	Value  string       `json:"value,omitempty"`
	Source ChoiceSource `json:"source"`
	Kind   string       `json:"kind,omitempty"`
}

// ModelChoice is the model and effort a Ticket's next turn runs with, as
// far as Code Central sets them, each worked out on its own.
type ModelChoice struct {
	Model  ChoiceValue `json:"model"`
	Effort ChoiceValue `json:"effort"`
}

// GetModelChoiceInput is the Factory's get_model_choice(ticket).
type GetModelChoiceInput struct {
	Ticket Ref `json:"ticket" path:"n"`
}

// ReportModelChoiceInput is the Factory's report_model_choice(ticket,
// claim_id, model, effort, full?): what a turn was started with, after
// the Factory's fallbacks, empty meaning Claude Code's default.
type ReportModelChoiceInput struct {
	Ticket  Ref    `json:"ticket" path:"n"`
	ClaimID string `json:"claim_id"`
	Model   string `json:"model,omitempty"`
	Effort  string `json:"effort,omitempty"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// The sources Code Central works out. SourceNone leaves the value to the
// Factory: its flag, or else Claude Code's default.
const (
	SourceTicketTag      ChoiceSource = "ticket_tag"
	SourceProjectKind    ChoiceSource = "project_kind"
	SourceProjectDefault ChoiceSource = "project_default"
	SourceNone           ChoiceSource = "none"
)
