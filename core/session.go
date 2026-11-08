package core

import (
	"context"

	"github.com/tomoram/code-central/store"
)

// SetSessionInput is set_session's input. The session ID is never
// checked; it is only shown to the human for resuming the session. The
// working directory and Resume Command are free-form and never parsed:
// one that isn't given (nil) leaves the stored value alone, and one given
// empty clears it.
type SetSessionInput struct {
	Ticket        Ref     `json:"ticket"`
	ClaimID       string  `json:"claim_id"`
	SessionID     string  `json:"session_id"`
	WorkingDir    *string `json:"working_dir,omitempty"`
	ResumeCommand *string `json:"resume_command,omitempty"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// SetSession is set_session(ticket, claim_id, session_id, working_dir?,
// resume_command?): the holder records the session now working its Claim,
// in any State the Claim spans, and how to get back into it. Pings from
// that session then count as the Claim's activity. The last write wins.
func (c *Core) SetSession(_ context.Context, key string, in SetSessionInput) (Written[TicketView], error) {
	return c.updateTicket(key, "set_session", in.Ticket, in.Full, func(t *store.Ticket) (bool, error) {
		cl, err := holder(t, key, in.ClaimID)
		if err != nil {
			return false, err
		}
		before := *cl
		cl.SessionID = in.SessionID
		setResume(cl, in.WorkingDir, in.ResumeCommand)
		return *cl != before, nil
	})
}

// setResume writes each of the working directory and Resume Command that
// is given, leaving the others as they are.
func setResume(cl *store.Claim, workingDir, resumeCommand *string) {
	if workingDir != nil {
		cl.WorkingDir = *workingDir
	}
	if resumeCommand != nil {
		cl.ResumeCommand = *resumeCommand
	}
}

// ReleaseInput is release's input; the note is optional.
type ReleaseInput struct {
	Ticket Ref    `json:"ticket"`
	Note   string `json:"note,omitempty"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// Release is the human's release(ticket, note?): In Progress or Answered
// back to Ready, ending the Claim for good, so the released session's
// next write is stale. Any note is stored as a comment.
func (c *Core) Release(ctx context.Context, key string, in ReleaseInput) (Written[TicketView], error) {
	return c.act(ctx, key, in.Ticket, in.Full, "release", "", in.Note)
}
