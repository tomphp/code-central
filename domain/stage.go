package domain

// StagePrefix is the reserved Label prefix that marks a Stage Ticket.
const StagePrefix = "stage:"

// StageName is a Stage's name, its Label's suffix.
type StageName string

// The Stages' names.
const (
	StageGrilling StageName = "grilling"
	StageSpec     StageName = "spec"
	StageTickets  StageName = "tickets"
)

// Stages are the three Stages in order.
func Stages() []StageName { return []StageName{StageGrilling, StageSpec, StageTickets} }
