package core

import (
	"context"
	"slices"

	"github.com/tomoram/code-central/store"
)

// CancelInput is cancel's input. An agent must name at least one
// Replacement; the human may name none. The reason is stored as a comment.
type CancelInput struct {
	Ticket       Ref    `json:"ticket"`
	Replacements []Ref  `json:"replacements,omitempty"`
	Reason       string `json:"reason,omitempty"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// Cancel is the agent's cancel(ticket, replacements, reason?) on a Draft
// or Ready Ticket, and the human's cancel(replacements?) on any
// unfinished one. Anything the Ticket was blocking becomes blocked by its
// Replacements when read, so only this Ticket's file is written.
func (c *Core) Cancel(ctx context.Context, key string, in CancelInput) (Written[TicketView], error) {
	actor := ActorOf(ctx)
	return c.updateTicketIn(key, "cancel", in.Ticket, in.Full, func(r store.Reader, t *store.Ticket) (bool, error) {
		claim := 0
		if live, ok := liveClaim(*t); ok {
			claim = live.Number
		}
		before := *t
		now := c.now()
		if err := transition(ctx, t, key, "cancel", "", now); err != nil {
			return false, err
		}
		if actor != Human && len(in.Replacements) == 0 {
			return false, invalid("replacements", "an agent cancels a Ticket only by naming at least one Replacement that takes over its work")
		}
		reps, err := ticketRefs(r, key, "cancel", "replacements", in.Replacements)
		if err != nil {
			return false, err
		}
		if err := checkReplacements(r, key, before, reps); err != nil {
			return false, err
		}
		t.Replacements = reps
		if in.Reason != "" {
			addComment(t, actor, now, claim, in.Reason)
		}
		return true, nil
	})
}

// checkReplacements rejects Replacements for t that are t itself, that
// lead back to t through other Replacements, or that would make a Ticket
// t was blocking wait on itself.
func checkReplacements(r store.Reader, key string, t store.Ticket, reps []int) error {
	var from []int
	for _, n := range reps {
		if n == t.Number {
			return invalid("replacements", "%s can't replace itself", DisplayID(key, n))
		}
		followed := store.Follow(r, key, n)
		if slices.Contains(followed, t.Number) {
			return cycleError(key, []int{t.Number, n, t.Number})
		}
		from = append(from, followed...)
	}
	var dependants []int
	for _, d := range r.Tickets(key) {
		if slices.Contains(waitsOn(r, key, d.Number), t.Number) {
			dependants = append(dependants, d.Number)
		}
	}
	if len(dependants) == 0 {
		return nil
	}
	cancelled := t
	cancelled.State, cancelled.Replacements = store.Cancelled, reps
	ov := overlay{Reader: r, key: key, t: cancelled}
	for _, d := range dependants {
		if path := waitPath(ov, key, from, func(n int) bool { return n == d }); path != nil {
			return cycleError(key, append([]int{d}, path...))
		}
	}
	return nil
}
