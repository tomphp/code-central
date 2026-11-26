package core

import (
	"context"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

// SetSession is set_session(ticket, claim_id, session_id, working_dir?,
// resume_command?): the holder records the session now working its Claim,
// in any State the Claim spans, and how to get back into it. Pings from
// that session then count as the Claim's activity. The last write wins.
func (c *Core) SetSession(_ context.Context, key string, in api.SetSessionInput) (api.Written[api.TicketView], error) {
	return c.updateTicket(key, "set_session", in.Ticket, in.Full, func(t *domain.Ticket) (bool, error) {
		cl, err := holder(t, key, in.ClaimID)
		if err != nil {
			return false, err
		}
		before := *cl
		cl.SessionID = in.SessionID
		setResume(cl, in.WorkingDir, in.ResumeCommand)
		return cl.SessionID != before.SessionID || cl.WorkingDir != before.WorkingDir || cl.ResumeCommand != before.ResumeCommand, nil
	})
}

// setResume writes each of the working directory and Resume Command that
// is given, leaving the others as they are.
func setResume(cl *domain.Claim, workingDir, resumeCommand *string) {
	if workingDir != nil {
		cl.WorkingDir = *workingDir
	}
	if resumeCommand != nil {
		cl.ResumeCommand = *resumeCommand
	}
}

// Release is the human's release(ticket, note?): In Progress or Answered
// back to Ready, ending the Claim for good, so the released session's
// next write is stale. Any note is stored as a comment.
func (c *Core) Release(ctx context.Context, key string, in api.ReleaseInput) (api.Written[api.TicketView], error) {
	return c.act(ctx, key, in.Ticket, in.Full, "release", "", in.Note)
}
