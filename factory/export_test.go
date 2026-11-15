package factory

import (
	"context"
	"io"
	"os/exec"
	"time"

	"github.com/tomoram/code-central/core"
)

// The Turn runner's tests drive the factorytest claude shim, and
// factorytest imports this package, so they are in package factory_test
// and reach the runner through these.

// TurnRunner is the Turn runner.
type TurnRunner = turnRunner

// TurnInput is a Turn's input.
type TurnInput = turnInput

// TurnOutcome is how a Turn ended.
type TurnOutcome = turnOutcome

// NewTurnRunner builds a Turn runner of claude in e, whose claude
// accepts the effort levels, as the Factory's flags bypass, claudeArgs
// and verbose say.
func NewTurnRunner(claude string, e Env, levels []string, bypass bool, claudeArgs []string, verbose bool) *TurnRunner {
	return newTurnRunner(claude, e, claudeAccepts{levels: levels}, bypass, claudeArgs, verbose)
}

// Run runs a Turn, as run does.
func (r *turnRunner) Run(stop context.Context, in turnInput) turnOutcome { return r.run(stop, in) }

// Choice is a Turn's model and effort.
func Choice(model, effort string) turnChoice { return turnChoice{model: model, effort: effort} }

// How is how the Turn ended, as Failure Reports and Chat say it.
func (t turnEnd) How() string { return t.how() }

// StartWaiting starts a child that waits, as startWaiting does.
var StartWaiting = startWaiting

// ReadTurn reads a started Turn's stream to its end, as the runner does.
func ReadTurn(cmd *exec.Cmd, stdout io.Reader, dir string) turnEnd {
	return readTurn(cmd, stdout, dir, newUsageReader("implementation", time.Now), func(core.SessionEvent) {}, func(core.UsageResponse) {})
}
