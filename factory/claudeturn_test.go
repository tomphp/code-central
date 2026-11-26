package factory_test

import (
	"context"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/factory"
	"github.com/tomoram/code-central/factory/factorytest"
)

// turnTest is a Turn runner of the rig's fake claude, and what its
// sinks and line function got.
type turnTest struct {
	h      *factorytest.Rig
	runner *factory.TurnRunner

	mu     sync.Mutex
	events []api.SessionEvent
	usage  []api.UsageResponse
	lines  []string
}

// defaultLevels are the effort levels factorytest.DefaultHelp lists.
var defaultLevels = []string{"low", "medium", "high", "xhigh", "max"}

// runnerOpts are the Factory's flags a test's runner is built with.
type runnerOpts struct {
	bypass     bool
	claudeArgs []string
	verbose    bool
}

func newTurnTest(t *testing.T, o runnerOpts) *turnTest {
	t.Helper()
	h := factorytest.New(t, factorytest.FakeGit())
	e := factory.Env{Environ: h.Environ(), Clock: h.Clock, KillGrace: factorytest.KillGrace}
	return &turnTest{h: h, runner: factory.NewTurnRunner(h.Claude(), e, defaultLevels, o.bypass, o.claudeArgs, o.verbose)}
}

// input is a Turn of id, whose script the fake runs, in the rig's
// repo, its sinks and line function recording what they get.
func (tt *turnTest) input(id string) factory.TurnInput {
	return factory.TurnInput{
		Dir:       tt.h.Repo,
		Session:   "11111111-2222-4333-8444-555555555555",
		Prompt:    "Do the work.\n\nTicket: " + id + "\n",
		UsageKind: "implementation",
		OnEvent: func(e api.SessionEvent) {
			tt.mu.Lock()
			defer tt.mu.Unlock()
			tt.events = append(tt.events, e)
		},
		OnUsage: func(r api.UsageResponse) {
			tt.mu.Lock()
			defer tt.mu.Unlock()
			tt.usage = append(tt.usage, r)
		},
		Linef: func(format string, args ...any) {
			tt.mu.Lock()
			defer tt.mu.Unlock()
			tt.lines = append(tt.lines, fmt.Sprintf(format, args...))
		},
	}
}

// run scripts id with steps and runs its Turn to its end.
func (tt *turnTest) run(id string, steps ...factorytest.Step) factory.TurnOutcome {
	tt.h.Script(id, steps...)
	return tt.runner.Run(context.Background(), tt.input(id))
}

func (tt *turnTest) gotLines() []string {
	tt.mu.Lock()
	defer tt.mu.Unlock()
	return slices.Clone(tt.lines)
}

func wantKind(t *testing.T, o factory.TurnOutcome, want string) {
	t.Helper()
	if string(o.Kind) != want {
		t.Fatalf("the Turn ended %q (%s, API error %q), want %q", o.Kind, o.End.How(), o.End.APIError, want)
	}
}

func TestATurnThatSucceedsIsDoneWithItsEventsAndLastText(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, runnerOpts{})

	o := tt.run("turn-1", factorytest.Say("All done."), factorytest.Result("success"))

	wantKind(t, o, "done")
	if o.End.LastText != "All done." {
		t.Errorf("the last text is %q, want %q", o.End.LastText, "All done.")
	}
	if !slices.ContainsFunc(tt.events, func(e api.SessionEvent) bool { return e.Kind == "text" && e.Text == "All done." }) {
		t.Errorf("the event sink got %+v, want the Say", tt.events)
	}
}

func TestATurnThatExitsWithNoResultFails(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, runnerOpts{})

	o := tt.run("turn-1", factorytest.Say("Hmm."), factorytest.Exit(1))

	wantKind(t, o, "failed")
	if got, want := o.End.How(), "exit 1, with no result message"; got != want {
		t.Errorf("the Turn ended %q, want %q", got, want)
	}
}

func TestATurnCutOffByAnOutageEndsInAnOutageHoweverItArrives(t *testing.T) {
	t.Parallel()
	for name, steps := range map[string][]factorytest.Step{
		"an API error on a message": {factorytest.APIError("rate_limit"), factorytest.Result("error_during_execution"), factorytest.Exit(1)},
		"a retry it never got past": {
			{Stdout: `{"type":"system","subtype":"api_retry","attempt":10,"error":"overloaded"}`},
			factorytest.Result("error_during_execution"), factorytest.Exit(1),
		},
		"the result's terminal reason alone": {
			{Stdout: `{"type":"result","subtype":"error_during_execution","is_error":true,"terminal_reason":"server_error"}`},
			factorytest.Exit(1),
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tt := newTurnTest(t, runnerOpts{})

			o := tt.run("turn-1", steps...)

			wantKind(t, o, "outage")
		})
	}
}

func TestATurnWhoseRetryLaterSucceededIsDone(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, runnerOpts{})

	o := tt.run("turn-1",
		factorytest.Step{Stdout: `{"type":"system","subtype":"api_retry","attempt":1,"error":"rate_limit"}`},
		factorytest.Say("Got there."), factorytest.Result("success"))

	wantKind(t, o, "done")
}

func TestAnExpiredLoginWinsOverASuccessResultAndTheExit(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, runnerOpts{})

	o := tt.run("turn-1", factorytest.APIError("authentication_failed"), factorytest.Result("success"), factorytest.Exit(1))

	wantKind(t, o, "login failed")
	if o.End.APIError != "authentication_failed" {
		t.Errorf("the raw end's API error is %q, want authentication_failed", o.End.APIError)
	}
}

func TestATurnWhoseModelClaudeRejectsEndsModelRejected(t *testing.T) {
	t.Parallel()
	for name, steps := range map[string][]factorytest.Step{
		"its message and result": factorytest.ModelNotFound("opus-9"),
		"its result alone": {
			{Stdout: `{"type":"result","subtype":"success","is_error":true,"terminal_reason":"api_error","api_error_status":404,"result":"no such model"}`},
			factorytest.Exit(1),
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tt := newTurnTest(t, runnerOpts{})

			o := tt.run("turn-1", steps...)

			wantKind(t, o, "model rejected")
		})
	}
}

func TestAnEffortClaudeDoesntAcceptStartsNothing(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, runnerOpts{})
	in := tt.input("turn-1")
	in.Choice = factory.Choice("", "extreme")
	hooked := false
	in.BeforeStart = func() { hooked = true }

	o := tt.runner.Run(context.Background(), in)

	wantKind(t, o, "effort rejected")
	if !slices.Equal(o.Levels, defaultLevels) {
		t.Errorf("the outcome has the levels %q, want %q", o.Levels, defaultLevels)
	}
	if hooked {
		t.Error("the before-start hook was called for a Turn that didn't start")
	}
	if calls := tt.h.Calls(); len(calls) != 0 {
		t.Errorf("claude ran: %+v", calls)
	}
}

func TestAStoppedTurnIsTakenThroughTheStopLadderAndEndsStopped(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, runnerOpts{})
	tt.h.Script("turn-1", factorytest.Say("Working."), factorytest.Wait(), factorytest.Result("success"))
	stop, cancel := context.WithCancel(context.Background())
	ran := make(chan factory.TurnOutcome)
	go func() { ran <- tt.runner.Run(stop, tt.input("turn-1")) }()
	tt.h.WaitFor("claude to start", func() bool { return len(tt.h.Calls()) == 1 })

	cancel()
	// It ignores the first stop step, so the runner waits out the kill
	// grace before the next. On Windows the next is a kill, so the fake
	// has to have noted the first before the grace ends.
	tt.h.WaitFor("the kill grace", func() bool {
		at, ok := tt.h.Clock.Next()
		return ok && at.Equal(tt.h.Clock.Now().Add(factorytest.KillGrace))
	})
	tt.h.WaitFor("the first stop signal noted", func() bool { return len(tt.h.Signals("turn-1")) == 1 })
	tt.h.Clock.Advance(factorytest.KillGrace)
	o := <-ran

	wantKind(t, o, "stopped")
	want := []string{"INT", "TERM"}
	if runtime.GOOS == "windows" {
		want = []string{"BREAK"}
	}
	if got := tt.h.Signals("turn-1"); !slices.Equal(got, want) {
		t.Errorf("the Turn got the signals %q, want %q", got, want)
	}
	// run returned only once claude had exited: its exit is on the end.
	if runtime.GOOS != "windows" && o.End.ExitCode != 143 {
		t.Errorf("the Turn ended %s, want the exit 143 of the SIGTERM", o.End.How())
	}
	if o.End.Result != nil {
		t.Errorf("a stopped Turn's end has the result %+v, want none", o.End.Result)
	}
}

func TestATurnWhoseClaudeCantStartFailsSayingWhy(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, runnerOpts{})
	tt.h.RemoveClaude()
	in := tt.input("turn-1")
	hooked := 0
	in.BeforeStart = func() { hooked++ }

	o := tt.runner.Run(context.Background(), in)

	wantKind(t, o, "failed")
	if o.StartErr == nil {
		t.Error("the outcome has no start error")
	}
	if lines := tt.gotLines(); len(lines) != 1 || !strings.HasPrefix(lines[0], "can't start claude: ") {
		t.Errorf("the line function got %q, want one \"can't start claude\" line", lines)
	}
	if hooked != 1 {
		t.Errorf("the before-start hook was called %d times, want once", hooked)
	}
}

func TestATurnPassesItsFlagsThenTheChoiceThenTheClaudeArgsThenThePrompt(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		resume     bool
		bypass     bool
		sessionArg string
		mode       []string
	}{
		"start":                         {sessionArg: "--session-id", mode: []string{"--permission-mode", "auto", "--permission-prompts", "none"}},
		"resume, bypassing permissions": {resume: true, bypass: true, sessionArg: "--resume", mode: []string{"--permission-mode", "bypassPermissions"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tt := newTurnTest(t, runnerOpts{bypass: tc.bypass, claudeArgs: []string{"--chrome", "--add-dir", "../other"}})
			tt.h.Script("turn-1", factorytest.Result("success"))
			in := tt.input("turn-1")
			in.Resume = tc.resume
			in.Choice = factory.Choice("opus", "high")
			in.AllowedTools = []string{"Bash(git:*)", "Read"}
			in.SystemPrompt = "You work on Tickets."

			wantKind(t, tt.runner.Run(context.Background(), in), "done")

			calls := tt.h.Calls()
			if len(calls) != 1 {
				t.Fatalf("claude ran %d times, want once", len(calls))
			}
			want := append([]string{"-p", tc.sessionArg, in.Session, "--output-format", "stream-json", "--verbose"}, tc.mode...)
			want = append(want, "--model", "opus", "--effort", "high",
				"--allowedTools", "Bash(git:*),Read", "--append-system-prompt", "You work on Tickets.",
				"--chrome", "--add-dir", "../other", "--", in.Prompt)
			if got := calls[0].Args; !slices.Equal(got, want) {
				t.Errorf("claude got the args\n%q\nwant\n%q", got, want)
			}
		})
	}
}

func TestATurnWithNoChoiceToolsOrSystemPromptPassesNoneOfTheirFlags(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, runnerOpts{})

	tt.run("turn-1", factorytest.Result("success"))

	c := tt.h.Calls()[0]
	for _, flag := range []string{"--model", "--effort", "--allowedTools", "--append-system-prompt"} {
		if c.Has(flag) {
			t.Errorf("claude got %s: %q", flag, c.Args)
		}
	}
	if got := c.Args[len(c.Args)-2]; got != "--" {
		t.Errorf("the prompt follows %q, want --", got)
	}
}

func TestATurnsUsageReachesTheUsageSinkWithItsKind(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, runnerOpts{})
	tt.h.Script("turn-1", factorytest.Response("msg_1", "", "", 100, 200, 300, 10), factorytest.Result("success"))
	in := tt.input("turn-1")
	in.UsageKind = "chat"

	tt.runner.Run(context.Background(), in)

	if len(tt.usage) != 1 || tt.usage[0].Kind != "chat" || tt.usage[0].OutputTokens != 10 {
		t.Errorf("the usage sink got %+v, want one chat response with 10 output tokens", tt.usage)
	}
}

func TestATurnsDenialsComeBackRawOnItsEnd(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, runnerOpts{})

	o := tt.run("turn-1", factorytest.ResultDenied(factorytest.Denied("rm -rf build")))

	wantKind(t, o, "done")
	if d := o.End.Result.PermissionDenials; len(d) != 1 || d[0].Tool != "Bash" {
		t.Errorf("the end has the denials %+v, want the one Bash call", d)
	}
}

func TestVerboseEchoesEachToolCallThroughTheLineFunction(t *testing.T) {
	t.Parallel()
	for name, verbose := range map[string]bool{"verbose": true, "not verbose": false} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tt := newTurnTest(t, runnerOpts{verbose: verbose})

			tt.run("turn-1", factorytest.ToolUse("Bash", `{"command":"go test ./..."}`), factorytest.Result("success"))

			var want []string
			if verbose {
				want = []string{"Bash: go test ./..."}
			}
			if got := tt.gotLines(); !slices.Equal(got, want) {
				t.Errorf("the line function got %q, want %q", got, want)
			}
		})
	}
}

func TestVerboseEchoesToolCallsThroughEchoInPlaceOfTheLineFunctionWhenSet(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, runnerOpts{verbose: true})
	tt.h.Script("turn-1", factorytest.ToolUse("Bash", `{"command":"go test ./..."}`), factorytest.Result("success"))
	in := tt.input("turn-1")
	var echoed []string
	in.Echo = func(toolCall string) { echoed = append(echoed, toolCall) }

	tt.runner.Run(context.Background(), in)

	if want := []string{"Bash: go test ./..."}; !slices.Equal(echoed, want) {
		t.Errorf("Echo got %q, want %q", echoed, want)
	}
	if got := tt.gotLines(); len(got) != 0 {
		t.Errorf("the line function got %q, want nothing", got)
	}
}

func TestTheBeforeStartHookRunsOnceBeforeClaudeStarts(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, runnerOpts{})
	tt.h.Script("turn-1", factorytest.Result("success"))
	in := tt.input("turn-1")
	var callsBefore []int
	in.BeforeStart = func() { callsBefore = append(callsBefore, len(tt.h.Calls())) }

	wantKind(t, tt.runner.Run(context.Background(), in), "done")

	if !slices.Equal(callsBefore, []int{0}) {
		t.Errorf("the hook saw these counts of claude runs %v, want it called once, before claude ran", callsBefore)
	}
}

func TestATurnKilledBySignalSaysWhichSignal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no signals; a killed process exits 1")
	}
	t.Parallel()
	cmd, stdout := factory.StartWaiting(t)
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}

	end := factory.ReadTurn(cmd, stdout, t.TempDir())
	if got, want := end.How(), "signal: killed, with no result message"; got != want {
		t.Errorf("the turn ended %q, want %q", got, want)
	}
}
