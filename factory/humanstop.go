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

// afterStop ends the line saying a Claim was resumed after a Stop.
const afterStop = "after a Stop"

// holdStopped holds c if the human has Stopped it on tk, and reports
// whether it did: its turn is stopped and the Claim kept, and a Claim
// with no turn is noted held when it is first seen Stopped.
func (f *factory) holdStopped(c claim, tk api.TicketView) bool {
	if !stoppedNow(tk, c) {
		return false
	}
	f.dropCutOff(c.ClaimID)
	f.restarted = slices.DeleteFunc(f.restarted, func(r claim) bool { return r.ClaimID == c.ClaimID })
	held := f.stops[c.ClaimID]
	f.stops[c.ClaimID] = true
	if t := f.turnFor(c.ClaimID); t != nil {
		f.claimf(c, "%s: stopped by the human: stopping its turn (Claim kept)", c.Ticket)
		f.stopTurn(t)
	} else if !held {
		f.claimf(c, "%s: stopped by the human: holding its Claim", c.Ticket)
	}
	return true
}

// noteResumed notes that c, if held for a Stop, is no longer Stopped.
func (f *factory) noteResumed(c claim) {
	if _, ok := f.stops[c.ClaimID]; ok {
		f.stops[c.ClaimID] = false
	}
}

// owedResume reports whether c was held for a Stop and has no turn, so
// once it is Resumed the poll resumes its session.
func (f *factory) owedResume(c claim) bool {
	_, ok := f.stops[c.ClaimID]
	return ok && f.turnFor(c.ClaimID) == nil
}

// takeStop reports whether c was held for a Stop, and forgets it.
func (f *factory) takeStop(c claim) bool {
	_, ok := f.stops[c.ClaimID]
	delete(f.stops, c.ClaimID)
	return ok
}

// resumeStopped resumes c's session, which a Stop cut off, now that the
// human has Resumed it, unless another process holds it.
func (f *factory) resumeStopped(ctx context.Context, c claim) {
	if f.resumeSession(ctx, c, afterStop, stoppedPrompt(c)) {
		f.takeStop(c)
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
