package factory_test

import (
	"context"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/factory"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/factory/harness"
	"github.com/tomoram/code-central/factory/harness/claude"
)

// runnerTest is a Turn runner of the Claude Code Harness over the rig's
// fake claude, and what its line function got.
type runnerTest struct {
	h      *factorytest.Rig
	runner *factory.TurnRunner
	lines  factorytest.Lines
}

func newRunnerTest(t *testing.T, verbose bool) *runnerTest {
	t.Helper()
	h := factorytest.New(t, factorytest.FakeGit())
	e := factory.Env{Environ: h.Environ(), Clock: h.Clock, KillGrace: factorytest.KillGrace}
	claudeCode := claude.New(claude.Options{Path: h.Claude(), Environ: h.Environ(), Clock: h.Clock})
	return &runnerTest{h: h, runner: factory.NewTurnRunner(claudeCode, e, verbose)}
}

// input is the rig's Turn of id, its lines recorded.
func (rt *runnerTest) input(id string) factory.TurnInput {
	return factory.TurnInput{Turn: rt.h.Turn(id, &rt.lines)}
}

func TestAStoppedTurnIsTakenThroughTheStopLadderAndEndsStopped(t *testing.T) {
	t.Parallel()
	rt := newRunnerTest(t, false)
	rt.h.Script("turn-1", factorytest.Say("Working."), factorytest.Wait(), factorytest.Result("success"))
	stop, cancel := context.WithCancel(context.Background())
	ran := make(chan harness.Outcome)
	go func() { ran <- rt.runner.Run(stop, rt.input("turn-1")) }()
	rt.h.WaitFor("claude to start", func() bool { return len(rt.h.Calls()) == 1 })

	cancel()
	// It ignores the first stop step, so the runner waits out the kill
	// grace before the next. On Windows the next is a kill, so the fake
	// has to have noted the first before the grace ends.
	rt.h.WaitFor("the kill grace", func() bool {
		at, ok := rt.h.Clock.Next()
		return ok && at.Equal(rt.h.Clock.Now().Add(factorytest.KillGrace))
	})
	rt.h.WaitFor("the first stop signal noted", func() bool { return len(rt.h.Signals("turn-1")) == 1 })
	rt.h.Clock.Advance(factorytest.KillGrace)
	o := <-ran

	if o.Kind != harness.Stopped {
		t.Fatalf("the Turn ended %q (%s), want %q", o.Kind, o.ExitDetail, harness.Stopped)
	}
	want := []string{"INT", "TERM"}
	if runtime.GOOS == "windows" {
		want = []string{"BREAK"}
	}
	if got := rt.h.Signals("turn-1"); !slices.Equal(got, want) {
		t.Errorf("the Turn got the signals %q, want %q", got, want)
	}
	// run returned only once claude had exited: its exit is on the outcome.
	if runtime.GOOS != "windows" && !strings.HasPrefix(o.ExitSummary, "exit 143,") {
		t.Errorf("the Turn ended %s, want the exit 143 of the SIGTERM", o.ExitDetail)
	}
	if o.Ended {
		t.Errorf("a stopped Turn ended with a result (%s), want none", o.ExitSummary)
	}
}

func TestATurnStartsWithItsModelChoice(t *testing.T) {
	t.Parallel()
	rt := newRunnerTest(t, false)
	rt.h.Script("turn-1", factorytest.Result("success"))
	in := rt.input("turn-1")
	in.Choice = factory.Choice("opus", "high")

	rt.runner.Run(context.Background(), in)

	calls := rt.h.Calls()
	if len(calls) != 1 {
		t.Fatalf("claude ran %d times, want once", len(calls))
	}
	model, _ := calls[0].Flag("--model")
	effort, _ := calls[0].Flag("--effort")
	if model != "opus" || effort != "high" {
		t.Errorf("claude ran with --model %q --effort %q, want opus and high: %q", model, effort, calls[0].Args)
	}
}

func TestVerboseEchoesEachToolCallThroughTheLineFunction(t *testing.T) {
	t.Parallel()
	for name, verbose := range map[string]bool{"verbose": true, "not verbose": false} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rt := newRunnerTest(t, verbose)
			rt.h.Script("turn-1", factorytest.ToolUse("Bash", `{"command":"go test ./..."}`), factorytest.Result("success"))

			rt.runner.Run(context.Background(), rt.input("turn-1"))

			var want []string
			if verbose {
				want = []string{"Bash: go test ./..."}
			}
			if got := rt.lines.Got(); !slices.Equal(got, want) {
				t.Errorf("the line function got %q, want %q", got, want)
			}
		})
	}
}

func TestVerboseEchoesToolCallsThroughEchoInPlaceOfTheLineFunctionWhenSet(t *testing.T) {
	t.Parallel()
	rt := newRunnerTest(t, true)
	rt.h.Script("turn-1", factorytest.ToolUse("Bash", `{"command":"go test ./..."}`), factorytest.Result("success"))
	in := rt.input("turn-1")
	var echoed []string
	in.Echo = func(toolCall string) { echoed = append(echoed, toolCall) }

	rt.runner.Run(context.Background(), in)

	if want := []string{"Bash: go test ./..."}; !slices.Equal(echoed, want) {
		t.Errorf("Echo got %q, want %q", echoed, want)
	}
	if got := rt.lines.Got(); len(got) != 0 {
		t.Errorf("the line function got %q, want nothing", got)
	}
}
