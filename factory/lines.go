package factory

import (
	"fmt"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/factory/harness"
)

// claimInfo is what the Factory's lines say of a held Claim's Ticket. It
// is kept in memory only, filled at the claim and from each read of the
// Ticket, so a restart fills it again from the first read.
type claimInfo struct {
	title string
	stage string // the guide's name, a domain Stage name, or prototype, retro or implementation
}

// noteTicket keeps what the lines say of c's Ticket, as tk gives it,
// unless c has ended.
func (f *factory) noteTicket(c claim, tk api.TicketView) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ended[c.ClaimID] {
		return
	}
	f.info[c.ClaimID] = claimInfo{title: tk.Title, stage: api.GuideName(tk.Labels)}
}

// about is "(<stage>): "<title>"", naming c's Ticket in a line.
func (f *factory) about(c claim) string {
	i := f.claimInfo(c)
	return fmt.Sprintf("(%s): %q", i.stage, i.title)
}

// claimInfo is what the lines say of c's Ticket; with nothing noted, its
// stage is implementation.
func (f *factory) claimInfo(c claim) claimInfo {
	f.mu.Lock()
	i, ok := f.info[c.ClaimID]
	f.mu.Unlock()
	if !ok {
		i.stage = api.ImplementationStage
	}
	return i
}

// claimf prints a line about the held Claim c and queues it as a factory
// Session Event for c's session. Every such line goes through here, the
// one per-Claim logger; lines with no Claim are only printed, and so is
// a --verbose tool-call line, whose tool call is posted already.
func (f *factory) claimf(c claim, format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	f.printf("%s", line)
	f.events.add(c.SessionID, api.SessionEvent{Time: f.e.Clock.Now(), Kind: api.EventFactory, Text: line})
}

// claimed prints the line for a new Claim on tk, once its Home worktree
// wt and its Resume Command are known and before its turn starts.
func (f *factory) claimed(c claim, tk api.TicketView, wt, resume string) {
	f.claimf(c, "%s claimed %s in Spec %s; worktree %s; resume: %s", c.Ticket, f.about(c), tk.Spec.ID, wt, resume)
}

// turnEnded prints the line for a turn that has ended and freed its
// slot: how it exited, the State the Factory left its Ticket in, if it
// read one, and how long it ran.
func (f *factory) turnEnded(t *turn, out harness.Outcome, state domain.State, ran time.Duration) {
	c := t.claim
	now := ""
	if state != "" {
		now = "; now " + string(state)
	}
	f.claimf(c, "%s turn ended %s: %s%s; ran %s", c.Ticket, f.about(c), out.Exit, now, ran.Round(time.Second))
}
