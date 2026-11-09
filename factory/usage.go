package factory

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/usage"
)

// Token usage (Spec code-central-201). The Factory reads each model
// response's usage from a turn's stream-json and posts it to Code
// Central's POST /api/usage, by the Session Events rules: the same
// sender, so a post that fails, or an older Code Central's 404, is
// dropped and a turn is never slowed or failed.

// The kinds of session a subagent's responses are posted under; the main
// session's kind is its Stage.
const (
	kindReview   = "review"
	kindSubagent = "subagent"
)

// reviewPrefixes start the description of an Agent call that is a review,
// as the Context Report reads them.
var reviewPrefixes = []string{"Spec review", "Standards review"}

// streamUsage is a stream-json assistant message's usage, as the API
// names its counts.
type streamUsage struct {
	InputTokens              int `json:"input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	OutputTokens             int `json:"output_tokens"`
}

// usageReader reads one turn's model responses. kind is the main
// session's kind, the Stage the Factory started; now dates each response.
type usageReader struct {
	kind   string
	now    func() time.Time
	seen   map[string]bool   // message IDs already read
	agents map[string]string // Agent calls' descriptions, by tool-use ID
}

func newUsageReader(kind string, now func() time.Time) *usageReader {
	return &usageReader{kind: kind, now: now, seen: map[string]bool{}, agents: map[string]string{}}
}

// read gives msg's model response, dated when it is read, the first
// time its message ID comes: claude prints a response with several
// content blocks as several lines, each with the same ID and usage. It
// also remembers msg's Agent calls, so their subagents' responses get
// their kind.
func (u *usageReader) read(msg streamMessage) (core.UsageResponse, bool) {
	if msg.Type != "assistant" {
		return core.UsageResponse{}, false
	}
	for _, c := range msg.Message.Content {
		if c.Type == "tool_use" && (c.Name == "Agent" || c.Name == "Task") {
			var in struct {
				Description string `json:"description"`
			}
			_ = json.Unmarshal(c.Input, &in)
			u.agents[c.ID] = in.Description
		}
	}
	m := msg.Message
	if m.ID == "" || m.Usage == nil || u.seen[m.ID] {
		return core.UsageResponse{}, false
	}
	u.seen[m.ID] = true
	return core.UsageResponse{
		Time:  u.now(),
		Agent: msg.ParentToolUseID,
		Kind:  u.kindOf(msg.ParentToolUseID),
		Tokens: usage.Tokens{
			InputTokens:      m.Usage.InputTokens,
			CacheWriteTokens: m.Usage.CacheCreationInputTokens,
			CacheReadTokens:  m.Usage.CacheReadInputTokens,
			OutputTokens:     m.Usage.OutputTokens,
		},
	}, true
}

// kindOf is the kind of the session running under the Agent call agent:
// the main session's for "", review for a review's, and subagent for any
// other.
func (u *usageReader) kindOf(agent string) string {
	if agent == "" {
		return u.kind
	}
	for _, p := range reviewPrefixes {
		if strings.HasPrefix(u.agents[agent], p) {
			return kindReview
		}
	}
	return kindSubagent
}
