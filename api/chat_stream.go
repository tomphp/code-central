package api

// ChatOutcomeEvent is the data of an outcome event.
type ChatOutcomeEvent struct {
	Outcome ChatOutcome `json:"outcome"`
	Error   string      `json:"error,omitempty"`
}

// ChatSessionEvent is the data of a session event.
type ChatSessionEvent struct {
	SessionID     string `json:"session_id"`
	ResumeCommand string `json:"resume_command"`
}

// ChatFrame is one event of the stream. ID is its sequence number, 0 for
// one that has none (replaced).
type ChatFrame struct {
	ID    int
	Event string
	Data  any
}
