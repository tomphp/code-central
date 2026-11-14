package core

import (
	"cmp"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/store"
)

// Chat (Spec code-central-633, ADR 0040): the human talks to Claude about
// a Project from the UI. Each Project has at most one current
// Conversation, held in memory only, beside Transcripts and Heartbeats,
// never in the store. A Factory with Chat support takes each turn on the
// next_chat_work long-poll and posts the reply's items back with
// PostChatEvents. Chat writes change only memory, so they add no Move,
// Edit or Comment, and send chat.changed rather than a store notice.

// ChatItemLimit is the most items a Conversation keeps across all its
// Chat Messages, as a Transcript keeps Session Events; the oldest whole
// Chat Messages are dropped first.
const ChatItemLimit = TranscriptLimit

// DefaultChatModel is the model a Conversation uses when neither the
// human's pick nor the Project's Chat row names one.
const DefaultChatModel = "haiku"

// factoryStoppedAnswering is the error of a reply whose Factory went
// Offline before it ended.
const factoryStoppedAnswering = "the Factory stopped answering"

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

// ChatOutcome is how a Claude Chat Message ended.
type ChatOutcome string

const (
	ChatDone    ChatOutcome = "done"
	ChatStopped ChatOutcome = "stopped"
	ChatFailed  ChatOutcome = "failed"
)

// Who wrote a Chat Message.
const (
	ChatFromHuman  = "human"
	ChatFromClaude = "claude"
)

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
	From    string             `json:"from"`
	Time    time.Time          `json:"time"`
	Text    string             `json:"text,omitempty"`
	Items   []HeldSessionEvent `json:"items,omitempty"`
	Outcome ChatOutcome        `json:"outcome,omitempty"`
	Error   string             `json:"error,omitempty"`
}

// size is how many items m counts towards ChatItemLimit: a human Chat
// Message is one.
func (m ChatMessageView) size() int {
	if m.From == ChatFromHuman {
		return 1
	}
	return len(m.Items)
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

// conversation is a Project's current Conversation.
type conversation struct {
	id            string
	model, effort string
	// factory is the Factory holding it; nil until one takes a turn.
	factory                  *heartbeatKey
	sessionID, resumeCommand string
	messages                 []ChatMessageView
	lastSeq                  int
	tail                     bool
	// running says a reply is pending or running; queued that its turn,
	// turn, waits to be handed out; stopAsked that the Factory has been
	// asked to stop it.
	running, queued, stopAsked bool
	turn                       string
}

// chats holds each Project's current Conversation and the Factories'
// open next_chat_work calls, in memory only.
type chats struct {
	mu  sync.Mutex
	all map[string]*conversation
	// polls is each Factory's one open next_chat_work call per Project.
	polls map[heartbeatKey]*chatPoll
	// stops are the Conversations whose running turn each Factory is yet
	// to be told to stop, oldest first.
	stops map[heartbeatKey][]string
	// seen is each Project's availability as chat.changed last announced.
	seen map[string]ChatAvailability
}

func newChats() *chats {
	return &chats{all: map[string]*conversation{}, polls: map[heartbeatKey]*chatPoll{}, stops: map[heartbeatKey][]string{}, seen: map[string]ChatAvailability{}}
}

// chatPoll is one open next_chat_work call. wake fires when there may be
// work for it; replaced is closed when a newer call from the same Factory
// takes its place.
type chatPoll struct {
	wake     chan struct{}
	replaced chan struct{}
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
	Factory string
	Host    string
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

// GetChat is the human's get_chat: the Project's current Conversation.
func (c *Core) GetChat(_ context.Context, key string) (ConversationView, error) {
	if err := c.store.View(func(r store.Reader) error {
		if _, ok := r.Project(key); !ok {
			return unknownProject(key)
		}
		return nil
	}); err != nil {
		return ConversationView{}, err
	}
	cs := c.chats
	cs.mu.Lock()
	defer cs.mu.Unlock()
	fs := c.settleChat(key)
	return c.conversationView(cs.all[key], fs), nil
}

// StartConversation is the human's start_conversation: it replaces the
// Project's current Conversation with a new one, not yet held by any
// Factory, first asking the old one's Factory to stop any running reply.
// The model is the one given, else the Chat row's, else haiku; the
// effort the one given, else the Chat row's, else none.
func (c *Core) StartConversation(ctx context.Context, key string, in StartConversationInput) (ConversationView, error) {
	if err := humanOnly(ctx, "start_conversation"); err != nil {
		return ConversationView{}, err
	}
	model := strings.TrimSpace(in.Model)
	var row store.ModelRow
	if err := c.store.View(func(r store.Reader) error {
		if err := writable(r, key); err != nil {
			return err
		}
		switch {
		case in.Model != "" && model == "":
			return invalid("model", "the model is blank; leave it empty for the Chat row's")
		case strings.ContainsAny(model, "\r\n"):
			return invalid("model", "the model spans more than one line; a model is one line")
		}
		if in.Effort != "" {
			if err := checkEffort(r, key, "effort", in.Effort); err != nil {
				return err
			}
		}
		row = chatRow(r, key)
		return nil
	}); err != nil {
		return ConversationView{}, err
	}
	cs := c.chats
	cs.mu.Lock()
	defer cs.mu.Unlock()
	c.settleChat(key)
	cv := c.newConversation(key, row, model, in.Effort)
	return c.conversationView(cv, c.chatFactories(key)), nil
}

// SendChatMessage is the human's send_chat_message. With no current
// Conversation it starts one with the Chat row's defaults. It adds the
// human's Chat Message and a pending reply, and queues the turn for the
// Conversation's Factory, or for the first Factory with Chat support to
// ask when none holds it yet. It is refused while a reply is running and
// while the Conversation is unavailable: messages are never queued.
func (c *Core) SendChatMessage(ctx context.Context, key string, in SendChatMessageInput) (ConversationView, error) {
	if err := humanOnly(ctx, "send_chat_message"); err != nil {
		return ConversationView{}, err
	}
	switch {
	case strings.TrimSpace(in.Text) == "":
		return ConversationView{}, invalid("text", "text is required: the Chat Message to send")
	case len(in.Text) > SessionEventTextLimit:
		return ConversationView{}, invalid("text", "a Chat Message is at most %d bytes; this one is %d", SessionEventTextLimit, len(in.Text))
	}
	var row store.ModelRow
	if err := c.store.View(func(r store.Reader) error {
		if err := writable(r, key); err != nil {
			return err
		}
		row = chatRow(r, key)
		return nil
	}); err != nil {
		return ConversationView{}, err
	}
	cs := c.chats
	cs.mu.Lock()
	defer cs.mu.Unlock()
	fs := c.settleChat(key)
	cv := cs.all[key]
	if cv == nil {
		if chatAvailability(&conversation{}, fs) != ChatAvailable {
			return ConversationView{}, unavailableChat(key, nil, fs)
		}
		cv = c.newConversation(key, row, "", "")
	}
	switch {
	case cv.running:
		return ConversationView{}, &Error{Code: ActionNotAllowed, AllowedActions: []string{"stop_chat_reply"},
			Message: "a reply is running; wait for it to end, or stop it, before sending another Chat Message"}
	case chatAvailability(cv, fs) != ChatAvailable:
		return ConversationView{}, unavailableChat(key, cv.factory, fs)
	}
	now := c.now()
	cv.messages = append(cv.messages,
		ChatMessageView{From: ChatFromHuman, Time: now, Text: in.Text},
		ChatMessageView{From: ChatFromClaude, Time: now})
	cv.running, cv.queued, cv.stopAsked, cv.turn = true, true, false, in.Text
	cv.bound()
	c.chatChanged(key)
	cs.wake(key)
	return c.conversationView(cv, fs), nil
}

// StopChatReply is the human's stop_chat_reply: it asks the
// Conversation's Factory to stop the running reply. A reply whose turn no
// Factory has taken yet ends stopped at once. With no reply running it
// does nothing.
func (c *Core) StopChatReply(ctx context.Context, key string, _ StopChatReplyInput) (ConversationView, error) {
	if err := humanOnly(ctx, "stop_chat_reply"); err != nil {
		return ConversationView{}, err
	}
	if err := c.store.View(func(r store.Reader) error { return writable(r, key) }); err != nil {
		return ConversationView{}, err
	}
	cs := c.chats
	cs.mu.Lock()
	defer cs.mu.Unlock()
	fs := c.settleChat(key)
	cv := cs.all[key]
	if cv != nil && cv.running {
		c.stopReply(key, cv)
	}
	return c.conversationView(cv, fs), nil
}

// NextChatWork is the Factory's next_chat_work long-poll. It holds until
// there is work for the Factory, for at most WaitTimeout on the injected
// clock or until ctx ends, and then returns none. A turn of a
// Conversation no Factory holds yet goes to the first call from an
// Online Factory with Chat support, and binds the Conversation to it; a
// later turn or stop goes only to that Factory. Each is handed out once.
// A newer call from the same Factory and host ends this one, empty.
func (c *Core) NextChatWork(ctx context.Context, key string, in NextChatWorkInput) (ChatWork, error) {
	name, host := strings.TrimSpace(in.Factory), strings.TrimSpace(in.Host)
	switch {
	case name == "":
		return ChatWork{}, invalid("factory", "factory is required: the Factory's name, as its Heartbeat gives it")
	case host == "":
		return ChatWork{}, invalid("host", "host is required: the machine the Factory runs on, as its Heartbeat gives it")
	}
	if err := c.store.View(func(r store.Reader) error {
		if _, ok := r.Project(key); !ok {
			return unknownProject(key)
		}
		return nil
	}); err != nil {
		return ChatWork{}, err
	}
	k := heartbeatKey{key, name, host}
	cs := c.chats
	p := &chatPoll{wake: make(chan struct{}, 1), replaced: make(chan struct{})}
	cs.mu.Lock()
	if old := cs.polls[k]; old != nil {
		close(old.replaced)
	}
	cs.polls[k] = p
	cs.mu.Unlock()
	defer func() {
		cs.mu.Lock()
		if cs.polls[k] == p {
			delete(cs.polls, k)
		}
		cs.mu.Unlock()
	}()
	var timeout <-chan time.Time
	for {
		if w, ok := c.takeChatWork(k); ok {
			return w, nil
		}
		if timeout == nil {
			timeout = c.clock.After(WaitTimeout)
		}
		select {
		case <-p.wake:
		case <-p.replaced:
			return ChatWork{}, nil
		case <-timeout:
			return ChatWork{}, nil
		case <-ctx.Done():
			return ChatWork{}, nil
		}
	}
}

// takeChatWork hands Factory k its next piece of work, if there is one:
// a stop first, then a queued turn.
func (c *Core) takeChatWork(k heartbeatKey) (ChatWork, bool) {
	cs := c.chats
	cs.mu.Lock()
	defer cs.mu.Unlock()
	fs := c.settleChat(k.project)
	if stops := cs.stops[k]; len(stops) > 0 {
		cs.stops[k] = stops[1:]
		if len(cs.stops[k]) == 0 {
			delete(cs.stops, k)
		}
		return ChatWork{Stop: &ChatStop{Conversation: stops[0]}}, true
	}
	cv := cs.all[k.project]
	if cv == nil || !cv.queued || cv.factory != nil && *cv.factory != k {
		return ChatWork{}, false
	}
	if f, ok := fs[k]; !ok || f.Status != FactoryOnline || !f.Chat {
		return ChatWork{}, false
	}
	cv.factory, cv.queued = &k, false
	c.chatChanged(k.project)
	return ChatWork{Turn: &ChatTurn{
		Conversation: cv.id, Text: cv.turn, Model: cv.model, Effort: cv.effort,
		First: cv.sessionID == "", SessionID: cv.sessionID,
	}}, true
}

// PostChatEvents is POST /api/p/{key}/chat/events: the Factory holding
// the Project's current Conversation posts its running reply's items,
// and its outcome on the last post. Like PostSessionEvents it is not one
// of the operations and never fails: a post for a Conversation that is no
// longer current, from a Factory that doesn't hold it, or for a reply
// that has already ended is dropped.
func (c *Core) PostChatEvents(_ context.Context, key string, in PostChatEventsInput) {
	cs := c.chats
	cs.mu.Lock()
	defer cs.mu.Unlock()
	c.settleChat(key)
	cv := cs.all[key]
	from := heartbeatKey{key, strings.TrimSpace(in.Factory), strings.TrimSpace(in.Host)}
	if cv == nil || cv.id != in.Conversation || cv.factory == nil || *cv.factory != from || !cv.running || cv.queued {
		return
	}
	if in.SessionID != "" {
		cv.sessionID = in.SessionID
	}
	if in.ResumeCommand != "" {
		cv.resumeCommand = in.ResumeCommand
	}
	reply := &cv.messages[len(cv.messages)-1]
	now := c.now()
	for _, e := range in.Items {
		if e.Time.IsZero() {
			e.Time = now
		}
		e.Text, e.Truncated = cutText(e.Text, e.Truncated)
		cv.lastSeq++
		reply.Items = append(reply.Items, HeldSessionEvent{Seq: cv.lastSeq, SessionEvent: e})
	}
	switch in.Outcome {
	case ChatDone, ChatStopped, ChatFailed:
		msg, _ := cutText(in.Error, false)
		if in.Outcome != ChatFailed {
			msg = ""
		}
		cv.end(in.Outcome, msg)
		c.chatChanged(key)
	}
	cv.bound()
}

// chatSession is the Project whose current Conversation has the session
// ID, for filing its usage.
func (c *Core) chatSession(sessionID string) (string, bool) {
	cs := c.chats
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for key, cv := range cs.all {
		if cv.sessionID == sessionID {
			return key, true
		}
	}
	return "", false
}

// chatHeartbeat runs after a Heartbeat of the Project's: a Factory may now
// take a waiting turn, or the Conversation's availability may have
// changed.
func (c *Core) chatHeartbeat(key string) {
	cs := c.chats
	cs.mu.Lock()
	defer cs.mu.Unlock()
	c.settleChat(key)
	cs.wake(key)
}

// watchChats is the Factory Status watch's tick for the Conversations:
// it ends a reply whose Factory went Offline with no write, and
// announces each change in availability.
func (c *Core) watchChats() {
	cs := c.chats
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for key := range cs.all {
		c.settleChat(key)
	}
}

// newConversation makes key's new current Conversation, after asking the
// old one's Factory to stop any running reply. Callers hold cs.mu.
func (c *Core) newConversation(key string, row store.ModelRow, model, effort string) *conversation {
	cs := c.chats
	if old := cs.all[key]; old != nil && old.running {
		c.stopReply(key, old)
	}
	cv := &conversation{
		id:     ids.Random{}.ClaimID(),
		model:  cmp.Or(model, row.Model, DefaultChatModel),
		effort: cmp.Or(effort, row.Effort),
	}
	cs.all[key] = cv
	c.chatChanged(key)
	return cv
}

// stopReply stops cv's running reply: at once while no Factory has taken
// its turn, else by asking the Factory holding it, once. Callers hold
// cs.mu.
func (c *Core) stopReply(key string, cv *conversation) {
	cs := c.chats
	if cv.queued {
		cv.end(ChatStopped, "")
		c.chatChanged(key)
		return
	}
	if cv.stopAsked {
		return
	}
	cv.stopAsked = true
	k := *cv.factory
	cs.stops[k] = append(cs.stops[k], cv.id)
	if p := cs.polls[k]; p != nil {
		p.notify()
	}
}

// settleChat ends the Project's running reply as failed when its Factory
// has gone Offline, announces any change in the Conversation's
// availability, and returns the Project's Factories by key. Callers hold
// cs.mu.
func (c *Core) settleChat(key string) map[heartbeatKey]FactoryView {
	cs := c.chats
	fs := c.chatFactories(key)
	cv := cs.all[key]
	if cv != nil && cv.running && cv.factory != nil {
		if f, ok := fs[*cv.factory]; !ok || f.Status == FactoryOffline {
			cv.end(ChatFailed, factoryStoppedAnswering)
			c.chatChanged(key)
		}
	}
	if a := chatAvailability(cv, fs); a != cs.seen[key] {
		if _, announced := cs.seen[key]; announced || a != ChatNone {
			c.chatChanged(key)
		}
		cs.seen[key] = a
	}
	return fs
}

// chatFactories is the Project's Factories as their Heartbeats stand now.
func (c *Core) chatFactories(key string) map[heartbeatKey]FactoryView {
	fs := map[heartbeatKey]FactoryView{}
	for _, f := range c.heartbeats.view(key, c.clock.Now()).Factories {
		fs[heartbeatKey{key, f.Name, f.Host}] = f
	}
	return fs
}

// chatAvailability is cv's availability among the Project's Factories fs.
func chatAvailability(cv *conversation, fs map[heartbeatKey]FactoryView) ChatAvailability {
	if cv == nil {
		return ChatNone
	}
	answers := func(f FactoryView) bool { return f.Status == FactoryOnline && f.Chat }
	if cv.factory != nil {
		if f, ok := fs[*cv.factory]; ok && answers(f) {
			return ChatAvailable
		}
		return ChatUnavailable
	}
	for _, f := range fs {
		if answers(f) {
			return ChatAvailable
		}
	}
	return ChatUnavailable
}

// unavailableChat is send_chat_message's refusal while the Conversation,
// held by factory or by none yet, is unavailable.
func unavailableChat(key string, factory *heartbeatKey, fs map[heartbeatKey]FactoryView) *Error {
	e := &Error{Code: ActionNotAllowed, AllowedActions: []string{"start_conversation"}}
	if factory == nil {
		e.AllowedActions = nil
		e.Message = fmt.Sprintf("no Online Factory of %s has Chat support; start code-central-factory without --no-chat", key)
		return e
	}
	f, ok := fs[*factory]
	switch {
	case !ok || f.Status != FactoryOnline:
		e.Message = fmt.Sprintf("the Conversation is unavailable: its Factory %s on %s is %s; start a new Conversation, or send again once it is back Online", factory.name, factory.host, cmp.Or(f.Status, FactoryOffline))
	default:
		e.Message = fmt.Sprintf("the Conversation is unavailable: its Factory %s on %s no longer has Chat support; start a new Conversation", factory.name, factory.host)
	}
	return e
}

// conversationView is cv as reads return it.
func (c *Core) conversationView(cv *conversation, fs map[heartbeatKey]FactoryView) ConversationView {
	v := ConversationView{Availability: chatAvailability(cv, fs), Messages: []ChatMessageView{}}
	if cv == nil {
		return v
	}
	v.ID, v.Model, v.Effort = cv.id, cv.model, cv.effort
	v.Running, v.Tail = cv.running, cv.tail
	v.SessionID, v.ResumeCommand = cv.sessionID, cv.resumeCommand
	if cv.factory != nil {
		v.Factory = &ChatFactory{Name: cv.factory.name, Host: cv.factory.host}
	}
	for _, m := range cv.messages {
		m.Items = append([]HeldSessionEvent(nil), m.Items...)
		v.Messages = append(v.Messages, m)
	}
	return v
}

// chatChanged announces a change in the Project's Conversation.
func (c *Core) chatChanged(key string) {
	c.events.Publish(events.Event{Type: events.ChatChanged, Project: key})
}

// end ends cv's running reply with outcome, and error text when failed.
func (cv *conversation) end(outcome ChatOutcome, errText string) {
	reply := &cv.messages[len(cv.messages)-1]
	reply.Outcome, reply.Error = outcome, errText
	cv.running, cv.queued, cv.stopAsked, cv.turn = false, false, false, ""
}

// bound keeps cv to ChatItemLimit items, dropping the oldest whole Chat
// Messages first, and the oldest items of the one left only when it alone
// is over, and marks it as a tail once it has dropped any.
func (cv *conversation) bound() {
	total := 0
	for _, m := range cv.messages {
		total += m.size()
	}
	for total > ChatItemLimit && len(cv.messages) > 1 {
		total -= cv.messages[0].size()
		cv.messages = cv.messages[1:]
		cv.tail = true
	}
	if over := total - ChatItemLimit; over > 0 {
		cv.messages[0].Items = cv.messages[0].Items[over:]
		cv.tail = true
	}
}

// wake tells every open next_chat_work call of the Project to look for
// work again. Callers hold cs.mu.
func (cs *chats) wake(key string) {
	for k, p := range cs.polls {
		if k.project == key {
			p.notify()
		}
	}
}

func (p *chatPoll) notify() {
	select {
	case p.wake <- struct{}{}:
	default: // already woken
	}
}

// chatRow is the Project's Chat row of its Models.
func chatRow(r store.Reader, key string) store.ModelRow {
	p, _ := r.Project(key)
	return p.Models.Chat
}
