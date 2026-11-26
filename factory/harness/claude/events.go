package claude

import (
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tomoram/code-central/api"
)

// Session Events (ADR 0008). The Harness turns each Turn's stream-json
// into Session Events, which the Factory posts to Code Central so the
// Claim's Transcript fills as the agent works.

// toolResultMax is the most bytes of a tool result's text posted.
const toolResultMax = 4 << 10

// streamEvents maps one stream-json message to its Session Events, seen
// at now: an assistant message's non-empty text blocks and tool calls,
// and a user message's tool results. Everything else (thinking, system
// messages, the result, the prompts) is left out. tools is the turn's
// tool calls so far, tool-use ID to tool name, so a result names its
// tool; wt is the worktree, for the calls' summaries.
func streamEvents(msg streamMessage, now time.Time, tools map[string]string, wt string) []api.SessionEvent {
	var out []api.SessionEvent
	for _, c := range msg.Message.Content {
		switch {
		case msg.Type == "assistant" && c.Type == "text" && strings.TrimSpace(c.Text) != "":
			out = append(out, api.SessionEvent{Time: now, Kind: api.EventText, Text: c.Text})
		case msg.Type == "assistant" && c.Type == "tool_use":
			tools[c.ID] = c.Name
			out = append(out, api.SessionEvent{Time: now, Kind: api.EventToolCall, Text: toolCall(c.Name, c.Input, wt), Tool: c.Name, ToolUseID: c.ID})
		case msg.Type == "user" && c.Type == "tool_result":
			text, cut := cutResult(resultText(c.Content))
			out = append(out, api.SessionEvent{Time: now, Kind: api.EventToolResult, Text: text, Tool: tools[c.ToolUseID], ToolUseID: c.ToolUseID, IsError: c.IsError, Truncated: cut})
		}
	}
	return out
}

// resultText is a tool result's content as text: either a string, or the
// text of its text blocks, a line each.
func resultText(content json.RawMessage) string {
	var s string
	if json.Unmarshal(content, &s) == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	_ = json.Unmarshal(content, &blocks)
	var texts []string
	for _, b := range blocks {
		if b.Type == "text" {
			texts = append(texts, b.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// cutResult cuts text to toolResultMax bytes of whole runes, and says
// whether it cut it.
func cutResult(text string) (string, bool) {
	if len(text) <= toolResultMax {
		return text, false
	}
	n := toolResultMax
	for n > 0 && !utf8.RuneStart(text[n]) {
		n--
	}
	return text[:n], true
}
