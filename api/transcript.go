package api

import "time"

// SessionEventKind is a Session Event's kind, one of SessionEventKinds as
// the Factory posts them. Code Central doesn't check it.
type SessionEventKind string

// The kinds of Session Event.
const (
	// EventText is the agent's text.
	EventText SessionEventKind = "text"
	// EventToolCall is a tool call, its text a summary of the call.
	EventToolCall SessionEventKind = "tool_call"
	// EventToolResult is a tool call's result.
	EventToolResult SessionEventKind = "tool_result"
	// EventFactory is a Factory log line about the Claim.
	EventFactory SessionEventKind = "factory"
	// EventPermissionDenials is a Chat reply's item listing the calls
	// its turn had denied, from its result's permission_denials.
	EventPermissionDenials SessionEventKind = "permission_denials"
)

// SessionEventKinds are the kinds of Session Event.
func SessionEventKinds() []SessionEventKind {
	return []SessionEventKind{EventText, EventToolCall, EventToolResult, EventFactory, EventPermissionDenials}
}

// SessionEvent is one thing a Claim's session did, as posted: agent text,
// a tool call, a tool result or a Factory log line. Tool and Text are
// opaque strings.
type SessionEvent struct {
	Time      time.Time        `json:"time"`
	Kind      SessionEventKind `json:"kind"`
	Text      string           `json:"text"`
	Tool      string           `json:"tool,omitempty"`
	ToolUseID string           `json:"tool_use_id,omitempty"`
	IsError   bool             `json:"is_error,omitempty"`
	Truncated bool             `json:"truncated,omitempty"`
}

// HeldSessionEvent is a Session Event in a Transcript, with its sequence
// number, which rises within the Transcript and is the SSE id its stream
// resumes from.
type HeldSessionEvent struct {
	Seq int `json:"seq"`
	SessionEvent
}

// TranscriptView is a Claim's Transcript as the read returns it: the held
// events, oldest first. Tail is true when it holds only part of the
// session: events were dropped from its front, as it keeps the last
// TranscriptLimit, or it began after the Claim did, as none are kept
// from before Code Central's last restart, or on another service before
// its session moved here. Host is the host of another service on the
// Store holding the Transcript, its Reporting Service, while that service
// is running: then Events is empty.
type TranscriptView struct {
	Events []HeldSessionEvent `json:"events"`
	Tail   bool               `json:"tail"`
	Host   string             `json:"host,omitempty"`
}
