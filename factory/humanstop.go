// A human's Stop of a Factory Claim: its turn is stopped and the Claim
// held, taking no slot and resumed by no path, until a Resume, when the
// same session is resumed.

package factory

import (
	"context"
	"fmt"
	"slices"

	"github.com/tomoram/code-central/api"
)

// holdIfStopped reports whether the human has Stopped c, whose Claim on
// its Ticket is v. A Stopped Claim's turn is stopped and the Claim kept,
// and a Claim with no turn is noted held the first time it is seen.
func (f *factory) holdIfStopped(c claim, v api.ClaimView) bool {
	if !v.Stopped {
		return false
	}
	f.dropCutOff(c.ClaimID)
	f.restarted = slices.DeleteFunc(f.restarted, func(r claim) bool { return r.ClaimID == c.ClaimID })
	seen := f.stoppedSeen[c.ClaimID]
	f.stoppedSeen[c.ClaimID] = true
	if t := f.turnFor(c.ClaimID); t != nil {
		f.claimf(c, "%s: stopped by the human: stopping its turn (Claim kept)", c.Ticket)
		f.stopTurn(t)
	} else if !seen {
		f.claimf(c, "%s: stopped by the human: holding its Claim", c.Ticket)
	}
	return true
}

// resumedAfterStop reports whether c was seen Stopped and has since been
// Resumed, so the poll resumes its session.
func (f *factory) resumedAfterStop(c claim) bool {
	return f.stoppedSeen[c.ClaimID] && f.turnFor(c.ClaimID) == nil
}

// resumeStopped resumes c's session, which a Stop cut off, now that the
// human has Resumed it, unless another process holds it.
func (f *factory) resumeStopped(ctx context.Context, c claim) {
	if f.resumeSession(ctx, c, "after a Stop", stoppedPrompt(c)) {
		delete(f.stoppedSeen, c.ClaimID)
	}
}

func stoppedPrompt(c claim) string {
	return fmt.Sprintf(`A human stopped your last turn and has now resumed you; carry on.

Ticket: %s
Claim: %s
`, c.Ticket, c.ClaimID)
}

// stoppedNow reports whether c's Claim on tk is live and Stopped.
func stoppedNow(tk api.TicketView, c claim) bool {
	v, ok := claimOn(tk, c)
	return ok && v.Stopped
}
