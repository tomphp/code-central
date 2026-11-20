package core

import (
	"context"

	"github.com/tomoram/code-central/store"
)

// The Conversation stream (Spec code-central-633): the UI follows a
// Project's current Conversation as it grows, rather than refetching it on
// each item. Every change to the Conversation is an event with a sequence
// number that rises across the Project's Conversations, so a reconnect's
// Last-Event-ID says where the client stood.

// Chat stream event names.
const (
	// ChatEventConversation is the Conversation as it stands (a
	// ConversationView), sent on opening and when a resume can't be
	// served from the events held.
	ChatEventConversation = "conversation"
	// ChatEventMessage is a new Chat Message (a ChatMessageView): the
	// human's, or the Claude one a reply fills.
	ChatEventMessage = "message"
	// ChatEventItem is a new item of the running reply (a
	// HeldSessionEvent).
	ChatEventItem = "item"
	// ChatEventOutcome is the end of the reply (a ChatOutcomeEvent).
	ChatEventOutcome = "outcome"
	// ChatEventSession is the session ID and Resume Command as the
	// Factory reported them (a ChatSessionEvent).
	ChatEventSession = "session"
	// ChatEventReplaced says the Conversation the client followed is
	// gone; the Conversation as it now stands follows it.
	ChatEventReplaced = "replaced"
)

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

// chatEvent is a change to a Conversation, as held for followers to
// catch up on.
type chatEvent struct {
	seq   int
	event string
	data  any
}

// emit records a change to cv.
func (cv *conversation) emit(event string, data any) {
	cv.evSeq++
	cv.log = append(cv.log, chatEvent{cv.evSeq, event, data})
	if over := len(cv.log) - ChatItemLimit; over > 0 {
		cv.log = cv.log[over:]
	}
}

// A ChatFeed follows a Project's current Conversation. Like a
// TranscriptFeed it holds no events of its own, only where it stands.
type ChatFeed struct {
	c    *Core
	key  string
	wake chan struct{}
	// opened says the feed has sent its opening; conv is the Conversation
	// it follows ("" for none) and after the last sequence number given.
	opened bool
	conv   string
	after  int
}

// FollowChat starts a feed of the Project's Conversation after sequence
// number after (0 for all of it), as a reconnect's Last-Event-ID asks. An
// unknown Project is not_found. Stop ends it.
func (c *Core) FollowChat(_ context.Context, key string, after int) (*ChatFeed, error) {
	if err := c.store.View(func(r store.Reader) error {
		if _, ok := r.Project(key); !ok {
			return unknownProject(key)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	cs := c.chats
	cs.mu.Lock()
	defer cs.mu.Unlock()
	f := &ChatFeed{c: c, key: key, wake: make(chan struct{}, 1), after: after}
	if cs.feeds[key] == nil {
		cs.feeds[key] = map[*ChatFeed]struct{}{}
	}
	cs.feeds[key][f] = struct{}{}
	f.wake <- struct{}{} // the opening is waiting already
	return f, nil
}

// Wake fires when Next may have frames to give.
func (f *ChatFeed) Wake() <-chan struct{} { return f.wake }

// Next gives the frames after the last it gave. It opens with the
// Conversation as it stands, unless a resume can be served from the events
// held: a client resuming from an earlier Conversation is first told it
// was replaced. Afterwards it gives each new event, or the Conversation
// again when the events it missed are no longer held.
func (f *ChatFeed) Next() []ChatFrame {
	cs := f.c.chats
	cs.mu.Lock()
	defer cs.mu.Unlock()
	fs := f.c.settleChat(f.key)
	cv := cs.all[f.key]
	if cv == nil {
		if f.opened {
			return nil
		}
		return []ChatFrame{f.snapshot(nil, fs)}
	}
	switch {
	case !f.opened && f.after > 0 && f.after >= cv.base && f.after <= cv.evSeq && cv.holdsAfter(f.after):
		// A resume the events held can serve.
		f.opened, f.conv = true, cv.id
	case !f.opened:
		var frames []ChatFrame
		if f.after > 0 && f.after < cv.base {
			frames = append(frames, ChatFrame{Event: ChatEventReplaced, Data: struct{}{}})
		}
		return append(frames, f.snapshot(cv, fs))
	case f.conv != cv.id:
		if f.conv == "" {
			return []ChatFrame{f.snapshot(cv, fs)}
		}
		return []ChatFrame{{Event: ChatEventReplaced, Data: struct{}{}}, f.snapshot(cv, fs)}
	case !cv.holdsAfter(f.after):
		return []ChatFrame{f.snapshot(cv, fs)}
	}
	var frames []ChatFrame
	for _, e := range cv.log {
		if e.seq > f.after {
			frames = append(frames, ChatFrame{ID: e.seq, Event: e.event, Data: e.data})
		}
	}
	f.after = cv.evSeq
	return frames
}

// snapshot is the Conversation as it stands, as the feed's next frame.
func (f *ChatFeed) snapshot(cv *conversation, fs map[heartbeatKey]FactoryView) ChatFrame {
	f.opened = true
	fr := ChatFrame{Event: ChatEventConversation, Data: f.c.conversationView(f.key, cv, fs)}
	if cv != nil {
		f.conv, f.after, fr.ID = cv.id, cv.evSeq, cv.evSeq
	}
	return fr
}

// holdsAfter says cv still holds every event after seq.
func (cv *conversation) holdsAfter(seq int) bool {
	if seq >= cv.evSeq {
		return true
	}
	return len(cv.log) > 0 && cv.log[0].seq <= seq+1
}

// Stop ends the feed.
func (f *ChatFeed) Stop() {
	cs := f.c.chats
	cs.mu.Lock()
	defer cs.mu.Unlock()
	delete(cs.feeds[f.key], f)
	if len(cs.feeds[f.key]) == 0 {
		delete(cs.feeds, f.key)
	}
}

// wakeFeeds tells the Project's followers to look for new events. Callers
// hold cs.mu.
func (cs *chats) wakeFeeds(key string) {
	for f := range cs.feeds[key] {
		select {
		case f.wake <- struct{}{}:
		default: // already woken
		}
	}
}
