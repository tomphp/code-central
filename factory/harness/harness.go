// Package harness is the seam between the Factory and its Harness, the
// coding-agent CLI it runs its Turns with (ADR 0077). Each Factory runs
// one Harness; the Factory gives it Turns and reads back only the
// neutral Outcome, never the Harness's own flags or output.
package harness

import (
	"context"
	"errors"
	"io"
	"os/exec"

	"github.com/tomoram/code-central/api"
)

// Harness runs Turns of one coding-agent CLI.
type Harness interface {
	// Run runs t to its end, stopping it when ctx ends, and says how it
	// ended once its process has exited and its output has been read.
	Run(ctx context.Context, t Turn) Outcome
}

// Turn is one run of a Harness: where and as what it runs, and where
// what it does goes. A callback left nil is a no-op.
type Turn struct {
	Dir string
	// AddDirs are the folders the Turn reaches beside Dir: the worktrees
	// of the Repositories other than the Home Repository.
	AddDirs []string
	Session string
	// Resume resumes Session rather than starting it.
	Resume bool
	// Model and Effort are the Turn's Model Choice; empty is the
	// Harness's own default.
	Model, Effort string
	AllowedTools  []string
	SystemPrompt  string
	Prompt        string
	// UsageKind is the kind the main session's model responses are
	// posted under: a Claim's Stage Guide name, or chat.
	UsageKind string
	// OnEvent gets each Session Event, in order, OnUsage each model
	// response's usage, and OnPlanLimits each reading of the Plan Limits.
	OnEvent      func(api.SessionEvent)
	OnUsage      func(api.UsageResponse)
	OnPlanLimits func(api.PlanLimits)
	// Linef prints a line about the Turn, such as why it couldn't start.
	Linef func(format string, args ...any)
	// BeforeStart is called once the Turn is sure to start, just before
	// its process does.
	BeforeStart func()
	// Exec runs the Turn's process; nil runs it plainly, killing it when
	// the Turn is stopped.
	Exec Exec
}

// Exec runs a Turn's process cmd: it starts it, hands its stdout to
// read, stops it when ctx ends, and returns how it exited once it has
// exited and read has returned. An error is one that kept it from
// starting.
type Exec func(ctx context.Context, cmd *exec.Cmd, read func(stdout io.Reader)) (Exit, error)

// Exit is how a Turn's process exited: its exit code, or the signal that
// ended it, as "signal: killed".
type Exit struct {
	Code   int
	Signal string
}

// WithDefaults is t with a no-op for each callback it leaves out, and
// the plain Exec for an Exec it leaves out.
func (t Turn) WithDefaults() Turn {
	if t.OnEvent == nil {
		t.OnEvent = func(api.SessionEvent) {}
	}
	if t.OnUsage == nil {
		t.OnUsage = func(api.UsageResponse) {}
	}
	if t.OnPlanLimits == nil {
		t.OnPlanLimits = func(api.PlanLimits) {}
	}
	if t.Linef == nil {
		t.Linef = func(string, ...any) {}
	}
	if t.BeforeStart == nil {
		t.BeforeStart = func() {}
	}
	if t.Exec == nil {
		t.Exec = plainExec
	}
	return t
}

// plainExec runs cmd with nothing around it, killing it when ctx ends.
func plainExec(ctx context.Context, cmd *exec.Cmd, read func(io.Reader)) (Exit, error) {
	stdout, err := cmd.StdoutPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		return Exit{}, err
	}
	stop := context.AfterFunc(ctx, func() { _ = cmd.Process.Kill() })
	defer stop()
	read(stdout)
	return Wait(cmd), nil
}

// Wait waits for cmd, started and its stdout read, and says how it
// exited.
func Wait(cmd *exec.Cmd) Exit {
	var exit *exec.ExitError
	switch err := cmd.Wait(); {
	case err == nil:
		return Exit{}
	case errors.As(err, &exit):
		if code := exit.ExitCode(); code != -1 {
			return Exit{Code: code}
		}
		return Exit{Code: -1, Signal: exit.ProcessState.String()}
	}
	return Exit{Code: -1}
}
