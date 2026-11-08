package factory

import (
	"context"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// restart reconciles the Claims a past Factory saved, before the first
// poll. An ended Claim is dropped, and so is one whose Ticket is In
// Review, Accepted or Done. An Answered one is kept: the poll picks up
// its answers and resumes it before anything else. An In Progress one was
// cut off by the stop, so it is resumed as cut off once a slot is free.
// A Needs Answer one is kept. A Claim the file doesn't hold is never
// looked for, so never adopted.
func (f *factory) restart(ctx context.Context) {
	for _, c := range f.heldClaims() {
		tk, err := f.client.GetTicket(ctx, f.project, core.Ref(c.Ticket))
		if err != nil {
			if ctx.Err() == nil {
				f.claimf(c, "%s: get_ticket: %v: keeping its saved Claim", c.Ticket, err)
			}
			continue
		}
		if ended, by := claimEnded(tk, c); ended {
			f.claimf(c, "%s: saved Claim %s has ended (%s): dropping it", c.Ticket, c.ClaimID, by)
			f.endClaim(c, by)
			continue
		}
		f.noteTicket(c, tk)
		switch tk.State {
		case store.InProgress:
			f.claimf(c, "%s: saved Claim %s was cut off: resuming it", c.Ticket, c.ClaimID)
			f.restarted = append(f.restarted, c)
		case store.Answered, store.NeedsAnswer:
			f.claimf(c, "%s: saved Claim %s is %s: keeping it", c.Ticket, c.ClaimID, tk.State)
		default:
			f.claimf(c, "%s: saved Claim %s is %s: dropping it", c.Ticket, c.ClaimID, tk.State)
			f.endClaim(c, string(tk.State))
		}
	}
}

// resumeRestarted resumes the cut-off saved Claims while slots are free.
// One another process holds waits a poll; one that has ended is gone.
func (f *factory) resumeRestarted(ctx context.Context) {
	var wait []claim
	for len(f.restarted) > 0 && !f.halted(ctx) && f.running() < f.o.parallel {
		c := f.restarted[0]
		f.restarted = f.restarted[1:]
		if !f.resumeCutOff(ctx, c) && f.live(c.ClaimID) {
			wait = append(wait, c)
		}
	}
	f.restarted = append(wait, f.restarted...)
}
