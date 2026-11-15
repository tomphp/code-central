// A Claim's turn: running it through the Turn runner in the worktree,
// taking a slot until it ends.

package factory

import (
	"context"
	"crypto/rand"
	"fmt"

	"github.com/tomoram/code-central/core"
)

// startTurn runs a Turn of claude for c in dir, taking a slot until it
// ends: it starts c's session, or resumes it, with mc and sends prompt.
// mc is recorded on the Claim just before claude starts. An effort
// claude doesn't accept fails the Claim instead, and a Turn that never
// ran gives its slot straight back.
func (f *factory) startTurn(ctx context.Context, c claim, dir string, mc turnChoice, resume bool, prompt string) {
	// Not derived from ctx: a Claim's Turn is stopped only by stopTurn.
	stop, cancel := context.WithCancel(context.Background())
	t := &turn{claim: c, dir: dir, choice: mc, started: f.e.Now(), cancel: cancel, done: make(chan struct{})}
	f.mu.Lock()
	f.turns[c.ClaimID] = t
	f.mu.Unlock()
	f.turnsDone.Add(1)
	go func() {
		defer f.turnsDone.Done()
		defer close(t.done)
		defer cancel()
		out := f.runner.run(stop, turnInput{
			Dir:          dir,
			Session:      c.SessionID,
			Resume:       resume,
			Choice:       mc,
			AllowedTools: c.AllowedTools,
			SystemPrompt: c.SystemPrompt,
			Prompt:       prompt,
			UsageKind:    f.claimInfo(c).stage,
			OnEvent:      func(e core.SessionEvent) { f.events.add(c.SessionID, e) },
			OnUsage:      func(r core.UsageResponse) { f.usage.add(c.SessionID, r) },
			Linef: func(format string, args ...any) {
				f.claimf(c, "%s: %s", c.Ticket, fmt.Sprintf(format, args...))
			},
			Echo: func(toolCall string) { f.printf("%s: %s", c.Ticket, toolCall) },
			// Before the Turn starts, which may end the Claim before a
			// report after it could land.
			BeforeStart: func() { f.reportChoice(ctx, c, mc) },
		})
		if out.Kind == turnEffortRejected || out.StartErr != nil {
			f.freeSlot(c)
			if out.Kind == turnEffortRejected {
				f.reportRejectedEffort(ctx, c, dir, mc, out.Levels)
			}
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
