package core

import (
	"slices"
	"strings"
)

// GetTicketInput is get_ticket's input. With no Parts, the read is the
// default: DefaultTicketParts (ADR 0015).
type GetTicketInput struct {
	Ticket Ref `json:"ticket"`
	// Parts are any of TicketParts: the read returns the Ticket's other
	// fields and only these parts.
	Parts []string `json:"parts,omitempty"`
}

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
var TicketParts = []string{PartClaims, PartFailureReports, PartComments, PartRounds, PartMoves, PartEdits}

// DefaultTicketParts is what get_ticket and claim return of a Ticket when
// no parts are named: how each Claim ended, every Failure Report and every
// comment, which carry the rejection, give-up and send-back notes a redo
// needs. Rounds, moves and edits are left out (ADR 0015).
var DefaultTicketParts = []string{PartClaims, PartFailureReports, PartComments}

// parts is the set of parts the read returns.
func (in GetTicketInput) parts() (map[string]bool, error) {
	asked := in.Parts
	if len(asked) == 0 {
		asked = DefaultTicketParts
	}
	out := map[string]bool{}
	for _, p := range asked {
		if !slices.Contains(TicketParts, p) {
			return nil, invalid("parts", "unknown part %q (the parts are %s)", p, strings.Join(TicketParts, ", "))
		}
		out[p] = true
	}
	return out, nil
}

// only is the view with just the parts named: each other part is left out
// (nil, so it has no key) and counted in LeftOut when it has anything.
func (v TicketView) only(parts map[string]bool) TicketView {
	leave := func(part string, n int) bool {
		if parts[part] {
			return false
		}
		if n > 0 {
			if v.LeftOut == nil {
				v.LeftOut = map[string]int{}
			}
			v.LeftOut[part] = n
		}
		return true
	}
	if leave(PartClaims, len(v.Claims)) {
		v.Claims = nil
	}
	if leave(PartFailureReports, len(v.FailureReports)) {
		v.FailureReports = nil
	}
	if leave(PartComments, len(v.Comments)) {
		v.Comments = nil
	}
	if leave(PartRounds, len(v.Rounds)) {
		v.Rounds = nil
	}
	if leave(PartMoves, len(v.Moves)) {
		v.Moves = nil
	}
	if leave(PartEdits, len(v.Edits)) {
		v.Edits = nil
	}
	return v
}

// defaultView is the Ticket as a read naming no parts returns it.
func defaultView(v TicketView) TicketView {
	parts, _ := GetTicketInput{}.parts()
	return v.only(parts)
}
