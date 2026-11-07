package core

import (
	"context"
	"slices"
	"strings"

	"github.com/tomoram/code-central/store"
)

// ClaimInput is claim's input: narrow to a Spec or name a Ticket, not both.
type ClaimInput struct {
	Spec      Ref    `json:"spec,omitempty"`
	Ticket    Ref    `json:"ticket,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

// ClaimResult is claim's result: the Claim ID with the full Ticket view,
// or {ticket: null} when nothing can be claimed.
type ClaimResult struct {
	ClaimID string      `json:"claim_id,omitempty"`
	Ticket  *TicketView `json:"ticket"`
}

// FinishInput is finish's input.
type FinishInput struct {
	Ticket  Ref    `json:"ticket"`
	ClaimID string `json:"claim_id"`
	Summary string `json:"summary"`
}

// FinishResult is finish's result: the Ticket and the next step.
type FinishResult struct {
	Ticket   TicketView `json:"ticket"`
	NextStep string     `json:"next_step"`
}

// RejectInput is reject's input.
type RejectInput struct {
	Ticket  Ref    `json:"ticket"`
	Comment string `json:"comment"`
}

// stopWorking is the next step after an action that ends the agent's work.
const stopWorking = "stop working this Ticket"

// Claim is claim(spec?, ticket?, session_id?): the next claimable Ticket
// in claim order (or the named one) moves to In Progress under a new
// Claim, in one atomic step under the store's write lock.
func (c *Core) Claim(ctx context.Context, key string, in ClaimInput) (ClaimResult, error) {
	if in.Spec != "" && in.Ticket != "" {
		return ClaimResult{}, invalid("ticket", "pass spec or ticket, not both")
	}
	var res ClaimResult
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		t, found, err := claimTarget(w, key, in)
		if err != nil || !found {
			return err
		}
		if t.State == store.Ready && blocked(w, key, t) {
			return blockedError(w, key, t, ActorOf(ctx))
		}
		now := c.now()
		t.Claims = append(t.Claims, store.Claim{Number: len(t.Claims) + 1, SessionID: in.SessionID, Started: now})
		if err := transition(ctx, &t, key, "claim", "", now); err != nil {
			return err
		}
		claim := &t.Claims[len(t.Claims)-1]
		claim.ID = c.ids.ClaimID()
		if err := w.PutTicket(key, t); err != nil {
			return err
		}
		v := c.ticketView(w, key, t)
		res = ClaimResult{ClaimID: claim.ID, Ticket: &v}
		return nil
	})
	return res, err
}

// claimTarget is the named Ticket, or the first claimable one (in the
// Spec, if given) in claim order. found is false when nothing is claimable.
func claimTarget(r store.Reader, key string, in ClaimInput) (t store.Ticket, found bool, err error) {
	if in.Ticket != "" {
		t, err := ticket(r, key, in.Ticket)
		if err != nil {
			return t, false, err
		}
		// A Ticket that isn't Ready is rejected by the transition with
		// its State as the reason.
		return t, true, nil
	}
	specN := 0
	if in.Spec != "" {
		if specN, err = in.Spec.number(key, "spec"); err != nil {
			return t, false, err
		}
		if _, ok := r.Spec(key, specN); !ok {
			return t, false, invalid("spec", "there is no Spec %s", DisplayID(key, specN))
		}
	}
	tickets := r.Tickets(key)
	slices.SortStableFunc(tickets, claimOrder)
	for _, t := range tickets {
		if t.State == store.Ready && !blocked(r, key, t) && (specN == 0 || t.Spec == specN) {
			return t, true, nil
		}
	}
	return t, false, nil
}

// Finish is finish(ticket, claim_id, summary): the holder moves In
// Progress to In Review, with the summary stored as a comment.
func (c *Core) Finish(ctx context.Context, key string, in FinishInput) (FinishResult, error) {
	if strings.TrimSpace(in.Summary) == "" {
		return FinishResult{}, invalid("summary", "finish needs a summary: what was done, where (branch or commit) and what the reviewer should check")
	}
	v, err := c.act(ctx, key, in.Ticket, "finish", in.ClaimID, in.Summary)
	if err != nil {
		return FinishResult{}, err
	}
	return FinishResult{Ticket: v, NextStep: nextStep("finish")}, nil
}

// Accept is the human's accept(ticket): In Review to Done.
func (c *Core) Accept(ctx context.Context, key string, in TicketInput) (TicketView, error) {
	return c.act(ctx, key, in.Ticket, "accept", "", "")
}

// Reject is the human's reject(ticket, comment): In Review back to Ready,
// with the comment required. The Ticket keeps its created time, so it
// keeps its place near the front of the claim order.
func (c *Core) Reject(ctx context.Context, key string, in RejectInput) (TicketView, error) {
	if strings.TrimSpace(in.Comment) == "" {
		return TicketView{}, invalid("comment", "reject needs a comment saying what to change")
	}
	return c.act(ctx, key, in.Ticket, "reject", "", in.Comment)
}

// act runs one action from the workflow table on a Ticket in one write:
// the transition, then the action's note (if any) as a comment under the
// Claim it was made on.
func (c *Core) act(ctx context.Context, key string, ref Ref, name, claimID, note string) (TicketView, error) {
	return c.updateTicket(key, ref, func(t *store.Ticket) (bool, error) {
		claim := 0
		if live, ok := liveClaim(*t); ok {
			claim = live.Number
		}
		now := c.now()
		if err := transition(ctx, t, key, name, claimID, now); err != nil {
			return false, err
		}
		if note != "" {
			addComment(t, ActorOf(ctx), now, claim, note)
		}
		return true, nil
	})
}

// ticket reads the Ticket ref names, or returns not_found.
func ticket(r store.Reader, key string, ref Ref) (store.Ticket, error) {
	n, err := ref.number(key, "ticket")
	if err != nil {
		return store.Ticket{}, err
	}
	t, ok := r.Ticket(key, n)
	if !ok {
		return store.Ticket{}, notFound("there is no Ticket %s", DisplayID(key, n))
	}
	return t, nil
}
