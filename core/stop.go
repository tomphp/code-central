package core

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core/foundation"
	"github.com/tomoram/code-central/domain"
)

// Stop is the human's stop(ticket, note?): it marks the live Factory
// Claim of an In Progress or Answered Ticket Stopped, keeping the Ticket's
// State and the Claim (ADR 0077). Any note is stored as a comment.
func (c *Core) Stop(ctx context.Context, key string, in api.StopInput) (api.Written[api.TicketView], error) {
	return c.markClaim(ctx, key, "stop", in.Ticket, in.Note, in.Full, whyNotStoppable, func(cl *domain.Claim, now time.Time) {
		cl.Stops = append(slices.Clone(cl.Stops), domain.Stop{Stopped: now})
	})
}

// Resume is the human's resume(ticket, note?): it ends the Stopped mark
// of the live Claim, so the Factory takes the Claim up again.
func (c *Core) Resume(ctx context.Context, key string, in api.ResumeInput) (api.Written[api.TicketView], error) {
	return c.markClaim(ctx, key, "resume", in.Ticket, in.Note, in.Full, whyNotResumable, func(cl *domain.Claim, now time.Time) {
		cl.Stops = slices.Clone(cl.Stops)
		cl.Stops[len(cl.Stops)-1].Resumed = now
	})
}

// markClaim changes the live Claim's Stopped mark by mark, once why
// finds nothing against it, and stores any note as a comment under it.
func (c *Core) markClaim(ctx context.Context, key, op string, ref api.Ref, note string, full bool,
	why func(key string, t domain.Ticket) string, mark func(*domain.Claim, time.Time)) (api.Written[api.TicketView], error) {
	if err := foundation.HumanOnly(ctx, op); err != nil {
		return api.Written[api.TicketView]{}, err
	}
	return c.updateTicket(key, op, ref, full, func(t *domain.Ticket) (bool, error) {
		if why := why(key, *t); why != "" {
			return false, &api.Error{Code: api.ActionNotAllowed, Message: why}
		}
		cl, _ := claimToWrite(t)
		now := c.now()
		mark(cl, now)
		if note != "" {
			addComment(t, api.Human, now, cl.Number, note)
		}
		return true, nil
	})
}

// whyNotStoppable is why stop would refuse t, or "" when it would accept
// it: only a live Factory Claim not yet Stopped, on an In Progress or
// Answered Ticket.
func whyNotStoppable(key string, t domain.Ticket) string {
	id := domain.DisplayID(key, t.Number)
	cl, live := liveClaim(t)
	switch {
	case live && cl.Stopped():
		return fmt.Sprintf("%s's Claim is already Stopped: resume or release it", id)
	case t.State == domain.NeedsAnswer:
		return fmt.Sprintf("%s is Needs Answer: answer its Round instead", id)
	case !live || (t.State != domain.InProgress && t.State != domain.Answered):
		return fmt.Sprintf("%s is %s: only an In Progress or Answered Ticket's Claim can be Stopped", id, t.State)
	case !cl.FactoryClaim():
		return fmt.Sprintf("%s's Claim was not made by a Factory: only a Factory Claim can be Stopped", id)
	}
	return ""
}

// stoppedSince is when t's live Claim was Stopped, if it is.
func stoppedSince(t domain.Ticket) (time.Time, bool) {
	cl, live := liveClaim(t)
	if !live || !cl.Stopped() {
		return time.Time{}, false
	}
	return cl.Stops[len(cl.Stops)-1].Stopped, true
}

// latestResume is when cl was last Resumed, zero if never.
func latestResume(cl domain.Claim) time.Time {
	var last time.Time
	for _, s := range cl.Stops {
		if s.Resumed.After(last) {
			last = s.Resumed
		}
	}
	return last
}

// whyNotResumable is why resume would refuse t, or "" when its live Claim
// is Stopped.
func whyNotResumable(key string, t domain.Ticket) string {
	id := domain.DisplayID(key, t.Number)
	cl, live := liveClaim(t)
	switch {
	case !live:
		return fmt.Sprintf("%s has no live Claim to resume", id)
	case !cl.Stopped():
		return fmt.Sprintf("%s's Claim is not Stopped", id)
	}
	return ""
}
