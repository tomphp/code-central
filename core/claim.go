package core

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/tomoram/code-central/store"
)

// ClaimInput is claim's input: narrow to a Spec or name a Ticket, not both.
type ClaimInput struct {
	Spec      Ref    `json:"spec,omitempty"`
	Ticket    Ref    `json:"ticket,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	// WorkingDir and ResumeCommand are free-form and never parsed; nil
	// leaves them unset.
	WorkingDir    *string `json:"working_dir,omitempty"`
	ResumeCommand *string `json:"resume_command,omitempty"`
}

// ClaimResult is claim's result: the Claim ID with the full Ticket view
// and the Stage Guide for its Stage (implementation when it has none),
// or {ticket: null} when nothing can be claimed.
type ClaimResult struct {
	ClaimID    string          `json:"claim_id,omitempty"`
	Ticket     *TicketView     `json:"ticket"`
	StageGuide *StageGuideView `json:"stage_guide,omitempty"`
}

// FinishInput is finish's input.
type FinishInput struct {
	Ticket  Ref    `json:"ticket"`
	ClaimID string `json:"claim_id"`
	Summary string `json:"summary"`
	// Branch and Commit name the work's branch and its head commit. Both
	// are optional and free-form; one left out keeps the stored value. A
	// Ticket with a branch is Accepted, not Done, when the human accepts it.
	Branch string `json:"branch,omitempty"`
	Commit string `json:"commit,omitempty"`
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
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// stopWorking is the next step after an action that ends the agent's work.
const stopWorking = "stop working this Ticket"

// Claim is claim(spec?, ticket?, session_id?, working_dir?, resume_command?): the next claimable Ticket
// in claim order (or the named one) moves to In Progress under a new
// Claim, in one atomic step under the store's write lock. A named Ticket
// whose live Claim has the caller's non-empty session ID returns that
// Claim instead, writing nothing.
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
		if live, ok := liveClaim(t); ok && in.Ticket != "" {
			if in.SessionID == "" || live.SessionID != in.SessionID {
				return heldError(key, t, live, in.SessionID, ActorOf(ctx))
			}
			// The holding session claims again: its Claim, nothing written
			// (ADR 0010).
			res = c.claimResult(w, key, t, live.ID)
			return nil
		}
		if t.State == store.Ready && blocked(w, key, t) {
			return blockedError(w, key, t, ActorOf(ctx))
		}
		now := c.now()
		cl := store.Claim{Number: len(t.Claims) + 1, SessionID: in.SessionID, Started: now}
		setResume(&cl, in.WorkingDir, in.ResumeCommand)
		t.Claims = append(t.Claims, cl)
		if err := transition(ctx, &t, key, "claim", "", now); err != nil {
			return err
		}
		claim := &t.Claims[len(t.Claims)-1]
		claim.ID = c.ids.ClaimID()
		if err := w.PutTicket(key, t); err != nil {
			return err
		}
		res = c.claimResult(w, key, t, claim.ID)
		return nil
	})
	return res, err
}

// claimResult is claim's result for Claim id on t.
func (c *Core) claimResult(r store.Reader, key string, t store.Ticket, id string) ClaimResult {
	v := c.ticketView(r, key, t)
	g := stageGuide(GuideStage(t.Labels))
	return ClaimResult{ClaimID: id, Ticket: &v, StageGuide: &g}
}

// heldError is claim's action_not_allowed on a named Ticket whose live
// Claim isn't the caller's, saying who holds it and what to do.
func heldError(key string, t store.Ticket, live *store.Claim, sessionID string, actor Actor) error {
	msg := fmt.Sprintf("%s is held by Claim %d of another session; claim another Ticket, or ask the human to release it",
		DisplayID(key, t.Number), live.Number)
	if sessionID == "" {
		msg += "; if this session holds it, call claim again with your session_id"
	}
	return &Error{Code: ActionNotAllowed, Message: msg, AllowedActions: allowedActions(t.State, actor)}
}

// claimTarget is the named Ticket, or the first claimable one (in the
// Spec, if given) in claim order. found is false when nothing is claimable.
func claimTarget(r store.Reader, key string, in ClaimInput) (t store.Ticket, found bool, err error) {
	if in.Ticket != "" {
		t, err := resolveTicket(r, key, "claim", "ticket", in.Ticket)
		if err != nil {
			return t, false, err
		}
		// A Ticket that isn't Ready is rejected by the transition with
		// its State as the reason.
		return t, true, nil
	}
	specN := 0
	if in.Spec != "" {
		sp, err := resolveSpec(r, key, "claim", "spec", in.Spec)
		if err != nil {
			return t, false, err
		}
		specN = sp.Number
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
	// With full: finish's result still holds the whole Ticket.
	v, err := c.actThen(ctx, key, in.Ticket, true, "finish", in.ClaimID, in.Summary, func(t *store.Ticket) error {
		if b := strings.TrimSpace(in.Branch); b != "" {
			t.Branch = b
		}
		if cm := strings.TrimSpace(in.Commit); cm != "" {
			t.Commit = cm
		}
		return nil
	})
	if err != nil {
		return FinishResult{}, err
	}
	return FinishResult{Ticket: *v.Record, NextStep: nextStep("finish")}, nil
}

// Accept is the human's accept(ticket): In Review to Accepted when the
// work's finish named a branch, so it waits to land (ADR 0006), and to
// Done otherwise.
func (c *Core) Accept(ctx context.Context, key string, in TicketInput) (Written[TicketView], error) {
	return c.updateTicket(key, "accept", in.Ticket, in.Full, func(t *store.Ticket) (bool, error) {
		to := store.Done
		if t.Branch != "" {
			to = store.Accepted
		}
		return true, transitionTo(ctx, t, key, "accept", "", to, c.now())
	})
}

// LandInput is land's input: the commit that put the work into the base.
type LandInput struct {
	Ticket Ref    `json:"ticket"`
	Commit string `json:"commit"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// Land is land(ticket, commit): human or agent, Accepted or In Review to
// Done once the work is in the base. The commit is recorded on the Move,
// with Via merge.
func (c *Core) Land(ctx context.Context, key string, in LandInput) (Written[TicketView], error) {
	commit := strings.TrimSpace(in.Commit)
	if commit == "" {
		return Written[TicketView]{}, invalid("commit", "land needs the commit that put the work into the base")
	}
	return c.actThen(ctx, key, in.Ticket, in.Full, "land", "", "", func(t *store.Ticket) error {
		m := &t.Moves[len(t.Moves)-1]
		m.Via, m.Commit = "merge", commit
		return nil
	})
}

// Reject is the human's reject(ticket, comment): In Review back to Ready,
// with the comment required. The Ticket keeps its created time, so it
// keeps its place near the front of the claim order.
func (c *Core) Reject(ctx context.Context, key string, in RejectInput) (Written[TicketView], error) {
	if strings.TrimSpace(in.Comment) == "" {
		return Written[TicketView]{}, invalid("comment", "reject needs a comment saying what to change")
	}
	return c.act(ctx, key, in.Ticket, in.Full, "reject", "", in.Comment)
}

// act runs one action from the workflow table on a Ticket in one write:
// the transition, then the action's note (if any) as a comment under the
// Claim it was made on. Its result is the Acknowledgement, or with full
// the Ticket.
func (c *Core) act(ctx context.Context, key string, ref Ref, full bool, name, claimID, note string) (Written[TicketView], error) {
	return c.actThen(ctx, key, ref, full, name, claimID, note, nil)
}

// actThen is act with then applied to the Ticket after the transition,
// in the same write. An error from then abandons the write.
func (c *Core) actThen(ctx context.Context, key string, ref Ref, full bool, name, claimID, note string, then func(*store.Ticket) error) (Written[TicketView], error) {
	return c.updateTicket(key, name, ref, full, func(t *store.Ticket) (bool, error) {
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
		if then != nil {
			if err := then(t); err != nil {
				return false, err
			}
		}
		return true, nil
	})
}
