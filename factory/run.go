package factory

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/tomoram/code-central/core"
)

// loop polls until ctx ends: the first poll is at once, the rest every
// --poll. When ctx ends, running turns are stopped and waited for.
func (f *factory) loop(ctx context.Context) {
	defer f.turns.Wait()
	for {
		f.poll(ctx)
		select {
		case <-ctx.Done():
			return
		case <-f.e.After(f.o.poll):
		}
	}
}

// poll claims and starts turns while a slot is free.
func (f *factory) poll(ctx context.Context) {
	for ctx.Err() == nil && f.running() < f.o.parallel {
		if !f.claimNext(ctx) {
			return
		}
	}
}

func (f *factory) running() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.nRunning
}

// claimNext claims the Project's next claimable Ticket and starts its
// turn. It reports whether it claimed one.
func (f *factory) claimNext(ctx context.Context) bool {
	session := newSessionID()
	resume := "claude --resume " + session
	// The worktree's path is fixed by the Ticket ID, which isn't known
	// until the claim, so the claim names it afterwards (set_session).
	res, err := f.client.Claim(ctx, f.project, core.ClaimInput{SessionID: session, ResumeCommand: &resume})
	if err != nil {
		if ctx.Err() == nil {
			f.printf("claim: %v", err)
		}
		return false
	}
	if res.Ticket == nil {
		return false
	}
	id := res.Ticket.ID
	stage := stageOf(*res.Ticket)
	guide := f.stageGuide(ctx, res.StageGuide, stage)
	c := claim{Ticket: id, ClaimID: res.ClaimID, SessionID: session, AllowedTools: []string{}}
	if err := f.addClaim(c); err != nil {
		f.printf("%s: can't write the state file: %v", id, err)
	}
	wt := filepath.Join(f.worktrees, id)
	if _, err := f.client.SetSession(ctx, f.project, core.SetSessionInput{Ticket: core.Ref(id), ClaimID: res.ClaimID, SessionID: session, WorkingDir: &wt, ResumeCommand: &resume}); err != nil {
		f.printf("%s: set_session: %v", id, err)
	}
	setUp := f.implementationWorktree
	if stage != "" {
		setUp = f.stageWorktree
	}
	if err := setUp(id, wt); err != nil {
		if fl := (*failure)(nil); errors.As(err, &fl) {
			f.reportFailure(ctx, c, fl)
			return true
		}
		f.printf("%s: can't set up the worktree: %v", id, err)
		return true
	}
	f.mu.Lock()
	f.nRunning++
	f.mu.Unlock()
	f.turns.Add(1)
	go func() {
		defer f.turns.Done()
		defer func() {
			f.mu.Lock()
			f.nRunning--
			f.mu.Unlock()
		}()
		end := f.runTurn(ctx, c, wt, firstPrompt(c, stage, guide))
		f.printf("%s: turn ended: %s", id, end)
	}()
	return true
}

// implementationWorktree gives implementation Ticket id its worktree at
// wt on branch id: reused when it is there, else added on the existing
// branch, else on a new branch from the base's tip.
func (f *factory) implementationWorktree(id, wt string) error {
	if _, err := os.Stat(wt); err == nil {
		if b, err := f.gitOut(wt, "symbolic-ref", "--short", "-q", "HEAD"); err == nil && b == id {
			return nil
		}
		return fmt.Errorf("%s is there but isn't a worktree on branch %s", wt, id)
	}
	if _, err := f.gitOut(f.repo, "rev-parse", "--verify", "-q", "refs/heads/"+id); err == nil {
		_, err := f.gitOut(f.repo, "worktree", "add", "-q", wt, id)
		return err
	}
	_, err := f.gitOut(f.repo, "worktree", "add", "-q", "-b", id, wt, f.base)
	return err
}

// result is the stream-json result message that ends a turn.
type result struct {
	Subtype           string            `json:"subtype"`
	IsError           bool              `json:"is_error"`
	TerminalReason    string            `json:"terminal_reason"`
	PermissionDenials []json.RawMessage `json:"permission_denials"`
	Result            string            `json:"result"`
}

// turnEnd is how a turn ended: its exit code and its result message, if
// it printed one.
type turnEnd struct {
	ExitCode int
	Result   *result
}

func (t turnEnd) String() string {
	s := fmt.Sprintf("exit %d", t.ExitCode)
	if t.Result == nil {
		return s + ", no result"
	}
	s += ", " + t.Result.Subtype
	if t.Result.TerminalReason != "" {
		s += " (" + t.Result.TerminalReason + ")"
	}
	return s
}

// claudeArgs are the flags every start passes.
func (f *factory) claudeArgs(c claim) []string {
	args := []string{"-p", "--session-id", c.SessionID, "--output-format", "stream-json", "--verbose"}
	if f.o.bypass {
		args = append(args, "--permission-mode", "bypassPermissions")
	} else {
		args = append(args, "--permission-mode", "auto", "--permission-prompts", "none")
	}
	if f.o.model != "" {
		args = append(args, "--model", f.o.model)
	}
	return args
}

// runTurn runs one `claude -p` turn in dir and reads how it ended. When
// ctx ends the turn gets SIGINT, then is killed if it lingers.
func (f *factory) runTurn(ctx context.Context, c claim, dir, prompt string) turnEnd {
	cmd := exec.CommandContext(ctx, f.claude, append(f.claudeArgs(c), prompt)...)
	cmd.Dir = dir
	cmd.Env = f.e.Environ
	cmd.Stderr = f.e.Stderr
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGINT) }
	cmd.WaitDelay = 5 * time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return turnEnd{ExitCode: -1}
	}
	if err := cmd.Start(); err != nil {
		f.printf("%s: can't start claude: %v", c.Ticket, err)
		return turnEnd{ExitCode: -1}
	}
	var end turnEnd
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var msg struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(sc.Bytes(), &msg) != nil || msg.Type != "result" {
			continue
		}
		var r result
		if json.Unmarshal(sc.Bytes(), &r) == nil {
			end.Result = &r
		}
	}
	err = cmd.Wait()
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		end.ExitCode = exit.ExitCode()
	default:
		end.ExitCode = -1
	}
	return end
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
