package core

import (
	"context"
	"strconv"
	"strings"

	"github.com/tomoram/code-central/store"
)

// Attention is the read behind OS Notifications (package notify): for
// each Ticket, what notify compares between looks to spot a new Round, a
// new Failure Report or a Claim turning Silent, and whether the Ticket is
// in Needs Me.

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
	NeedsMe     bool
}

// Attention returns every Ticket in every Project.
func (c *Core) Attention(context.Context) []AttentionTicket {
	var out []AttentionTicket
	c.store.View(func(r store.Reader) error {
		for _, p := range r.Projects() {
			for _, t := range r.Tickets(p.Key) {
				out = append(out, c.attention(r, p.Key, t))
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
			out = c.attention(r, project, t)
		}
		return nil
	})
	return out, ok
}

func (c *Core) attention(r store.Reader, key string, t store.Ticket) AttentionTicket {
	a := AttentionTicket{
		Project: key, ID: DisplayID(key, t.Number), Title: t.Title,
		Rounds: len(t.Rounds), FailureReports: len(t.FailureReports),
	}
	silent := c.silentSince(t) != nil
	if cl, ok := liveClaim(t); ok && silent {
		a.SilentClaim = cl.Number
	}
	a.NeedsMe = needsMe(r, key, t, silent)
	return a
}

// needsMe is a stand-in for ticket 32's needs_me_reason, which the
// merger should use here instead (NeedsMe = reason != ""). It covers
// every reason but "blocked by a Cancelled Ticket with no Replacements"
// and "closed externally".
func needsMe(_ store.Reader, _ string, t store.Ticket, silent bool) bool {
	switch t.State {
	case store.NeedsAnswer, store.Answered, store.Failed, store.InReview:
		return true
	}
	return silent
}
