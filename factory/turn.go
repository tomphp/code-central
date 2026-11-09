// A turn's claude process: the flags that start or resume it, running it
// in the worktree, and reading the stream-json it prints until it ends.

package factory

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/tomoram/code-central/core"
)

// result is the stream-json result message that ends a turn.
type result struct {
	Subtype           string   `json:"subtype"`
	IsError           bool     `json:"is_error"`
	TerminalReason    string   `json:"terminal_reason"`
	PermissionDenials []denial `json:"permission_denials"`
	Result            string   `json:"result"`
}

// turnEnd is how a turn ended: its exit code, its result message if it
// printed one, and the agent's last text.
type turnEnd struct {
	ExitCode int
	Result   *result
	LastText string
	// APIError is the API error the turn ended on, such as "rate_limit"
	// or "authentication_failed"; empty when there was none.
	APIError string
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

// startArgs are a new session's flags.
func (f *factory) startArgs(c claim) []string {
	return append([]string{"-p", "--session-id", c.SessionID}, f.turnFlags(c)...)
}

// resumeArgs are the flags that resume c's session.
func (f *factory) resumeArgs(c claim) []string {
	return append([]string{"-p", "--resume", c.SessionID}, f.turnFlags(c)...)
}

// turnFlags are the flags every start and resume passes: the output
// format, the permission mode, the model when set, and the Claim's
// allowed tools.
func (f *factory) turnFlags(c claim) []string {
	args := []string{"--output-format", "stream-json", "--verbose"}
	if f.o.bypass {
		args = append(args, "--permission-mode", "bypassPermissions")
	} else {
		args = append(args, "--permission-mode", "auto", "--permission-prompts", "none")
	}
	if f.o.model != "" {
		args = append(args, "--model", f.o.model)
	}
	if len(c.AllowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(c.AllowedTools, ","))
	}
	return args
}

// startTurn runs `claude args...` in dir for c, taking a slot until it
// ends.
func (f *factory) startTurn(ctx context.Context, c claim, dir string, args []string) {
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
	t := &turn{claim: c, dir: dir, proc: cmd.Process, started: f.e.Now(), done: make(chan struct{})}
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

// streamMessage is the part of a stream-json message the Factory reads.
type streamMessage struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	// Error is an assistant message's API error, or an api_retry's.
	Error string `json:"error"`
	// ParentToolUseID is the Agent call a subagent's message runs under;
	// empty for the main session's.
	ParentToolUseID string `json:"parent_tool_use_id"`
	Message         struct {
		// ID and Usage are an assistant message's model response's.
		ID      string       `json:"id"`
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

// readTurn reads a started turn's stream-json to its end and waits for
// it, giving how it ended. onEvent is called with each Session Event the
// stream maps to, in order, and onResponse with each model response's
// usage, as usage reads them; dir is the turn's worktree.
func readTurn(cmd *exec.Cmd, stdout io.Reader, dir string, usage *usageReader, onEvent func(core.SessionEvent), onResponse func(core.UsageResponse)) turnEnd {
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
		for _, e := range streamEvents(msg, time.Now(), tools, dir) {
			if e.Kind == eventText {
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
	// A retry that later succeeded is no error; one the turn never got
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
