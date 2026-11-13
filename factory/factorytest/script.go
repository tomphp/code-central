// What the fake claude does on a run, its Script of Steps, and what the
// harness reads back: its Calls and the signals a Wait step noted.

package factorytest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tomoram/code-central/core"
)

// Step is one thing the fake claude does. In CLI and Git args, Write's
// content and Stdout, {{ticket}}, {{claim}} and {{session}} stand for the
// run's Ticket, Claim and session ID.
type Step struct {
	// CLI runs the code-central CLI with these args in the worktree.
	CLI []string `json:"cli,omitempty"`
	// Write writes a file in the worktree.
	Write *File `json:"write,omitempty"`
	// Git runs git on PATH with these args in the worktree.
	Git []string `json:"git,omitempty"`
	// Need fails the run unless this file is in the worktree.
	Need string `json:"need,omitempty"`
	// Stdout prints one line, such as a stream-json message.
	Stdout string `json:"stdout,omitempty"`
	// Exit exits with this code, skipping the rest.
	Exit *int `json:"exit,omitempty"`
	// Wait blocks until the test calls Continue for the run's Ticket. It
	// notes each stop signal it gets (see Signals): on Unix it ignores
	// SIGINT and exits 143 on SIGTERM; on Windows it ignores Ctrl-Break.
	Wait bool `json:"wait,omitempty"`
}

// File is a file a Write step writes, Name relative to the worktree.
type File struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// Wait blocks the run until Continue, so the turn stays running.
func Wait() Step { return Step{Wait: true} }

// PostRound posts a Round of one Question under the run's Claim.
func PostRound(title string) Step {
	q, _ := json.Marshal([]core.QuestionInput{{Title: title, Body: title}})
	return CLI("post-round", "{{ticket}}", "--claim", "{{claim}}", "--session", "{{session}}", "--questions", string(q))
}

// Continue lets the run of ticket now in a Wait step carry on.
func (h *Harness) Continue(ticket string) {
	h.t.Helper()
	must(h.t, os.WriteFile(filepath.Join(h.data, "continue", ticket), nil, 0o644))
	h.WaitFor(ticket+"'s run to carry on", func() bool {
		_, err := os.Stat(filepath.Join(h.data, "continue", ticket))
		return os.IsNotExist(err)
	})
}

// Signals is the signals runs of ticket got while in a Wait step, in
// order: "INT" and "TERM" on Unix, "BREAK" on Windows.
func (h *Harness) Signals(ticket string) []string {
	b, err := os.ReadFile(filepath.Join(h.data, "signals", ticket))
	if os.IsNotExist(err) {
		return nil
	}
	must(h.t, err)
	return strings.Fields(string(b))
}

// DefaultHelp is what the fake claude's --help prints unless ClaudeHelp
// says otherwise: the --effort and --model options as Claude Code
// 2.1.311 prints them, among others.
const DefaultHelp = `Usage: claude [options] [command] [prompt]

Claude Code - starts an interactive session by default, use -p/--print for
non-interactive output

Arguments:
  prompt                                Your prompt

Options:
  --effort <level>                      Effort level for the current session
                                        (low, medium, high, xhigh, max)
  -h, --help                            Display help for command
  --model <model>                       Model for the current session. Provide
                                        an alias for the latest model (e.g.
                                        'fable', 'opus', or 'sonnet') or a
                                        model's full name.
  -p, --print                           Print response and exit (useful for
                                        pipes).

Commands:
  mcp                                   Configure and manage MCP servers
`

// ClaudeHelp makes the fake claude's --help print help.
func (h *Harness) ClaudeHelp(help string) {
	h.t.Helper()
	must(h.t, os.WriteFile(filepath.Join(h.data, "help.txt"), []byte(help), 0o644))
}

// ClaudeRejects makes the fake claude name arg an unknown option when the
// Factory checks its Claude Args, as real claude does for a typo.
func (h *Harness) ClaudeRejects(arg string) {
	h.t.Helper()
	must(h.t, os.WriteFile(filepath.Join(h.data, "rejects.txt"), []byte(arg+"\n"), 0o644))
}

// ClaudeSays makes the fake claude print output, and exit 0, when the
// Factory checks its Claude Args.
func (h *Harness) ClaudeSays(output string) {
	h.t.Helper()
	must(h.t, os.WriteFile(filepath.Join(h.data, "says.txt"), []byte(output), 0o644))
}

// CLI calls the code-central CLI.
func CLI(args ...string) Step { return Step{CLI: args} }

// WriteFile writes name in the worktree with content.
func WriteFile(name, content string) Step { return Step{Write: &File{Name: name, Content: content}} }

// Git runs git with args in the worktree.
func Git(args ...string) Step { return Step{Git: args} }

// NeedFile fails the run unless name is in the worktree.
func NeedFile(name string) Step { return Step{Need: name} }

// Finish finishes the run's Ticket under its Claim.
func Finish(summary string) Step {
	return CLI("finish", "{{ticket}}", "--claim", "{{claim}}", "--summary", summary)
}

// FinishSpec finishes the run's spec Stage under its Claim with its Spec
// recommendation.
func FinishSpec(summary, recommendation string) Step {
	return CLI("finish", "{{ticket}}", "--claim", "{{claim}}", "--summary", summary, "--recommendation", recommendation)
}

// Result prints the stream-json result message with this subtype.
func Result(subtype string) Step {
	return Step{Stdout: `{"type":"result","subtype":"` + subtype + `","is_error":` + fmt.Sprint(subtype != "success") + `,"session_id":"{{session}}","result":"Turn over."}`}
}

// Denial is one call auto mode denied, as a result's permission_denials
// lists it.
type Denial struct {
	Tool  string         `json:"tool_name"`
	ID    string         `json:"tool_use_id"`
	Input map[string]any `json:"tool_input"`
}

// Denied is a denied Bash call running command.
func Denied(command string) Denial {
	return Denial{Tool: "Bash", ID: "toolu_" + fmt.Sprint(len(command)), Input: map[string]any{"command": command}}
}

// ResultDenied prints a success result listing these denied calls.
func ResultDenied(denials ...Denial) Step {
	b, _ := json.Marshal(map[string]any{
		"type": "result", "subtype": "success", "is_error": false, "session_id": "{{session}}",
		"result": "I couldn't do that.", "permission_denials": denials,
	})
	return Step{Stdout: string(b)}
}

// APIError prints the stream-json assistant message claude prints when an
// API call fails for good, with this error code ("rate_limit",
// "authentication_failed", ...).
func APIError(code string) Step {
	b, _ := json.Marshal(map[string]any{
		"type":    "assistant",
		"message": map[string]any{"role": "assistant", "content": []map[string]string{{"type": "text", "text": "API Error: " + code}}},
		"error":   code,
	})
	return Step{Stdout: string(b)}
}

// ModelNotFound prints what claude prints when the API rejects its model:
// an assistant message with error model_not_found, then an error result
// with terminal_reason api_error and api_error_status 404, as Claude Code
// 2.1.311 does.
func ModelNotFound(model string) []Step {
	text := fmt.Sprintf("There's an issue with the selected model (%s). It may not exist or you may not have access to it. Run /model to pick a different model.", model)
	msg, _ := json.Marshal(map[string]any{
		"type":    "assistant",
		"message": map[string]any{"role": "assistant", "content": []map[string]string{{"type": "text", "text": text}}},
		"error":   "model_not_found",
	})
	res, _ := json.Marshal(map[string]any{
		"type": "result", "subtype": "success", "is_error": true, "session_id": "{{session}}",
		"terminal_reason": "api_error", "api_error_status": 404, "result": text,
	})
	return []Step{{Stdout: string(msg)}, {Stdout: string(res)}, Exit(1)}
}

// Say prints a stream-json assistant message with this text.
func Say(text string) Step {
	b, _ := json.Marshal(map[string]any{
		"type":    "assistant",
		"message": map[string]any{"role": "assistant", "content": []map[string]string{{"type": "text", "text": text}}},
	})
	return Step{Stdout: string(b)}
}

// ToolUse prints a stream-json assistant message calling tool with input,
// a JSON object kept as written so its key order holds.
func ToolUse(tool, input string) Step {
	b, _ := json.Marshal(map[string]any{
		"type": "assistant",
		"message": map[string]any{"role": "assistant", "content": []map[string]any{
			{"type": "tool_use", "id": "toolu_" + tool, "name": tool, "input": json.RawMessage(input)},
		}},
	})
	return Step{Stdout: string(b)}
}

// Response prints a stream-json assistant message that is one content
// block of the model response id, carrying the response's model and
// usage, as claude prints each block of a response. parent is the Agent
// call a subagent's response runs under, "" for the main session's;
// model is left out of the message when "".
func Response(id, parent, model string, input, cacheWrite, cacheRead, output int) Step {
	message := map[string]any{
		"id": id, "role": "assistant",
		"content": []map[string]string{{"type": "thinking", "thinking": "Responding."}},
		"usage": map[string]int{
			"input_tokens": input, "cache_creation_input_tokens": cacheWrite,
			"cache_read_input_tokens": cacheRead, "output_tokens": output,
		},
	}
	if model != "" {
		message["model"] = model
	}
	msg := map[string]any{"type": "assistant", "message": message}
	if parent != "" {
		msg["parent_tool_use_id"] = parent
	}
	b, _ := json.Marshal(msg)
	return Step{Stdout: string(b)}
}

// AgentCall prints a stream-json assistant message calling the Agent tool
// with this tool-use ID and description, starting a subagent.
func AgentCall(id, description string) Step {
	b, _ := json.Marshal(map[string]any{
		"type": "assistant",
		"message": map[string]any{"role": "assistant", "content": []map[string]any{
			{"type": "tool_use", "id": id, "name": "Agent", "input": map[string]string{"description": description, "prompt": description}},
		}},
	})
	return Step{Stdout: string(b)}
}

// Think prints a stream-json assistant message with a thinking block.
func Think(thinking string) Step {
	b, _ := json.Marshal(map[string]any{
		"type":    "assistant",
		"message": map[string]any{"role": "assistant", "content": []map[string]string{{"type": "thinking", "thinking": thinking}}},
	})
	return Step{Stdout: string(b)}
}

// ToolResult prints a stream-json user message answering the tool call
// toolUseID with content, a string or a list of content blocks.
func ToolResult(toolUseID string, content any, isError bool) Step {
	b, _ := json.Marshal(map[string]any{
		"type": "user",
		"message": map[string]any{"role": "user", "content": []map[string]any{
			{"type": "tool_result", "tool_use_id": toolUseID, "content": content, "is_error": isError},
		}},
	})
	return Step{Stdout: string(b)}
}

// Exit exits with code.
func Exit(code int) Step { return Step{Exit: &code} }

// DefaultScript is what the fake does for a Ticket with no Script: finish
// the Ticket, then end the turn with a success result.
var DefaultScript = []Step{Finish("Done."), Result("success")}

// Script sets what the fake does on every run for ticket.
func (h *Harness) Script(ticket string, steps ...Step) {
	b, err := json.Marshal(steps)
	must(h.t, err)
	must(h.t, os.WriteFile(filepath.Join(h.data, "scripts", ticket+".json"), b, 0o644))
}

// Call is one run of the fake claude. Its Ticket and Claim are the
// prompt's Ticket: and Claim: lines, or else its system prompt's.
type Call struct {
	Args   []string `json:"args"`
	Dir    string   `json:"dir"`
	Prompt string   `json:"prompt"`
	// SystemPrompt is what the run adds to the system prompt
	// (--append-system-prompt), if anything.
	SystemPrompt string `json:"system_prompt"`
	Ticket       string `json:"ticket"`
	Claim        string `json:"claim"`
	Session      string `json:"session"`
}

// Flag is the value after flag in the call's args, and whether it is there.
func (c Call) Flag(flag string) (string, bool) {
	for i, a := range c.Args {
		if a == flag && i+1 < len(c.Args) {
			return c.Args[i+1], true
		}
	}
	return "", false
}

// Has reports whether arg is one of the call's args.
func (c Call) Has(arg string) bool {
	for _, a := range c.Args {
		if a == arg {
			return true
		}
	}
	return false
}

// Calls is every run of the fake claude so far, oldest first.
func (h *Harness) Calls() []Call {
	h.t.Helper()
	dir := filepath.Join(h.data, "calls")
	entries, err := os.ReadDir(dir)
	must(h.t, err)
	var out []Call
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		must(h.t, err)
		var c Call
		must(h.t, json.Unmarshal(b, &c))
		out = append(out, c)
	}
	return out
}
