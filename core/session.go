package core

import (
	"context"

	"github.com/tomoram/code-central/store"
)

// SetSessionInput is set_session's input. The session ID is never
// checked; it is only shown to the human for resuming the session.
type SetSessionInput struct {
	Ticket    Ref    `json:"ticket"`
	ClaimID   string `json:"claim_id"`
	SessionID string `json:"session_id"`
}

// SetSession is set_session(ticket, claim_id, session_id): the holder
// records the session now working its Claim, in any State the Claim
// spans. Pings from that session then count as the Claim's activity.
func (c *Core) SetSession(_ context.Context, key string, in SetSessionInput) (TicketView, error) {
	return c.updateTicket(key, in.Ticket, func(t *store.Ticket) (bool, error) {
		cl, err := holder(t, key, in.ClaimID)
		if err != nil {
			return false, err
		}
		if cl.SessionID == in.SessionID {
			return false, nil
		}
		cl.SessionID = in.SessionID
		return true, nil
	})
}

// ReleaseInput is release's input; the note is optional.
type ReleaseInput struct {
	Ticket Ref    `json:"ticket"`
	Note   string `json:"note,omitempty"`
}

// Release is the human's release(ticket, note?): In Progress or Answered
// back to Ready, ending the Claim for good, so the released session's
// next write is stale. Any note is stored as a comment.
func (c *Core) Release(ctx context.Context, key string, in ReleaseInput) (TicketView, error) {
	return c.act(ctx, key, in.Ticket, "release", "", in.Note)
}
