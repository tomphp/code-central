// The Turn runner: one Turn of the Factory's Harness, a Claim's or a
// Chat's. It gives the Harness the Turn with its Model Choice, runs the
// Turn's process in its own process group, tells the Factory's work of
// it, and takes it through the stop ladder when its stop context ends.

package factory

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/factory/harness"
)

// turnRunner runs Turns of the Factory's Harness. It holds only what
// every Turn needs, none of the Factory's Claims, client or poster.
type turnRunner struct {
	harness harness.Harness
	// killGrace is how long a Turn being stopped has after each stop step
	// but the last (stopSteps).
	killGrace time.Duration
	verbose   bool
	// clock times the kill grace.
	clock clock.Clock
	// work hears of a stop's kill graces (Env.Work).
	work work
}

// newTurnRunner is the runner of h's Turns in e, with the Factory's
// --verbose.
func newTurnRunner(h harness.Harness, e Env, verbose bool) *turnRunner {
	clk := e.Clock
	if clk == nil {
		clk = clock.System{}
	}
	return &turnRunner{harness: h, killGrace: e.killGrace(), verbose: verbose, clock: clk, work: work{e.Work}}
}

// turnInput is one Turn: the Harness's Turn, its Model Choice, and how
// the Factory tells of it.
type turnInput struct {
	harness.Turn
	Choice turnChoice
	// Echo, if set, prints the tool calls echoed with verbose on, in
	// place of Linef: a Claim's go to stdout alone, as its Transcript has
	// them already as Session Events.
	Echo func(toolCall string)
	// Key names the Turn in its work (Work): its Ticket ID, or chat.
	Key string
	// Work, if set, is the Turn's line of work, busy as the Turn is
	// given: it rests while the Turn's process runs and is busy handling
	// the exit once it has exited. StopWork hands the runner the busy
	// work of a stop, which it ends once the first stop step is under way.
	Work     *stint
	StopWork handoff
}

// run runs the Turn in, stopping it when stop ends, and returns how it
// ended once its process has exited and its output has been read.
func (r *turnRunner) run(stop context.Context, in turnInput) harness.Outcome {
	in = in.withDefaults()
	t := in.Turn
	t.Model, t.Effort = in.Choice.model, in.Choice.effort
	if r.verbose {
		t.OnEvent = func(e api.SessionEvent) {
			in.OnEvent(e)
			if e.Kind == api.EventToolCall {
				in.Echo(e.Text)
			}
		}
	}
	t.Exec = r.exec(in)
	return r.harness.Run(stop, t)
}

// withDefaults is in with the Turn's defaults, Linef for an Echo it
// leaves out and a stint that tells nothing for a Work it leaves out.
func (in turnInput) withDefaults() turnInput {
	in.Turn = in.Turn.WithDefaults()
	if in.Echo == nil {
		linef := in.Linef
		in.Echo = func(toolCall string) { linef("%s", toolCall) }
	}
	if in.Work == nil {
		in.Work = &stint{}
	}
	return in
}

// exec runs in's process in its own process group, so the terminal's
// Ctrl-C reaches only the Factory, which drains rather than interrupting
// the Turn.
func (r *turnRunner) exec(in turnInput) harness.Exec {
	return func(stop context.Context, cmd *exec.Cmd, read func(io.Reader)) (harness.Exit, error) {
		startInOwnGroup(cmd)
		stdout, err := cmd.StdoutPipe()
		if err == nil {
			err = cmd.Start()
		}
		if err != nil {
			return harness.Exit{}, err
		}
		in.Work.rest(TurnRunning, in.Key, TurnRunningEnd)
		// streamRead is closed once the process has exited and its output
		// has been read; stopperDone once the goroutine stopping it has
		// returned.
		streamRead := make(chan struct{})
		stopperDone := make(chan struct{})
		go func() {
			defer close(stopperDone)
			select {
			case <-stop.Done():
				r.stop(cmd.Process, streamRead, in.Key, in.StopWork, func(err error) { in.Linef("can't stop the Turn: %v", err) })
			case <-streamRead:
			}
		}()
		read(stdout)
		exit := harness.Wait(cmd)
		in.Work.busy("handling the exit of turn ", in.Key)
		close(streamRead)
		<-stopperDone
		return exit, nil
	}
}

// stop takes a Turn's process proc through stopSteps until ended is
// closed, moving to the next step if it lingers past the kill grace and
// telling warn of a step that fails: on Unix SIGINT, then SIGTERM, then
// kill; on Windows Ctrl-Break, then kill. Each kill grace before the kill
// rests on the clock as the "kill grace of turn <key>"; the kill is busy
// until the process has gone. Once the first grace has begun it ends the
// stop's busy work stopWork hands it.
func (r *turnRunner) stop(proc *os.Process, ended <-chan struct{}, key string, stopWork handoff, warn func(error)) {
	s := stint{w: r.work}
	defer s.stop()
	defer stopWork.take()
	steps := stopSteps(proc)
	for i, step := range steps {
		if err := step(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			warn(err)
		}
		var grace <-chan time.Time
		if i < len(steps)-1 {
			grace = s.after(r.clock, r.killGrace, KillGrace, key)
		} else {
			s.busy("killing turn ", key)
			grace = r.clock.After(r.killGrace)
		}
		stopWork.take()
		select {
		case <-ended:
			return
		case <-grace:
		}
	}
	<-ended
}
