package core

import (
	"context"
	"strconv"
	"strings"

	"github.com/tomoram/code-central/store"
)

// Attention is the read behind OS Notifications (package notify): for
// each Ticket, what notify compares between looks to spot a new Round, a
// new Failure Report, a Claim turning Silent or Long or a Ticket going to
// CI failed, and whether the Ticket is in Needs Me. It reads each
// Ticket's reason from its Project's evaluation, so a Ticket notify
// flags is in Needs Me, with a row of its own or in a CI item.

// AttentionTicket is one Ticket as notify watches it.
type AttentionTicket struct {
	Project        string
	ID             string
	Title          string
	Rounds         int
	FailureReports int
	// SilentClaim is the number of the live Claim while it is Silent,
	// else 0.
	SilentClaim int
	// LongClaim is the number of the live Claim while it is Long, else 0.
	LongClaim int
	// CIFailedCommit is the pushed commit while the Ticket is CI failed,
	// else "".
	CIFailedCommit string
	NeedsMe        bool
}

// Attention returns every Ticket in every Project, from one evaluation
// of each.
func (c *Core) Attention(context.Context) []AttentionTicket {
	var out []AttentionTicket
	c.store.View(func(r store.Reader) error {
		for _, p := range r.Projects() {
			e := c.evaluate(r, p)
			for _, t := range e.tickets() {
				out = append(out, attention(e, t))
			}
		}
		return nil
	})
	return out
}

// AttentionOf returns one Ticket by display ID, or false if there is none.
func (c *Core) AttentionOf(_ context.Context, project, id string) (AttentionTicket, bool) {
	n, err := strconv.Atoi(strings.TrimPrefix(id, project+"-"))
	if err != nil {
		return AttentionTicket{}, false
	}
	var out AttentionTicket
	var ok bool
	c.store.View(func(r store.Reader) error {
		var t store.Ticket
		if t, ok = r.Ticket(project, n); ok {
			p, _ := r.Project(project)
			out = attention(c.evaluate(r, p), t)
		}
		return nil
	})
	return out, ok
}

// attention is t as notify watches it, with its reason from its
// Project's evaluation e.
func attention(e *evaluation, t store.Ticket) AttentionTicket {
	a := AttentionTicket{
		Project: e.p.Key, ID: DisplayID(e.p.Key, t.Number), Title: t.Title,
		Rounds: len(t.Rounds), FailureReports: len(t.FailureReports),
	}
	reason, _ := e.reason(t)
	if cl, ok := liveClaim(t); ok {
		switch reason {
		case ReasonSilentClaim:
			a.SilentClaim = cl.Number
		case ReasonLongClaim:
			a.LongClaim = cl.Number
		}
	}
	if it, ok := e.itemOf(t); ok && it.reason == ReasonCIFailed {
		a.CIFailedCommit = it.commit
	}
	a.NeedsMe = reason != ""
	return a
}
