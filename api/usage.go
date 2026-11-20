package api

import (
	"time"
)

// UsageResponse is one model response's token usage, as a Factory posts
// it: when it came, the subagent it was for ("" for the main session)
// with that Subagent's Agent Type and description, the kind of session,
// the model that gave it ("" when not posted), and its four token counts.
type UsageResponse struct {
	Time        time.Time `json:"time"`
	Agent       string    `json:"agent,omitempty"`
	AgentType   string    `json:"agent_type,omitempty"`
	Description string    `json:"description,omitempty"`
	Kind        string    `json:"kind"`
	Model       string    `json:"model,omitempty"`
	Tokens
}

// UsageInput is a Usage read: one Project, or every Project (Archived
// ones too) when Project is "", over the last Days local days.
type UsageInput struct {
	Project string
	// Days is one of UsageDays; 0 is the default.
	Days int
}

// UsageView is Usage as the read returns it. Kinds, Sessions and Days
// come from the token usage a Factory posts: Kinds over every session,
// main and Subagent; Sessions the main sessions, newest first and at
// most RecentSessions, each with all its Subagents; and Days a row for
// every local day in the range, oldest first.
type UsageView struct {
	Operations []UsageOperation `json:"operations"`
	Kinds      []UsageKind      `json:"kinds"`
	Sessions   []UsageSession   `json:"sessions"`
	Days       []UsageDay       `json:"days"`
}

// UsageOperation is one MCP operation's row: its calls, their total and
// average request and response sizes in bytes, and how many were
// rejected. Averages are rounded to the nearest byte.
type UsageOperation struct {
	Op                   string `json:"op"`
	Calls                int    `json:"calls"`
	RequestBytes         int    `json:"request_bytes"`
	AverageRequestBytes  int    `json:"average_request_bytes"`
	ResponseBytes        int    `json:"response_bytes"`
	AverageResponseBytes int    `json:"average_response_bytes"`
	Errors               int    `json:"errors"`
}

// UsageKind is one kind of session's Context Size figures: how many
// sessions there were, and the average and largest of their peaks. The
// average is rounded to the nearest token.
type UsageKind struct {
	Kind        string `json:"kind"`
	Sessions    int    `json:"sessions"`
	AveragePeak int    `json:"average_peak"`
	LargestPeak int    `json:"largest_peak"`
}

// UsageSession is one recent session: a Claim's main session, or a
// Subagent of it (Agent), with the Project, Ticket, Claim and kind of its
// latest response. Started is its first response's time, Peak the largest
// Context Size among its responses, Tokens all their tokens, and Models
// each distinct model its responses name, in order of first use. A
// Subagent has the latest non-empty Agent Type and description among its
// responses.
//
// A main session has its Subagents, oldest first, and TotalTokens, its
// Tokens and theirs; a Subagent has no Subagents and no TotalTokens. A
// main session whose only responses are its Subagents' has none of its
// own: no kind, models, peak or tokens, its Subagents' first start and
// the Project, Ticket and Claim of their latest response.
type UsageSession struct {
	Project     string         `json:"project"`
	Ticket      string         `json:"ticket"`
	Claim       int            `json:"claim"`
	SessionID   string         `json:"session_id"`
	Agent       string         `json:"agent,omitempty"`
	AgentType   string         `json:"agent_type,omitempty"`
	Description string         `json:"description,omitempty"`
	Kind        string         `json:"kind"`
	Started     time.Time      `json:"started"`
	Peak        int            `json:"peak"`
	Responses   int            `json:"responses"`
	Models      []string       `json:"models"`
	Tokens      int            `json:"tokens"`
	TotalTokens *int           `json:"total_tokens,omitempty"`
	Subagents   []UsageSession `json:"subagents"`
}

// UsageDay is one local day's token usage, by kind of session: the kinds
// with any usage that day, in kindOrder; none on a day with no usage.
// Day is YYYY-MM-DD in the service's time zone.
type UsageDay struct {
	Day   string           `json:"day"`
	Kinds []UsageDayTokens `json:"kinds"`
}

// UsageDayTokens is one kind's tokens on one day, by token type.
type UsageDayTokens struct {
	Kind string `json:"kind"`
	Tokens
}

// SessionUsageInput is one session's read: its Project, its session ID,
// and its subagent, "" for the main session.
type SessionUsageInput struct {
	Project string
	Session string
	Agent   string
}

// SessionUsageView is one session's responses, oldest first, for its
// line chart. For a Subagent it also has its Agent Type and description,
// the latest non-empty of each among its responses.
type SessionUsageView struct {
	AgentType   string                 `json:"agent_type,omitempty"`
	Description string                 `json:"description,omitempty"`
	Responses   []SessionUsageResponse `json:"responses"`
}

// SessionUsageResponse is one of a session's model responses: when it
// came, its Context Size and its four token counts.
type SessionUsageResponse struct {
	Time        time.Time `json:"time"`
	ContextSize int       `json:"context_size"`
	Tokens
}

// Tokens are a model response's four token counts.
type Tokens struct {
	InputTokens      int `json:"input_tokens"`
	CacheWriteTokens int `json:"cache_write_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens"`
	OutputTokens     int `json:"output_tokens"`
}

// Add adds o's counts to t's.
func (t *Tokens) Add(o Tokens) {
	t.InputTokens += o.InputTokens
	t.CacheWriteTokens += o.CacheWriteTokens
	t.CacheReadTokens += o.CacheReadTokens
	t.OutputTokens += o.OutputTokens
}

// ContextSize is how many tokens the model read for the response: how
// full the session's context window was.
func (t Tokens) ContextSize() int { return t.InputTokens + t.CacheWriteTokens + t.CacheReadTokens }

// Total is all four counts as one number: what the response used.
func (t Tokens) Total() int { return t.ContextSize() + t.OutputTokens }
