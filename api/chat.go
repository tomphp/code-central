package api

import "time"

// ChatAvailability is whether the Project's current Conversation can take
// a Chat Message now, worked out from the Heartbeats on every read.
type ChatAvailability string

const (
	// ChatNone is a Project with no current Conversation.
	ChatNone ChatAvailability = "none"
	// ChatAvailable is a Conversation its Factory, or before any Factory
	// has taken it any Factory with Chat support, can answer now.
	ChatAvailable ChatAvailability = "available"
	// ChatUnavailable is a Conversation whose Factory is Draining,
	// Offline or without Chat support, or, before any Factory has taken
	// it, one no Online Factory with Chat support can take.
	ChatUnavailable ChatAvailability = "unavailable"
)

// ChatAvailabilities are the Chat availabilities.
func ChatAvailabilities() []ChatAvailability {
	return []ChatAvailability{ChatNone, ChatAvailable, ChatUnavailable}
}

// ChatOutcome is how a Claude Chat Message ended.
type ChatOutcome string

const (
	// ChatDone is a reply that ran to its end.
	ChatDone ChatOutcome = "done"
	// ChatStopped is a reply the human stopped.
	ChatStopped ChatOutcome = "stopped"
	// ChatFailed is a reply that went wrong, with its error text.
	ChatFailed ChatOutcome = "failed"
)

// ChatOutcomes are the ways a Claude Chat Message ends.
func ChatOutcomes() []ChatOutcome { return []ChatOutcome{ChatDone, ChatStopped, ChatFailed} }

// ChatAuthor is who wrote a Chat Message.
type ChatAuthor string

// The authors of Chat Messages.
const (
	ChatFromHuman  ChatAuthor = "human"
	ChatFromClaude ChatAuthor = "claude"
)

// ChatAuthors are the authors of Chat Messages.
func ChatAuthors() []ChatAuthor { return []ChatAuthor{ChatFromHuman, ChatFromClaude} }

// ConversationView is a Project's current Conversation as get_chat and
// the Chat writes return it. With none, Availability is none and the rest
// is empty.
type ConversationView struct {
	Availability ChatAvailability `json:"availability"`
	ID           string           `json:"id,omitempty"`
	Model        string           `json:"model,omitempty"`
	// Effort is empty for Claude Code's default.
	Effort string `json:"effort,omitempty"`
	// Factory is the Factory holding the Conversation, left out until one
	// takes its first turn.
	Factory *ChatFactory `json:"factory,omitempty"`
	// Running says a reply is pending or running.
	Running bool `json:"running"`
	// Tail says older Chat Messages were dropped from the front.
	Tail bool `json:"tail"`
	// SessionID and ResumeCommand are as the Factory reported them, empty
	// until it does.
	SessionID     string            `json:"session_id,omitempty"`
	ResumeCommand string            `json:"resume_command,omitempty"`
	Messages      []ChatMessageView `json:"messages"`
	// RunsOn is the host of another live service sharing the Store that
	// an Online Factory with Chat support reports to, when none reports
	// to this one: Chat runs there (Spec code-central-1107). Empty when
	// Chat runs here, or nowhere.
	RunsOn string `json:"runs_on,omitempty"`
}

// ChatFactory names the Factory holding a Conversation, by its
// Heartbeat's name and host.
type ChatFactory struct {
	Name string `json:"name"`
	Host string `json:"host"`
}

// ChatMessageView is one Chat Message. A human's holds its Text; a
// Claude's holds its Items, in the shape of Session Events (with
// permission_denials as an item of their own), and its Outcome once the
// reply has ended, with Error on a failed one.
type ChatMessageView struct {
	From    ChatAuthor         `json:"from"`
	Time    time.Time          `json:"time"`
	Text    string             `json:"text,omitempty"`
	Items   []HeldSessionEvent `json:"items,omitempty"`
	Outcome ChatOutcome        `json:"outcome,omitempty"`
	Error   string             `json:"error,omitempty"`
}

// ChatWork is what next_chat_work hands a Factory: a Turn, a Stop, or,
// when the long-poll times out, neither.
type ChatWork struct {
	Turn *ChatTurn `json:"turn,omitempty"`
	Stop *ChatStop `json:"stop,omitempty"`
}

// ChatTurn is one Chat Message for the Factory to answer.
type ChatTurn struct {
	Conversation string `json:"conversation"`
	Text         string `json:"text"`
	Model        string `json:"model"`
	Effort       string `json:"effort,omitempty"`
	// First says the turn starts the Conversation's session; otherwise
	// SessionID is the session to resume.
	First     bool   `json:"first"`
	SessionID string `json:"session_id,omitempty"`
}

// ChatStop asks the Factory to stop the Conversation's running turn.
type ChatStop struct {
	Conversation string `json:"conversation"`
}

// StartConversationInput is the human's start_conversation(model?,
// effort?): either left empty takes the Chat row's.
type StartConversationInput struct {
	Model  string `json:"model"`
	Effort string `json:"effort"`
}

// SendChatMessageInput is the human's send_chat_message(text).
type SendChatMessageInput struct {
	Text string `json:"text"`
}

// StopChatReplyInput is the human's stop_chat_reply(), which takes nothing.
type StopChatReplyInput struct{}

// NextChatWorkInput is the Factory's next_chat_work(factory, host), named
// as its Heartbeat names it.
type NextChatWorkInput struct {
	Factory string `json:"-" query:"factory"`
	Host    string `json:"-" query:"host"`
}

// PostChatEventsInput is POST /api/p/{key}/chat/events: the Factory
// holding Conversation posts its running reply's new Items, the session
// ID and Resume Command on its first post, and the Outcome, with Error
// when failed, on its last.
type PostChatEventsInput struct {
	Conversation  string         `json:"conversation"`
	Factory       string         `json:"factory"`
	Host          string         `json:"host"`
	SessionID     string         `json:"session_id"`
	ResumeCommand string         `json:"resume_command"`
	Items         []SessionEvent `json:"items"`
	Outcome       ChatOutcome    `json:"outcome"`
	Error         string         `json:"error"`
}

// GetChatInput is the human's get_chat(), which takes nothing.
type GetChatInput struct{}
