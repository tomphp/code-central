package factory

import (
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tomoram/code-central/core"
)

// Session Events (ADR 0008). The Factory turns each turn's stream-json
// and its own lines about a held Claim into Session Events and posts them
// to Code Central, so the Claim's Transcript fills as the agent works.
// Posting never blocks a turn and never fails one: events wait in a
// bounded queue, and a batch that can't be posted is dropped.

// The kinds of Session Event the Factory posts.
const (
	eventText       = "text"
	eventToolCall   = "tool_call"
	eventToolResult = "tool_result"
	eventFactory    = "factory"
)

// toolResultMax is the most bytes of a tool result's text posted.
const toolResultMax = 4 << 10

// streamEvents maps one stream-json message to its Session Events, seen
// at now: an assistant message's non-empty text blocks and tool calls,
// and a user message's tool results. Everything else (thinking, system
// messages, the result, the prompts) is left out. tools is the turn's
// tool calls so far, tool-use ID to tool name, so a result names its
// tool; wt is the worktree, for the calls' summaries.
func streamEvents(msg streamMessage, now time.Time, tools map[string]string, wt string) []core.SessionEvent {
	var out []core.SessionEvent
	for _, c := range msg.Message.Content {
		switch {
		case msg.Type == "assistant" && c.Type == "text" && strings.TrimSpace(c.Text) != "":
			out = append(out, core.SessionEvent{Time: now, Kind: eventText, Text: c.Text})
		case msg.Type == "assistant" && c.Type == "tool_use":
			tools[c.ID] = c.Name
			out = append(out, core.SessionEvent{Time: now, Kind: eventToolCall, Text: toolCall(c.Name, c.Input, wt), Tool: c.Name, ToolUseID: c.ID})
		case msg.Type == "user" && c.Type == "tool_result":
			text, cut := cutResult(resultText(c.Content))
			out = append(out, core.SessionEvent{Time: now, Kind: eventToolResult, Text: text, Tool: tools[c.ToolUseID], ToolUseID: c.ToolUseID, IsError: c.IsError, Truncated: cut})
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
