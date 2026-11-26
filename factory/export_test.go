package factory

import (
	"context"

	"github.com/tomoram/code-central/factory/harness"
)

// The Turn runner's tests drive the factorytest claude shim, and
// factorytest imports this package, so they are in package factory_test
// and reach the runner through these.

// TurnRunner is the Turn runner.
type TurnRunner = turnRunner

// TurnInput is a Turn's input.
type TurnInput = turnInput

// NewTurnRunner builds a Turn runner of h in e, as verbose says.
func NewTurnRunner(h harness.Harness, e Env, verbose bool) *TurnRunner {
	return newTurnRunner(h, e, verbose)
}

// Run runs a Turn, as run does.
func (r *turnRunner) Run(stop context.Context, in turnInput) harness.Outcome { return r.run(stop, in) }

// Choice is a Turn's model and effort.
func Choice(model, effort string) turnChoice { return turnChoice{model: model, effort: effort} }

// WakeRetry is the wake loop's pause after a next_factory_wake that
// failed.
const WakeRetry = wakeRetry
