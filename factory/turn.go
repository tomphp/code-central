// A Claim's turn: running it through the Turn runner in the worktree,
// taking a slot until it ends.

package factory

import (
	"context"
	"crypto/rand"
	"fmt"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
)

// startTurn runs c's Turn in, which says where it runs, whether it
// starts c's session or resumes it, its Model Choice and its prompt,
// taking a slot until it ends. The Model Choice is recorded on the Claim
// just before claude starts. An effort claude doesn't accept fails the
// Claim instead, and a Turn that never ran gives its slot back with no
// "turn ended" line.
func (f *factory) startTurn(ctx context.Context, c claim, in turnInput) {
	in.Session = c.SessionID
	in.AllowedTools, in.SystemPrompt, in.AddDirs = c.AllowedTools, c.SystemPrompt, c.AddDirs
	in.UsageKind = f.claimInfo(c).stage
	in.OnEvent = func(e core.SessionEvent) { f.events.add(c.SessionID, e) }
	in.OnUsage = func(r core.UsageResponse) { f.usage.add(c.SessionID, r) }
	in.OnPlanLimits = f.planLimits.set
	in.Linef = func(format string, args ...any) {
		f.claimf(c, "%s: %s", c.Ticket, fmt.Sprintf(format, args...))
	}
	in.Echo = func(toolCall string) { f.printf("%s: %s", c.Ticket, toolCall) }
	// Before the Turn starts, which may end the Claim before a report
	// after it could land.
	in.BeforeStart = func() { f.reportChoice(ctx, c, in.Choice) }
	// Busy from here until claude runs, and again once it has exited.
	in.Key = c.Ticket
	in.Work = f.starting("turn ", c.Ticket)
	// Not derived from ctx: a Claim's Turn is stopped only by stopTurn.
	stop, cancel := context.WithCancel(context.Background())
	t := &turn{claim: c, dir: in.Dir, choice: in.Choice, started: f.e.Clock.Now(), cancel: cancel, stopWork: newHandoff(), done: make(chan struct{})}
	in.StopWork = t.stopWork
	f.mu.Lock()
	f.turns[c.ClaimID] = t
	f.mu.Unlock()
	f.turnsDone.Add(1)
	go func() {
		defer in.Work.stop()
		// Last, whatever the outcome and even if no Ticket changed: the
		// slot is free, so the poll loop polls at once.
		defer f.wake()
		defer f.turnsDone.Done()
		defer f.endTurn(t)
		defer cancel()
		out := f.runner.run(stop, in)
		if out.Kind == turnEffortRejected || out.StartErr != nil {
			// The slot is held while the report is filed, so no poll acts
			// on the Claim meanwhile.
			if out.Kind == turnEffortRejected {
				f.reportRejectedEffort(ctx, c, in.Dir, in.Choice, out.Levels)
			}
			f.freeSlot(c)
			f.forgetEnded(c)
			return
		}
		ran := f.e.Clock.Now().Sub(t.started)
		// The turn keeps its slot until its outcome is acted on, and
		// "turn ended" is printed once the slot is free.
		state := f.afterTurn(ctx, t, out)
		if state == domain.Ready {
			f.holdBack(c.Ticket)
		}
		f.freeSlot(c)
		f.turnEnded(t, out.End, state, ran)
		f.forgetEnded(c)
	}()
}

// freeSlot gives c's turn's slot up.
func (f *factory) freeSlot(c claim) {
	f.mu.Lock()
	delete(f.turns, c.ClaimID)
	f.mu.Unlock()
}

// holdBack keeps ticket, whose turn just ended with it back in Ready,
// from being claimed again until the next timed poll or until --poll
// has passed, whichever is first: wakes alone could put the timed poll
// off for good.
func (f *factory) holdBack(ticket string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.backInReady[ticket] = f.e.Clock.Now()
}

// releaseHeldBack lets every held-back Ticket be claimed again, as a
// timed poll is starting.
func (f *factory) releaseHeldBack() {
	f.mu.Lock()
	defer f.mu.Unlock()
	clear(f.backInReady)
}

// heldBack reports whether ticket is held back: a turn on it is still
// running, as when its agent has given up but not yet exited, or its
// turn ended with it back in Ready since the last timed poll, less than
// --poll ago. A turn is held back before it gives its slot up, so the
// two leave no moment between them.
func (f *factory) heldBack(ticket string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.turns {
		if t.claim.Ticket == ticket {
			return true
		}
	}
	at, ok := f.backInReady[ticket]
	if ok && f.e.Clock.Now().Sub(at) >= f.o.poll {
		delete(f.backInReady, ticket)
		return false
	}
	return ok
}

// newSessionID is a random UUID (version 4), as claude --session-id takes.
func newSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
