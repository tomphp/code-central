package factory

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/tomoram/code-central/client"
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

const (
	// toolResultMax is the most bytes of a tool result's text posted.
	toolResultMax = 4 << 10
	// eventQueueMax is the most events waiting to be posted; when it is
	// full the oldest are dropped.
	eventQueueMax = 1000
	// eventBatchEvery is how often waiting events are posted.
	eventBatchEvery = time.Second
	// eventBatchMax is how many waiting events make a post at once.
	eventBatchMax = 100
	// eventPostTimeout bounds each post, the last flush's too.
	eventPostTimeout = 2 * time.Second
)

// sessionEvent is a Session Event waiting to be posted for a session.
type sessionEvent struct {
	session string
	event   core.SessionEvent
}

// sender is the Factory's one poster of Session Events. It posts what is
// waiting every eventBatchEvery, or sooner once eventBatchMax are waiting,
// one post per session in the order they came, so each session's order
// holds. every is eventBatchEvery unless a test shortens it.
type sender struct {
	client *client.Client
	every  time.Duration

	mu      sync.Mutex
	waiting []sessionEvent
	full    chan struct{} // signalled when eventBatchMax are waiting
	stop    chan struct{}
	done    chan struct{}
}

func newSender(c *client.Client, every time.Duration) *sender {
	return &sender{client: c, every: every, full: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{})}
}

// add queues e for session, dropping the oldest waiting event when the
// queue is full. It never blocks on a post.
func (s *sender) add(session string, e core.SessionEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.waiting) >= eventQueueMax {
		s.waiting = s.waiting[1:]
	}
	s.waiting = append(s.waiting, sessionEvent{session, e})
	if len(s.waiting) >= eventBatchMax {
		select {
		case s.full <- struct{}{}:
		default:
		}
	}
}

// run posts batches until close.
func (s *sender) run() {
	defer close(s.done)
	tick := time.NewTicker(s.every)
	defer tick.Stop()
	for {
		select {
		case <-s.stop:
			s.flush()
			return
		case <-tick.C:
		case <-s.full:
		}
		s.flush()
	}
}

// close makes one last flush and stops the sender.
func (s *sender) close() {
	close(s.stop)
	<-s.done
}

// flush posts every waiting event, one post per session, each with
// eventPostTimeout. A post that fails is dropped, never retried.
func (s *sender) flush() {
	s.mu.Lock()
	waiting := s.waiting
	s.waiting = nil
	s.mu.Unlock()
	var sessions []string
	bySession := map[string][]core.SessionEvent{}
	for _, w := range waiting {
		if _, ok := bySession[w.session]; !ok {
			sessions = append(sessions, w.session)
		}
		bySession[w.session] = append(bySession[w.session], w.event)
	}
	for _, session := range sessions {
		ctx, cancel := context.WithTimeout(context.Background(), eventPostTimeout)
		_ = s.client.PostSessionEvents(ctx, session, bySession[session])
		cancel()
	}
}

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
