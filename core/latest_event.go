package core

import (
	"strings"
	"sync"
	"time"

	"github.com/tomoram/code-central/events"
)

// The latest event (ticket 107): a Claim's newest Session Event, shown as
// one short line on the Claim view and on its Ticket's row while the
// Claim is live. It is computed on read from the Transcript, never
// stored. A Session Event changes no stored state, so a new one sends
// ticket.changed itself, throttled per Ticket.

// LatestEventLimit is the most characters the latest event's text keeps;
// longer text is cut, ending in "…".
const LatestEventLimit = 120

// LatestEventThrottle is the shortest gap between two ticket.changed
// notices a Ticket's new latest events send. A change inside it is sent
// when it ends, so the last change is never lost.
const LatestEventThrottle = 5 * time.Second

// LatestEvent is a Claim's latest Session Event, shown short.
type LatestEvent struct {
	Kind string    `json:"kind"`
	Text string    `json:"text"`
	Time time.Time `json:"time"`
}

// latestEvent is Claim number claim's latest event, or nil when its
// Transcript holds none.
func (c *Core) latestEvent(key string, ticket, claim int) *LatestEvent {
	ts := c.transcripts
	ts.mu.Lock()
	defer ts.mu.Unlock()
	tr := ts.all[markKey{key, ticket, claim}]
	if tr == nil || len(tr.events) == 0 {
		return nil
	}
	e := tr.events[len(tr.events)-1]
	return &LatestEvent{Kind: e.Kind, Text: cutLine(tr.line(e.SessionEvent)), Time: e.Time}
}

// line is e as one line: a tool result as "<Tool> result" or "<Tool>
// failed", naming the tool from the call it answers when it names none;
// anything else as its first non-empty line.
func (tr *transcript) line(e SessionEvent) string {
	if e.Kind != "tool_result" {
		for l := range strings.Lines(e.Text) {
			if l = strings.TrimSpace(l); l != "" {
				return l
			}
		}
		return ""
	}
	tool := e.Tool
	for i := len(tr.events) - 1; tool == "" && e.ToolUseID != "" && i >= 0; i-- {
		if call := tr.events[i]; call.Kind == "tool_call" && call.ToolUseID == e.ToolUseID {
			tool = call.Tool
		}
	}
	if tool == "" {
		tool = "Tool"
	}
	if e.IsError {
		return tool + " failed"
	}
	return tool + " result"
}

// cutLine cuts s to LatestEventLimit characters, ending in "…" when cut.
func cutLine(s string) string {
	r := []rune(s)
	if len(r) <= LatestEventLimit {
		return s
	}
	return string(r[:LatestEventLimit-1]) + "…"
}

// latestNotices is, per Ticket, when a new latest event last sent
// ticket.changed and whether a trailing one is waiting.
type latestNotices struct {
	mu      sync.Mutex
	sent    map[markKey]time.Time
	pending map[markKey]bool
}

// latestEventChanged sends ticket.changed for the Ticket now, unless one
// went within LatestEventThrottle; then it sends one when the throttle
// ends, standing for every change in between.
func (c *Core) latestEventChanged(project string, ticket int) {
	n := c.latestNotices
	k := markKey{project, ticket, 0}
	n.mu.Lock()
	if n.pending[k] {
		n.mu.Unlock()
		return
	}
	now := c.clock.Now()
	wait := n.sent[k].Add(LatestEventThrottle).Sub(now)
	if wait <= 0 {
		n.sent[k] = now
		n.mu.Unlock()
		c.publishTicketChanged(project, ticket)
		return
	}
	n.pending[k] = true
	timer := c.clock.After(wait)
	n.mu.Unlock()
	go func() {
		<-timer
		n.mu.Lock()
		n.sent[k] = c.clock.Now()
		delete(n.pending, k)
		n.mu.Unlock()
		c.publishTicketChanged(project, ticket)
	}()
}

func (c *Core) publishTicketChanged(project string, ticket int) {
	c.events.Publish(events.Event{Type: events.TicketChanged, Project: project, ID: DisplayID(project, ticket)})
}
