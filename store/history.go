package store

import (
	"slices"

	"github.com/tomoram/code-central/domain"
)

// Part is one part of a Ticket History: a Ticket read loads only the
// parts it names, and a Ticket write writes only the parts it names. The
// names are ADR 0015's.
type Part string

// The Ticket History's parts.
const (
	Claims         Part = "claims"
	Rounds         Part = "rounds"
	Comments       Part = "comments"
	Moves          Part = "moves"
	Edits          Part = "edits"
	FailureReports Part = "failure_reports"
)

// History is every part of a Ticket History, for a read or write of the
// whole record.
var History = []Part{Claims, Rounds, Comments, Moves, Edits, FailureReports}

// withParts is t with the parts parts doesn't name taken from from: from's
// parts, or none, where t is a read, and the stored ones where t is
// written over them.
func withParts(t, from domain.Ticket, parts []Part) domain.Ticket {
	if !slices.Contains(parts, Claims) {
		t.Claims = from.Claims
	}
	if !slices.Contains(parts, Rounds) {
		t.Rounds = from.Rounds
	}
	if !slices.Contains(parts, Comments) {
		t.Comments = from.Comments
	}
	if !slices.Contains(parts, Moves) {
		t.Moves = from.Moves
	}
	if !slices.Contains(parts, Edits) {
		t.Edits = from.Edits
	}
	if !slices.Contains(parts, FailureReports) {
		t.FailureReports = from.FailureReports
	}
	return t
}

// withHead is t, which holds its whole History, with its derived head
// fields worked out from it: every Store Kind keeps them so on every
// write and load.
func withHead(t domain.Ticket) domain.Ticket {
	t.Head = HeadOf(t)
	return t
}

// HeadOf is the derived head fields of t, which holds its whole History.
// The Store keeps them on every write; core works them out afresh for a
// Ticket it changed in an Update, to build a view before writing it.
func HeadOf(t domain.Ticket) domain.TicketHead {
	var h domain.TicketHead
	for _, m := range slices.Backward(t.Moves) {
		if m.To == t.State {
			h.Entered = m.Time
			break
		}
	}
	for _, m := range slices.Backward(t.Moves) {
		if m.To == domain.Ready && m.From != "" {
			h.BackToReady = m.Time
			break
		}
	}
	for _, c := range t.Claims {
		if c.Live() {
			h.LiveClaim = &c
			break
		}
	}
	if n := len(t.Rounds); n > 0 {
		r := t.Rounds[n-1]
		r.Questions = slices.Clone(r.Questions)
		h.LatestRound = &r
	}
	if n := len(t.FailureReports); n > 0 {
		f := t.FailureReports[n-1]
		h.LatestFailureReport = &f
	}
	for _, e := range slices.Backward(t.Edits) {
		if _, ok := e.Previous["external_state"]; ok {
			h.ExternalStateChanged = e.Time
			break
		}
	}
	return h
}

// cloneHead is h with nothing shared with the original.
func cloneHead(h domain.TicketHead) domain.TicketHead {
	if h.LiveClaim != nil {
		c := *h.LiveClaim
		c.Stops = slices.Clone(c.Stops)
		h.LiveClaim = &c
	}
	if h.LatestRound != nil {
		r := *h.LatestRound
		r.Questions = slices.Clone(r.Questions)
		h.LatestRound = &r
	}
	if h.LatestFailureReport != nil {
		f := *h.LatestFailureReport
		h.LatestFailureReport = &f
	}
	return h
}
