package core

import (
	"maps"
	"slices"
	"strings"

	"github.com/tomoram/code-central/store"
)

// GetTicketInput is get_ticket's input. With no Parts, the read is the
// default: DefaultTicketParts, brief (ADR 0015, code-central-320).
type GetTicketInput struct {
	Ticket Ref `json:"ticket"`
	// Parts are any of TicketParts: the read returns the Ticket's other
	// fields and only these parts.
	Parts []string `json:"parts,omitempty"`
	// WithModelChoice adds the Ticket's Model Choice, for the UI's read
	// over HTTP. Agents' MCP get_ticket can't ask for it, to keep their
	// context small.
	WithModelChoice bool `json:"-"`
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
// needs. Rounds, moves and edits are left out (ADR 0015), and the view is
// Brief.
var DefaultTicketParts = []string{PartClaims, PartFailureReports, PartComments}

// parts is the set of parts the read returns.
func (in GetTicketInput) parts() (map[string]bool, error) {
	asked := in.Parts
	if len(asked) == 0 {
		asked = DefaultTicketParts
	}
	return partSet(asked, TicketParts)
}

// partSet is the parts asked for as a set, each checked against all the
// parts there are.
func partSet(asked, all []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, p := range asked {
		if !slices.Contains(all, p) {
			return nil, invalid("parts", "unknown part %q (the parts are %s)", p, strings.Join(all, ", "))
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
		v.countLeftOut(part, n)
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
	parts := map[string]bool{}
	for _, p := range DefaultTicketParts {
		parts[p] = true
	}
	return v.only(parts).Brief()
}

// Brief is the view without what an agent working the Ticket doesn't use
// (code-central-320): each Done Blocker is its ID and State only, and each
// Claim leaves out its working dir, resume command and latest Session
// Event, which are for the human and the UI. LeftOut counts the Claims
// that had any under claims, which parts: [claims] returns in full.
func (v TicketView) Brief() TicketView {
	if v.Blockers != nil {
		v.Blockers = slices.Clone(v.Blockers)
		for i := range v.Blockers {
			v.Blockers[i].brief = v.Blockers[i].State == store.Done
		}
	}
	if v.Claims != nil {
		v.Claims = slices.Clone(v.Claims)
		n := 0
		for i := range v.Claims {
			c := &v.Claims[i]
			if c.WorkingDir != "" || c.ResumeCommand != "" || c.LatestEvent != nil {
				n++
			}
			c.WorkingDir, c.ResumeCommand, c.LatestEvent = "", "", nil
		}
		v.countLeftOut(PartClaims, n)
	}
	return v
}

// countLeftOut counts n of part in LeftOut, when there are any; it never
// writes to the map v was given.
func (v *TicketView) countLeftOut(part string, n int) {
	if n == 0 {
		return
	}
	v.LeftOut = maps.Clone(v.LeftOut)
	if v.LeftOut == nil {
		v.LeftOut = map[string]int{}
	}
	v.LeftOut[part] = n
}
