package claude_test

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/factory/harness"
	"github.com/tomoram/code-central/factory/harness/claude"
)

func TestMain(m *testing.M) { factorytest.Main(m) }

// turnTest is the Claude Code Harness over the rig's fake claude, and
// what a Turn's callbacks got.
type turnTest struct {
	h       *factorytest.Rig
	harness *claude.Harness
	lines   factorytest.Lines

	mu         sync.Mutex
	events     []api.SessionEvent
	usage      []api.UsageResponse
	planLimits []api.PlanLimits
}

// defaultLevels are the effort levels factorytest.DefaultHelp lists.
var defaultLevels = []string{"low", "medium", "high", "xhigh", "max"}

// options are the Factory's flags a test's Harness is built with.
type options struct {
	bypass bool
	args   []string
}

func newTurnTest(t *testing.T, o options) *turnTest {
	t.Helper()
	h := factorytest.New(t, factorytest.FakeGit())
	return &turnTest{h: h, harness: claude.New(claude.Options{
		Path: h.Claude(), Environ: h.Environ(), Clock: h.Clock,
		Efforts: defaultLevels, Bypass: o.bypass, Args: o.args,
	})}
}

// turn is a Turn of id, whose script the fake runs, in the rig's repo,
// its callbacks recording what they get.
func (tt *turnTest) turn(id string) harness.Turn {
	t := tt.h.Turn(id, &tt.lines)
	t.UsageKind = "implementation"
	t.OnEvent = func(e api.SessionEvent) {
		tt.mu.Lock()
		defer tt.mu.Unlock()
		tt.events = append(tt.events, e)
	}
	t.OnUsage = func(r api.UsageResponse) {
		tt.mu.Lock()
		defer tt.mu.Unlock()
		tt.usage = append(tt.usage, r)
	}
	t.OnPlanLimits = func(pl api.PlanLimits) {
		tt.mu.Lock()
		defer tt.mu.Unlock()
		tt.planLimits = append(tt.planLimits, pl)
	}
	return t
}

// run scripts id with steps and runs its Turn to its end.
func (tt *turnTest) run(id string, steps ...factorytest.Step) harness.Outcome {
	tt.h.Script(id, steps...)
	return tt.harness.Run(context.Background(), tt.turn(id))
}

func wantKind(t *testing.T, o harness.Outcome, want harness.Kind) {
	t.Helper()
	if o.Kind != want {
		t.Fatalf("the Turn ended %q (%s, error class %q), want %q", o.Kind, o.ExitDetail, o.ErrorClass, want)
	}
}

func TestATurnThatSucceedsIsDoneWithItsEventsAndLastText(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, options{})

	o := tt.run("turn-1", factorytest.Say("All done."), factorytest.Result("success"))

	wantKind(t, o, harness.Done)
	if o.LastText != "All done." {
		t.Errorf("the last text is %q, want %q", o.LastText, "All done.")
	}
	if !o.Ended || o.ExitSummary != "exit 0, success" {
		t.Errorf("the Turn ended %q (ended: %t), want exit 0, success, ended", o.ExitSummary, o.Ended)
	}
	if !slices.ContainsFunc(tt.events, func(e api.SessionEvent) bool { return e.Kind == "text" && e.Text == "All done." }) {
		t.Errorf("the event sink got %+v, want the Say", tt.events)
	}
}

func TestATurnThatExitsWithNoResultFails(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, options{})

	o := tt.run("turn-1", factorytest.Say("Hmm."), factorytest.Exit(1))

	wantKind(t, o, harness.Failed)
	if o.Ended {
		t.Error("the Turn has ended its session, want no end")
	}
	if got, want := o.ExitSummary, "exit 1, no result"; got != want {
		t.Errorf("the Turn ended %q, want %q", got, want)
	}
	if got, want := o.ExitDetail, "exit 1, with no result message"; got != want {
		t.Errorf("the Turn ended %q in full, want %q", got, want)
	}
}

func TestATurnCutOffByAnOutageEndsInAnOutageHoweverItArrives(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		steps []factorytest.Step
		class string
	}{
		"an API error on a message": {
			steps: []factorytest.Step{factorytest.APIError("rate_limit"), factorytest.Result("error_during_execution"), factorytest.Exit(1)},
			class: "rate_limit",
		},
		"a retry it never got past": {
			steps: []factorytest.Step{
				{Stdout: `{"type":"system","subtype":"api_retry","attempt":10,"error":"overloaded"}`},
				factorytest.Result("error_during_execution"), factorytest.Exit(1),
			},
			class: "overloaded",
		},
		"the result's terminal reason alone": {
			steps: []factorytest.Step{
				{Stdout: `{"type":"result","subtype":"error_during_execution","is_error":true,"terminal_reason":"server_error"}`},
				factorytest.Exit(1),
			},
			class: "server_error",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tt := newTurnTest(t, options{})

			o := tt.run("turn-1", tc.steps...)

			wantKind(t, o, harness.Outage)
			if o.ErrorClass != tc.class {
				t.Errorf("the error class is %q, want %q", o.ErrorClass, tc.class)
			}
		})
	}
}

func TestATurnWhoseRetryLaterSucceededIsDone(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, options{})

	o := tt.run("turn-1",
		factorytest.Step{Stdout: `{"type":"system","subtype":"api_retry","attempt":1,"error":"rate_limit"}`},
		factorytest.Say("Got there."), factorytest.Result("success"))

	wantKind(t, o, harness.Done)
	if o.ErrorClass != "" {
		t.Errorf("the error class is %q, want none", o.ErrorClass)
	}
}

func TestAnExpiredLoginWinsOverASuccessResultAndTheExit(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, options{})

	o := tt.run("turn-1", factorytest.APIError("authentication_failed"), factorytest.Result("success"), factorytest.Exit(1))

	wantKind(t, o, harness.LoginFailed)
	if o.ErrorClass != "authentication_failed" {
		t.Errorf("the error class is %q, want authentication_failed", o.ErrorClass)
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
			tt := newTurnTest(t, options{})

			o := tt.run("turn-1", steps...)

			wantKind(t, o, harness.ModelRejected)
			if o.ErrorClass != "model_not_found" {
				t.Errorf("the error class is %q, want model_not_found", o.ErrorClass)
			}
		})
	}
}

func TestATurnEndsWithTheResultsTextAsItsEndText(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, options{})

	o := tt.run("turn-1", factorytest.Step{Stdout: `{"type":"result","subtype":"success","is_error":false,"result":"Answered."}`})

	if o.EndText != "Answered." || o.LastText != "Answered." {
		t.Errorf("the Turn ended with the end text %q and last text %q, want the result's text for both", o.EndText, o.LastText)
	}
}

func TestAnEffortClaudeDoesntAcceptStartsNothing(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, options{})
	turn := tt.turn("turn-1")
	turn.Effort = "extreme"
	hooked := false
	turn.BeforeStart = func() { hooked = true }

	o := tt.harness.Run(context.Background(), turn)

	wantKind(t, o, harness.EffortRejected)
	if !slices.Equal(o.Efforts, defaultLevels) {
		t.Errorf("the outcome has the efforts %q, want %q", o.Efforts, defaultLevels)
	}
	if hooked {
		t.Error("the before-start hook was called for a Turn that didn't start")
	}
	if calls := tt.h.Calls(); len(calls) != 0 {
		t.Errorf("claude ran: %+v", calls)
	}
}

func TestAStoppedTurnEndsStoppedSayingWhatEndedIt(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, options{})
	tt.h.Script("turn-1", factorytest.Say("Working."), factorytest.Wait(), factorytest.Result("success"))
	stop, cancel := context.WithCancel(context.Background())
	ran := make(chan harness.Outcome)
	go func() { ran <- tt.harness.Run(stop, tt.turn("turn-1")) }()
	tt.h.WaitFor("claude to start", func() bool { return len(tt.h.Calls()) == 1 })

	cancel()
	o := <-ran

	wantKind(t, o, harness.Stopped)
	if runtime.GOOS != "windows" && o.ExitDetail != "signal: killed, with no result message" {
		t.Errorf("the Turn ended %q, want killed, with no result message", o.ExitDetail)
	}
}

func TestATurnWhoseClaudeCantStartFailsSayingWhy(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, options{})
	tt.h.RemoveClaude()
	turn := tt.turn("turn-1")
	hooked := 0
	turn.BeforeStart = func() { hooked++ }

	o := tt.harness.Run(context.Background(), turn)

	wantKind(t, o, harness.Failed)
	if o.StartErr == nil {
		t.Error("the outcome has no start error")
	}
	if o.ExitSummary != "" || o.ExitDetail != "" {
		t.Errorf("a Turn that never started ended %q (%q), want neither", o.ExitSummary, o.ExitDetail)
	}
	if lines := tt.lines.Got(); len(lines) != 1 || !strings.HasPrefix(lines[0], "can't start claude: ") {
		t.Errorf("the line function got %q, want one \"can't start claude\" line", lines)
	}
	if hooked != 1 {
		t.Errorf("the before-start hook was called %d times, want once", hooked)
	}
}

func TestATurnRunsItsProcessThroughItsExecAndReadsWhatItHandsBack(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, options{})
	turn := tt.turn("turn-1")
	var argv []string
	turn.Exec = func(_ context.Context, cmd *exec.Cmd, read func(io.Reader)) (harness.Exit, error) {
		argv = cmd.Args[1:]
		read(strings.NewReader(`{"type":"result","subtype":"success","is_error":false,"result":"ok"}` + "\n"))
		return harness.Exit{Code: -1, Signal: "signal: terminated"}, nil
	}

	o := tt.harness.Run(context.Background(), turn)

	wantKind(t, o, harness.Failed)
	if o.ExitSummary != "signal: terminated, success" {
		t.Errorf("the Turn ended %q, want signal: terminated, success", o.ExitSummary)
	}
	if len(argv) == 0 || argv[0] != "-p" {
		t.Errorf("the Exec got the args %q, want claude's", argv)
	}
	if calls := tt.h.Calls(); len(calls) != 0 {
		t.Errorf("claude ran outside the Exec: %+v", calls)
	}
}

func TestATurnWhoseExecCantStartItFailsSayingWhy(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, options{})
	turn := tt.turn("turn-1")
	turn.Exec = func(context.Context, *exec.Cmd, func(io.Reader)) (harness.Exit, error) {
		return harness.Exit{}, errors.New("no slot")
	}

	o := tt.harness.Run(context.Background(), turn)

	wantKind(t, o, harness.Failed)
	if o.StartErr == nil || o.StartErr.Error() != "no slot" {
		t.Errorf("the start error is %v, want no slot", o.StartErr)
	}
	if want := []string{"can't start claude: no slot"}; !slices.Equal(tt.lines.Got(), want) {
		t.Errorf("the line function got %q, want %q", tt.lines.Got(), want)
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
			tt := newTurnTest(t, options{bypass: tc.bypass, args: []string{"--chrome", "--add-dir", "../other"}})
			tt.h.Script("turn-1", factorytest.Result("success"))
			turn := tt.turn("turn-1")
			turn.Resume = tc.resume
			turn.Model, turn.Effort = "opus", "high"
			turn.AllowedTools = []string{"Bash(git:*)", "Read"}
			turn.SystemPrompt = "You work on Tickets."
			turn.AddDirs = []string{"../spec"}

			wantKind(t, tt.harness.Run(context.Background(), turn), harness.Done)

			calls := tt.h.Calls()
			if len(calls) != 1 {
				t.Fatalf("claude ran %d times, want once", len(calls))
			}
			want := append([]string{"-p", tc.sessionArg, turn.Session, "--output-format", "stream-json", "--verbose"}, tc.mode...)
			want = append(want, "--model", "opus", "--effort", "high",
				"--allowedTools", "Bash(git:*),Read", "--append-system-prompt", "You work on Tickets.", "--add-dir", "../spec",
				"--chrome", "--add-dir", "../other", "--", turn.Prompt)
			if got := calls[0].Args; !slices.Equal(got, want) {
				t.Errorf("claude got the args\n%q\nwant\n%q", got, want)
			}
		})
	}
}

func TestATurnWithNoChoiceToolsSystemPromptOrAddedDirsPassesNoneOfTheirFlags(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, options{})

	tt.run("turn-1", factorytest.Result("success"))

	c := tt.h.Calls()[0]
	for _, flag := range []string{"--model", "--effort", "--allowedTools", "--append-system-prompt", "--add-dir"} {
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
	tt := newTurnTest(t, options{})
	tt.h.Script("turn-1", factorytest.Response("msg_1", "", "", 100, 200, 300, 10), factorytest.Result("success"))
	turn := tt.turn("turn-1")
	turn.UsageKind = "chat"

	tt.harness.Run(context.Background(), turn)

	if len(tt.usage) != 1 || tt.usage[0].Kind != "chat" || tt.usage[0].OutputTokens != 10 {
		t.Errorf("the usage sink got %+v, want one chat response with 10 output tokens", tt.usage)
	}
}

func TestATurnsDenialsComeBackWithTheirToolAndInput(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, options{})

	o := tt.run("turn-1", factorytest.ResultDenied(factorytest.Denied("rm -rf build")))

	wantKind(t, o, harness.Done)
	if d := o.Denials; len(d) != 1 || d[0].Tool != "Bash" || !strings.Contains(string(d[0].Input), "rm -rf build") {
		t.Errorf("the outcome has the denials %+v, want the one Bash call", d)
	}
}

func TestACallAHookRefusedIsNotADenial(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, options{})
	refused := factorytest.Denied("python3 replace.py")

	o := tt.run("turn-1",
		factorytest.ToolResult(refused.ID, "PreToolUse:Bash hook error: Edit the file with the Edit tool", true),
		factorytest.ResultDenied(refused, factorytest.Denied("golangci-lint run")))

	if d := o.Denials; len(d) != 1 || !strings.Contains(string(d[0].Input), "golangci-lint run") {
		t.Errorf("the outcome has the denials %+v, want only the golangci-lint call", d)
	}
}

func TestATurnSaysWhetherItCalledTheCodeReviewSkill(t *testing.T) {
	t.Parallel()
	for skill, want := range map[string]bool{"mattpocock-skills:code-review": true, "code-review": true, "tdd": false} {
		t.Run(skill, func(t *testing.T) {
			t.Parallel()
			tt := newTurnTest(t, options{})

			o := tt.run("turn-1", factorytest.ToolUse("Skill", `{"skill":"`+skill+`"}`), factorytest.Result("success"))

			if o.ReviewCalled != want {
				t.Errorf("the outcome says the review was called: %t, want %t", o.ReviewCalled, want)
			}
		})
	}
}

func TestATurnsPlanLimitsReachThePlanLimitsSink(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, options{})
	resetsAt := tt.h.Clock.Now().Add(time.Hour).Unix()

	tt.run("turn-1", factorytest.RateLimitEvent(map[string]factorytest.RateLimitWindow{
		"five_hour": {Utilization: 0.42, ResetsAt: resetsAt},
	}), factorytest.Result("success"))

	if len(tt.planLimits) != 1 {
		t.Fatalf("the Plan Limits sink got %+v, want one reading", tt.planLimits)
	}
	pl := tt.planLimits[0]
	if w := pl.FiveHour; w == nil || w.UsedPercent != 42 || !w.ResetsAt.Equal(time.Unix(resetsAt, 0)) || pl.Weekly != nil {
		t.Errorf("the Plan Limits sink got %+v, want the 5-hour window 42%% used, resetting at %d, and no weekly", pl, resetsAt)
	}
}

func TestTheBeforeStartHookRunsOnceBeforeClaudeStarts(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, options{})
	tt.h.Script("turn-1", factorytest.Result("success"))
	turn := tt.turn("turn-1")
	var callsBefore []int
	turn.BeforeStart = func() { callsBefore = append(callsBefore, len(tt.h.Calls())) }

	wantKind(t, tt.harness.Run(context.Background(), turn), harness.Done)

	if !slices.Equal(callsBefore, []int{0}) {
		t.Errorf("the hook saw these counts of claude runs %v, want it called once, before claude ran", callsBefore)
	}
}
