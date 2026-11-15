// A Claim's turn: running it through the Turn runner in the worktree,
// taking a slot until it ends.

package factory

import (
	"context"
	"crypto/rand"
	"fmt"

	"github.com/tomoram/code-central/core"
)

// startTurn runs c's Turn in, which says where it runs, whether it
// starts c's session or resumes it, its Model Choice and its prompt,
// taking a slot until it ends. The Model Choice is recorded on the Claim
// just before claude starts. An effort claude doesn't accept fails the
// Claim instead, and a Turn that never ran gives its slot back with no
// "turn ended" line.
func (f *factory) startTurn(ctx context.Context, c claim, in turnInput) {
	in.Session = c.SessionID
	in.AllowedTools, in.SystemPrompt = c.AllowedTools, c.SystemPrompt
	in.UsageKind = f.claimInfo(c).stage
	in.OnEvent = func(e core.SessionEvent) { f.events.add(c.SessionID, e) }
	in.OnUsage = func(r core.UsageResponse) { f.usage.add(c.SessionID, r) }
	in.Linef = func(format string, args ...any) {
		f.claimf(c, "%s: %s", c.Ticket, fmt.Sprintf(format, args...))
	}
	in.Echo = func(toolCall string) { f.printf("%s: %s", c.Ticket, toolCall) }
	// Before the Turn starts, which may end the Claim before a report
	// after it could land.
	in.BeforeStart = func() { f.reportChoice(ctx, c, in.Choice) }
	// Not derived from ctx: a Claim's Turn is stopped only by stopTurn.
	stop, cancel := context.WithCancel(context.Background())
	t := &turn{claim: c, dir: in.Dir, choice: in.Choice, started: f.e.Now(), cancel: cancel, done: make(chan struct{})}
	f.mu.Lock()
	f.turns[c.ClaimID] = t
	f.mu.Unlock()
	f.turnsDone.Add(1)
	go func() {
		// Last, whatever the outcome and even if no Ticket changed: the
		// slot is free, so the poll loop polls at once.
		defer f.wake()
		defer f.turnsDone.Done()
		defer close(t.done)
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
		ran := f.e.Now().Sub(t.started)
		// The turn keeps its slot until its outcome is acted on, and
		// "turn ended" is printed once the slot is free.
		state := f.afterTurn(ctx, t, out)
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
