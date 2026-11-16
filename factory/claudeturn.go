// The Turn runner: one run of claude, a Claim's or a Chat's, from its
// flags to how it ended. It starts claude in its own process group,
// stops it when its stop context ends, reads the stream-json it prints
// and classifies the end.

package factory

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
)

// turnRunner runs Turns of claude. It holds only what every Turn needs,
// none of the Factory's Claims, client or poster.
type turnRunner struct {
	claude  string
	environ []string
	stderr  io.Writer
	// accepts is what claude --help said claude accepts, read at startup.
	accepts    claudeAccepts
	bypass     bool
	claudeArgs []string
	// killGrace is how long a Turn being stopped has after each stop step
	// but the last (stopSteps).
	killGrace time.Duration
	verbose   bool
	// clock times the kill grace and Session Events, and dates model
	// responses.
	clock clock.Clock
	// work hears of a stop's kill graces (Env.Work).
	work work
}

// newTurnRunner is the runner of claude in e, which accepts what accepts
// says, with the Factory's --bypass-permissions, Claude Args and
// --verbose.
func newTurnRunner(claude string, e Env, accepts claudeAccepts, bypass bool, claudeArgs []string, verbose bool) *turnRunner {
	clk := e.Clock
	if clk == nil {
		clk = clock.System{}
	}
	return &turnRunner{claude: claude, environ: e.Environ, stderr: e.Stderr, accepts: accepts, bypass: bypass, claudeArgs: claudeArgs, killGrace: e.killGrace(), verbose: verbose, clock: clk, work: work{e.Work}}
}

// turnInput is one Turn: where and as what it runs, and where what it
// does goes.
type turnInput struct {
	Dir     string
	Session string
	// Resume resumes Session (--resume) rather than starting it
	// (--session-id).
	Resume       bool
	Choice       turnChoice
	AllowedTools []string
	SystemPrompt string
	Prompt       string
	// UsageKind is the kind its model responses are posted under: a
	// Claim's Stage Guide name, or kindChat.
	UsageKind string
	// OnEvent gets each Session Event, in order, OnUsage each model
	// response's usage, and OnPlanLimits the Plan Limits of each
	// rate_limit_event.
	OnEvent      func(core.SessionEvent)
	OnUsage      func(core.UsageResponse)
	OnPlanLimits func(core.PlanLimits)
	// Linef prints a line about the Turn: a warning, or with verbose on
	// a tool call unless Echo is set.
	Linef func(format string, args ...any)
	// Echo, if set, prints the tool calls echoed with verbose on, in
	// place of Linef: a Claim's go to stdout alone, as its Transcript has
	// them already as Session Events.
	Echo func(toolCall string)
	// BeforeStart, if set, is called once the effort has passed its
	// check, just before claude starts.
	BeforeStart func()
	// Key names the Turn in its work (Work): its Ticket ID, or chat.
	Key string
	// Work, if set, is the Turn's line of work, busy as the Turn is
	// given: it rests while claude runs and is busy handling the exit
	// once claude has exited. StopWork hands the runner the busy work of
	// a stop, which it ends once the first stop step is under way.
	Work     *stint
	StopWork handoff
}

// outcomeKind is how a Turn ended, classified.
type outcomeKind string

const (
	turnDone           outcomeKind = "done"
	turnFailed         outcomeKind = "failed"
	turnOutage         outcomeKind = "outage"
	turnLoginFailed    outcomeKind = "login failed"
	turnModelRejected  outcomeKind = "model rejected"
	turnEffortRejected outcomeKind = "effort rejected"
	turnStopped        outcomeKind = "stopped"
)

// turnOutcome is how a Turn ended: its kind, and the raw end the kind
// was read from.
type turnOutcome struct {
	Kind outcomeKind
	End  turnEnd
	// StartErr is why claude couldn't start, for a failed Turn that
	// never ran.
	StartErr error
	// Levels are the effort levels claude accepts, for an effort
	// rejected Turn.
	Levels []string
}

// run runs the Turn in, stopping it when stop ends, and returns how it
// ended once claude has exited and its stream has been read. An effort
// claude doesn't accept starts nothing.
func (r *turnRunner) run(stop context.Context, in turnInput) turnOutcome {
	if !r.accepts.takesEffort(in.Choice.effort) {
		return turnOutcome{Kind: turnEffortRejected, Levels: r.accepts.levels}
	}
	in = in.withDefaults()
	if in.BeforeStart != nil {
		in.BeforeStart()
	}
	cmd := exec.Command(r.claude, r.args(in)...)
	cmd.Dir = in.Dir
	cmd.Env = r.environ
	cmd.Stderr = r.stderr
	// Its own process group, so the terminal's Ctrl-C reaches only the
	// Factory, which drains rather than interrupting the Turn.
	startInOwnGroup(cmd)
	stdout, err := cmd.StdoutPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		in.Linef("can't start claude: %v", err)
		return turnOutcome{Kind: turnFailed, StartErr: err}
	}
	in.Work.rest(TurnRunning, in.Key, TurnRunningEnd)
	// streamRead is closed once claude has exited and its stream has been
	// read; stopperDone once the goroutine stopping it has returned.
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
	end := readTurn(cmd, stdout, in.Dir, r.clock.Now, newUsageReader(in.UsageKind, r.clock.Now), func(e core.SessionEvent) {
		in.OnEvent(e)
		if r.verbose && e.Kind == core.EventToolCall {
			in.Echo(e.Text)
		}
	}, in.OnUsage, in.OnPlanLimits)
	in.Work.busy("handling the exit of turn ", in.Key)
	close(streamRead)
	<-stopperDone
	return turnOutcome{Kind: end.kind(stop.Err() != nil), End: end}
}

// withDefaults is in with a no-op for each sink and line function it
// leaves out, and Linef for an Echo it leaves out.
func (in turnInput) withDefaults() turnInput {
	if in.OnEvent == nil {
		in.OnEvent = func(core.SessionEvent) {}
	}
	if in.OnUsage == nil {
		in.OnUsage = func(core.UsageResponse) {}
	}
	if in.OnPlanLimits == nil {
		in.OnPlanLimits = func(core.PlanLimits) {}
	}
	if in.Linef == nil {
		in.Linef = func(string, ...any) {}
	}
	if in.Echo == nil {
		linef := in.Linef
		in.Echo = func(toolCall string) { linef("%s", toolCall) }
	}
	if in.Work == nil {
		in.Work = &stint{}
	}
	return in
}

// args are claude's args for in: -p, the session to start or resume,
// the flags, then the prompt.
func (r *turnRunner) args(in turnInput) []string {
	session := "--session-id"
	if in.Resume {
		session = "--resume"
	}
	args := append([]string{"-p", session, in.Session}, r.flags(in.Choice, in.AllowedTools, in.SystemPrompt)...)
	return append(args, in.Prompt)
}

// flags are the flags every claude Turn passes, a Claim's or a Chat's:
// the output format, the permission mode, the Turn's model and effort
// when set, and the allowed tools and system prompt when there are any,
// then the Claude Args and a -- that ends the flags, so that a flag
// taking several values can't swallow the prompt that follows.
func (r *turnRunner) flags(mc turnChoice, allowedTools []string, systemPrompt string) []string {
	args := []string{"--output-format", "stream-json", "--verbose"}
	if r.bypass {
		args = append(args, "--permission-mode", "bypassPermissions")
	} else {
		args = append(args, "--permission-mode", "auto", "--permission-prompts", "none")
	}
	if mc.model != "" {
		args = append(args, "--model", mc.model)
	}
	if mc.effort != "" {
		args = append(args, "--effort", mc.effort)
	}
	if len(allowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(allowedTools, ","))
	}
	if systemPrompt != "" {
		args = append(args, "--append-system-prompt", systemPrompt)
	}
	args = append(args, r.claudeArgs...)
	return append(args, "--")
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

// result is the stream-json result message that ends a Turn.
type result struct {
	Subtype           string   `json:"subtype"`
	IsError           bool     `json:"is_error"`
	TerminalReason    string   `json:"terminal_reason"`
	APIErrorStatus    int      `json:"api_error_status"`
	PermissionDenials []denial `json:"permission_denials"`
	Result            string   `json:"result"`
}

// turnEnd is how a Turn ended: its exit code, its result message if it
// printed one, and the agent's last text.
type turnEnd struct {
	ExitCode int
	// Signal is the signal that ended the Turn, as "signal: killed";
	// empty when it exited by itself.
	Signal   string
	Result   *result
	LastText string
	// APIError is the API error the Turn ended on, such as "rate_limit",
	// "authentication_failed" or "model_not_found"; empty when there was
	// none.
	APIError string
}

func (t turnEnd) String() string {
	s := t.exit()
	if t.Result == nil {
		return s + ", no result"
	}
	s += ", " + t.Result.Subtype
	if t.Result.TerminalReason != "" {
		s += " (" + t.Result.TerminalReason + ")"
	}
	return s
}

// exit is how the Turn's process ended: the signal that ended it, else
// its exit code.
func (t turnEnd) exit() string {
	if t.Signal != "" {
		return t.Signal
	}
	return fmt.Sprintf("exit %d", t.ExitCode)
}

// how is how the Turn ended, as Failure Reports and Chat say it: its
// exit code or signal, subtype and terminal reason.
func (t turnEnd) how() string {
	s := t.exit()
	if t.Result == nil {
		return s + ", with no result message"
	}
	s += ", subtype " + t.Result.Subtype
	if t.Result.TerminalReason != "" {
		s += ", terminal_reason " + t.Result.TerminalReason
	}
	return s
}

// kind classifies the end, first match winning: stopped when its stop
// context had ended; then an expired login, a rejected model and an
// Outage, read from its API error before its exit and result; then done
// when it exited 0 with a result that isn't an error; else failed.
func (t turnEnd) kind(stopped bool) outcomeKind {
	switch apiErr := t.apiError(); {
	case stopped:
		return turnStopped
	case apiErr == authFailed:
		return turnLoginFailed
	case apiErr == modelNotFound:
		return turnModelRejected
	case outageErrors[apiErr]:
		return turnOutage
	case t.ExitCode == 0 && t.Signal == "" && t.Result != nil && !t.Result.IsError:
		return turnDone
	}
	return turnFailed
}

// outageErrors are the API errors of an Outage: the trouble is the
// service's or the account's limits, not the Ticket's: usage and rate
// limits, overload, server and network errors. A Claim's Turn cut off by
// one is resumed later, never failed.
//
// Source: docs/research/agent-sdk-host.md (ticket 50, on branch
// research/agent-sdk-host), section 5, names the stream-json
// `system/api_retry` event's `error` values `rate_limit`,
// `authentication_failed` and `billing_error` (the usage limit), from the
// headless docs. The other codes here (overload, server, network and
// timeout, and the `_error` spellings) are unverified: neither that note
// nor docs/research/claude-code-session-host.md lists them, nor the
// result's `terminal_reason` values.
var outageErrors = map[string]bool{
	"rate_limit":       true,
	"rate_limit_error": true,
	"billing_error":    true,
	"overloaded":       true,
	"overloaded_error": true,
	"server_error":     true,
	"api_error":        true,
	"network_error":    true,
	"connection_error": true,
	"timeout":          true,
}

// authFailed is the API error of an expired or missing login, which only
// the human can fix.
const authFailed = "authentication_failed"

// modelNotFound is the API error of a model claude rejects: the Turn's
// model, not the service or the Factory, is at fault.
const modelNotFound = "model_not_found"

// apiError is the API error the Turn ended on, from its messages or its
// result's terminal reason. An api_error with status 404 is a model
// claude rejects, even with no message saying so.
func (t turnEnd) apiError() string {
	if t.APIError != "" {
		return t.APIError
	}
	if t.Result != nil && t.Result.TerminalReason == "api_error" && t.Result.APIErrorStatus == 404 {
		return modelNotFound
	}
	if t.Result != nil && (outageErrors[t.Result.TerminalReason] || t.Result.TerminalReason == authFailed) {
		return t.Result.TerminalReason
	}
	return ""
}

// streamMessage is the part of a stream-json message the Factory reads.
type streamMessage struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	// Error is an assistant message's API error, or an api_retry's.
	Error string `json:"error"`
	// ParentToolUseID is the Agent call a subagent's message runs under;
	// empty for the main session's.
	ParentToolUseID string `json:"parent_tool_use_id"`
	// RateLimitInfo is a rate_limit_event's, as readPlanLimits reads
	// it; kept raw so a shape it doesn't expect never loses the line.
	RateLimitInfo json.RawMessage `json:"rate_limit_info"`
	Message       struct {
		// ID, Model and Usage are an assistant message's model
		// response's.
		ID      string       `json:"id"`
		Model   string       `json:"model"`
		Usage   *streamUsage `json:"usage"`
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
			// ToolUseID, Content and IsError are a tool result's.
			ToolUseID string          `json:"tool_use_id"`
			Content   json.RawMessage `json:"content"`
			IsError   bool            `json:"is_error"`
		} `json:"content"`
	} `json:"message"`
}

// readTurn reads a started Turn's stream-json to its end and waits for
// it, giving how it ended. onEvent is called with each Session Event the
// stream maps to, in order, and onResponse with each model response's
// usage, as usage reads them, and onLimits with the Plan Limits of each
// rate_limit_event; dir is the Turn's worktree, and now times each
// Session Event and dates each Plan Limits.
func readTurn(cmd *exec.Cmd, stdout io.Reader, dir string, now func() time.Time, usage *usageReader, onEvent func(core.SessionEvent), onResponse func(core.UsageResponse), onLimits func(core.PlanLimits)) turnEnd {
	var end turnEnd
	var retryErr string
	tools := map[string]string{}
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var msg streamMessage
		if json.Unmarshal(sc.Bytes(), &msg) != nil {
			continue
		}
		if r, ok := usage.read(msg); ok {
			onResponse(r)
		}
		if pl, ok := readPlanLimits(msg, now()); ok {
			onLimits(pl)
		}
		for _, e := range streamEvents(msg, now(), tools, dir) {
			if e.Kind == core.EventText {
				end.LastText = e.Text
			}
			onEvent(e)
		}
		switch msg.Type {
		case "system":
			if msg.Subtype == "api_retry" && msg.Error != "" {
				retryErr = msg.Error
			}
		case "assistant":
			if msg.Error != "" {
				end.APIError = msg.Error
			}
		case "result":
			var r result
			if json.Unmarshal(sc.Bytes(), &r) == nil {
				end.Result = &r
			}
		}
	}
	if end.LastText == "" && end.Result != nil {
		end.LastText = end.Result.Result
	}
	// A retry that later succeeded is no error; one the Turn never got
	// past is.
	if end.APIError == "" && (end.Result == nil || end.Result.IsError) {
		end.APIError = retryErr
	}
	err := cmd.Wait()
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		end.ExitCode = exit.ExitCode()
		if end.ExitCode == -1 {
			end.Signal = exit.ProcessState.String()
		}
	default:
		end.ExitCode = -1
	}
	return end
}
