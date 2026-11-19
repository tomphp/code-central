package api

// The parts of a Spec a lighter get_spec can ask for. A Ticket has edits
// too, so PartEdits names its edits part as well.
const (
	PartBody    = "body"
	PartTickets = "tickets"
	PartEdits   = "edits"
	// PartOutline reads the outline alone: every read but the full one
	// returns it, so naming it asks for nothing else.
	PartOutline = "outline"
)
