package api

// The parts of a Ticket a read can ask for, with PartEdits.
const (
	PartClaims         = "claims"
	PartFailureReports = "failure_reports"
	PartComments       = "comments"
	PartRounds         = "rounds"
	PartMoves          = "moves"
)

// TicketParts is every part of a Ticket: a read asking for them all is
// the whole Ticket.
func TicketParts() []string {
	return []string{PartClaims, PartFailureReports, PartComments, PartRounds, PartMoves, PartEdits}
}

// DefaultTicketParts is what get_ticket and claim return of a Ticket when
// no parts are named: how each Claim ended, every Failure Report and every
// comment, which carry the rejection, give-up and send-back notes a redo
// needs. Rounds, moves and edits are left out (ADR 0015), and the view is
// Brief.
func DefaultTicketParts() []string { return []string{PartClaims, PartFailureReports, PartComments} }
