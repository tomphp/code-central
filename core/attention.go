package core

import (
	"context"
	"strconv"
	"strings"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// Attention is the read behind OS Notifications (package notify): for
// each Ticket, what notify compares between looks to spot a new Round, a
// new Failure Report, a Claim turning Silent or Long or a Ticket going to
// CI failed, and whether the Ticket is in Needs Me. It reads each
// Ticket's reason from its Project's evaluation, so a Ticket notify
// flags is in Needs Me, with a row of its own or in a CI item.

// Attention returns every Ticket in every Project, from one evaluation
// of each.
func (c *Core) Attention(context.Context) []api.AttentionTicket {
	var out []api.AttentionTicket
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
func (c *Core) AttentionOf(_ context.Context, project, id string) (api.AttentionTicket, bool) {
	n, err := strconv.Atoi(strings.TrimPrefix(id, project+"-"))
	if err != nil {
		return api.AttentionTicket{}, false
	}
	var out api.AttentionTicket
	var ok bool
	c.store.View(func(r store.Reader) error {
		var t domain.Ticket
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
func attention(e *evaluation, t domain.Ticket) api.AttentionTicket {
	a := api.AttentionTicket{
		Project: e.p.Key, ID: DisplayID(e.p.Key, t.Number), Title: t.Title,
	}
	if rd := t.Head.LatestRound; rd != nil {
		a.Rounds = rd.Number
	}
	if fr := t.Head.LatestFailureReport; fr != nil {
		a.FailureReports = fr.Number
	}
	reason, _ := e.reason(t)
	if cl, ok := liveClaim(t); ok {
		switch reason {
		case api.ReasonSilentClaim:
			a.SilentClaim = cl.Number
		case api.ReasonLongClaim:
			a.LongClaim = cl.Number
		}
	}
	if it, ok := e.itemOf(t); ok && it.reason == api.ReasonCIFailed {
		a.CIFailedCommit = it.commit
	}
	a.NeedsMe = reason != ""
	return a
}
