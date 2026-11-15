// A Claim's turn: the flags that start or resume it, and running it in
// the worktree, taking a slot until it ends.

package factory

import (
	"context"
	"crypto/rand"
	"fmt"
	"os/exec"

	"github.com/tomoram/code-central/core"
)

// startArgs are a new session's flags.
func (f *factory) startArgs(c claim, mc turnChoice) []string {
	return append([]string{"-p", "--session-id", c.SessionID}, f.turnFlags(c, mc)...)
}

// resumeArgs are the flags that resume c's session.
func (f *factory) resumeArgs(c claim, mc turnChoice) []string {
	return append([]string{"-p", "--resume", c.SessionID}, f.turnFlags(c, mc)...)
}

// turnFlags are the flags every start and resume of c passes (flags).
func (f *factory) turnFlags(c claim, mc turnChoice) []string {
	return f.flags(mc, c.AllowedTools, c.SystemPrompt)
}

// flags are the flags every claude turn passes, as the Turn runner
// builds them.
func (f *factory) flags(mc turnChoice, allowedTools []string, systemPrompt string) []string {
	return f.runner.flags(mc, allowedTools, systemPrompt)
}

// startTurn runs `claude args...` in dir for c, taking a slot until it
// ends. args start the turn with mc, which is recorded on the Claim. An
// effort claude doesn't accept fails the Claim instead.
func (f *factory) startTurn(ctx context.Context, c claim, dir string, mc turnChoice, args []string) {
	if !f.effortAccepted(ctx, c, dir, mc) {
		return
	}
	// Before the turn starts, which may end the Claim before a report
	// after it could land.
	f.reportChoice(ctx, c, mc)
	cmd := exec.Command(f.claude, args...)
	cmd.Dir = dir
	cmd.Env = f.e.Environ
	cmd.Stderr = f.e.Stderr
	// Its own process group, so the terminal's Ctrl-C reaches only the
	// Factory, which drains rather than interrupting the turn.
	startInOwnGroup(cmd)
	stdout, err := cmd.StdoutPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		f.claimf(c, "%s: can't start claude: %v", c.Ticket, err)
		return
	}
	t := &turn{claim: c, dir: dir, choice: mc, proc: cmd.Process, started: f.e.Now(), done: make(chan struct{})}
	f.mu.Lock()
	f.turns[c.ClaimID] = t
	f.mu.Unlock()
	f.turnsDone.Add(1)
	go func() {
		defer f.turnsDone.Done()
		end := readTurn(cmd, stdout, dir, newUsageReader(f.claimInfo(c).stage, f.e.Now), func(e core.SessionEvent) {
			f.events.add(c.SessionID, e)
			if f.o.verbose && e.Kind == eventToolCall {
				f.printf("%s: %s", c.Ticket, e.Text)
			}
		}, func(r core.UsageResponse) { f.usage.add(c.SessionID, r) })
		ran := f.e.Now().Sub(t.started)
		// The turn keeps its slot until its outcome is acted on, and
		// "turn ended" is printed once the slot is free.
		state := f.afterTurn(ctx, t, end)
		f.mu.Lock()
		delete(f.turns, c.ClaimID)
		f.mu.Unlock()
		f.turnEnded(t, end, state, ran)
		f.forgetEnded(c)
		close(t.done)
	}()
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
