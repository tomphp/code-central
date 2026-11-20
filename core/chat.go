package core

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/store"
)

// Chat (Spec code-central-633, ADR 0040): the human talks to Claude about
// a Project from the UI. Each Project has at most one current
// Conversation, held in memory only, beside Transcripts, never in the
// store. A Factory with Chat support takes each turn on the
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

// chatMessageSize is how many items m counts towards ChatItemLimit: a human Chat
// Message is one.
func chatMessageSize(m api.ChatMessageView) int {
	if m.From == api.ChatFromHuman {
		return 1
	}
	return len(m.Items)
}

// conversation is a Project's current Conversation.
type conversation struct {
	id            string
	model, effort string
	// factory is the Factory holding it; nil until one takes a turn.
	factory                  *heartbeatKey
	sessionID, resumeCommand string
	messages                 []api.ChatMessageView
	lastSeq                  int
	tail                     bool
	// running says a reply is pending or running; queued that its turn,
	// turn, waits to be handed out; stopAsked that the Factory has been
	// asked to stop it.
	running, queued, stopAsked bool
	turn                       string
	// base is the sequence number the Conversation's stream events start
	// after, evSeq the last one given, and log the newest events held
	// (see FollowChat).
	base, evSeq int
	log         []chatEvent
}

// chats holds each Project's current Conversation and the Factories'
// open next_chat_work calls, in memory only.
type chats struct {
	mu  sync.Mutex
	all map[string]*conversation
	// polls is each Factory's one open next_chat_work call per Project.
	polls map[heartbeatKey]*longPoll
	// stops are the Conversations whose running turn each Factory is yet
	// to be told to stop, oldest first.
	stops map[heartbeatKey][]string
	// seen is each Project's availability as chat.changed last announced.
	seen map[string]api.ChatAvailability
	// feeds is each Project's followers (FollowChat).
	feeds map[string]map[*ChatFeed]struct{}
}

// newChats is an empty holder.
func newChats() *chats {
	return &chats{all: map[string]*conversation{}, polls: map[heartbeatKey]*longPoll{}, stops: map[heartbeatKey][]string{}, seen: map[string]api.ChatAvailability{}, feeds: map[string]map[*ChatFeed]struct{}{}}
}

// GetChat is the human's get_chat: the Project's current Conversation.
func (c *Core) GetChat(_ context.Context, key string, _ api.GetChatInput) (api.ConversationView, error) {
	if err := c.store.View(func(r store.Reader) error {
		if _, ok := r.Project(key); !ok {
			return unknownProject(key)
		}
		return nil
	}); err != nil {
		return api.ConversationView{}, err
	}
	cs := c.chats
	cs.mu.Lock()
	defer cs.mu.Unlock()
	fs := c.settleChat(key)
	return c.conversationView(key, cs.all[key], fs), nil
}

// StartConversation is the human's start_conversation: it replaces the
// Project's current Conversation with a new one, not yet held by any
// Factory, first asking the old one's Factory to stop any running reply.
// The model is the one given, else the Chat row's, else haiku; the
// effort the one given, else the Chat row's, else none.
func (c *Core) StartConversation(ctx context.Context, key string, in api.StartConversationInput) (api.ConversationView, error) {
	if err := humanOnly(ctx, "start_conversation"); err != nil {
		return api.ConversationView{}, err
	}
	model := strings.TrimSpace(in.Model)
	var row domain.ModelRow
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
		return api.ConversationView{}, err
	}
	cs := c.chats
	cs.mu.Lock()
	defer cs.mu.Unlock()
	c.settleChat(key)
	cv := c.newConversation(key, row, model, in.Effort)
	return c.conversationView(key, cv, c.chatFactories(key)), nil
}

// SendChatMessage is the human's send_chat_message. With no current
// Conversation it starts one with the Chat row's defaults. It adds the
// human's Chat Message and a pending reply, and queues the turn for the
// Conversation's Factory, or for the first Factory with Chat support to
// ask when none holds it yet. It is refused while a reply is running and
// while the Conversation is unavailable: messages are never queued.
func (c *Core) SendChatMessage(ctx context.Context, key string, in api.SendChatMessageInput) (api.ConversationView, error) {
	if err := humanOnly(ctx, "send_chat_message"); err != nil {
		return api.ConversationView{}, err
	}
	switch {
	case strings.TrimSpace(in.Text) == "":
		return api.ConversationView{}, invalid("text", "text is required: the Chat Message to send")
	case len(in.Text) > SessionEventTextLimit:
		return api.ConversationView{}, invalid("text", "a Chat Message is at most %d bytes; this one is %d", SessionEventTextLimit, len(in.Text))
	}
	var row domain.ModelRow
	if err := c.store.View(func(r store.Reader) error {
		if err := writable(r, key); err != nil {
			return err
		}
		row = chatRow(r, key)
		return nil
	}); err != nil {
		return api.ConversationView{}, err
	}
	cs := c.chats
	cs.mu.Lock()
	defer cs.mu.Unlock()
	fs := c.settleChat(key)
	cv := cs.all[key]
	if cv == nil {
		if chatAvailability(&conversation{}, fs) != api.ChatAvailable {
			return api.ConversationView{}, c.unavailableChat(key, nil, fs)
		}
		cv = c.newConversation(key, row, "", "")
	}
	switch {
	case cv.running:
		return api.ConversationView{}, &api.Error{Code: api.ActionNotAllowed, AllowedActions: []string{"stop_chat_reply"},
			Message: "a reply is running; wait for it to end, or stop it, before sending another Chat Message"}
	case chatAvailability(cv, fs) != api.ChatAvailable:
		return api.ConversationView{}, c.unavailableChat(key, cv.factory, fs)
	}
	now := c.now()
	cv.messages = append(cv.messages,
		api.ChatMessageView{From: api.ChatFromHuman, Time: now, Text: in.Text},
		api.ChatMessageView{From: api.ChatFromClaude, Time: now})
	cv.emit(ChatEventMessage, cv.messages[len(cv.messages)-2])
	cv.emit(ChatEventMessage, cv.messages[len(cv.messages)-1])
	cv.running, cv.queued, cv.stopAsked, cv.turn = true, true, false, in.Text
	cv.bound()
	c.chatChanged(key)
	cs.wake(key)
	return c.conversationView(key, cv, fs), nil
}

// StopChatReply is the human's stop_chat_reply: it asks the
// Conversation's Factory to stop the running reply. A reply whose turn no
// Factory has taken yet ends stopped at once. With no reply running it
// does nothing.
func (c *Core) StopChatReply(ctx context.Context, key string, _ api.StopChatReplyInput) (api.ConversationView, error) {
	if err := humanOnly(ctx, "stop_chat_reply"); err != nil {
		return api.ConversationView{}, err
	}
	if err := c.store.View(func(r store.Reader) error { return writable(r, key) }); err != nil {
		return api.ConversationView{}, err
	}
	cs := c.chats
	cs.mu.Lock()
	defer cs.mu.Unlock()
	fs := c.settleChat(key)
	cv := cs.all[key]
	if cv != nil && cv.running {
		c.stopReply(key, cv)
	}
	return c.conversationView(key, cv, fs), nil
}

// NextChatWork is the Factory's next_chat_work long-poll. It holds until
// there is work for the Factory, for at most WaitTimeout on the injected
// clock or until ctx ends, and then returns none. A turn of a
// Conversation no Factory holds yet goes to the first call from an
// Online Factory with Chat support, and binds the Conversation to it; a
// later turn or stop goes only to that Factory. Each is handed out once.
// A newer call from the same Factory and host ends this one, empty.
func (c *Core) NextChatWork(ctx context.Context, key string, in api.NextChatWorkInput) (api.ChatWork, error) {
	k, err := c.longPollKey(key, in.Factory, in.Host)
	if err != nil {
		return api.ChatWork{}, err
	}
	cs := c.chats
	p := newLongPoll()
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
		if w, ok := c.takeChatWork(k, p); ok {
			return w, nil
		}
		if timeout == nil {
			timeout = c.clock.After(WaitTimeout)
		}
		select {
		case <-p.wake:
		case <-p.replaced:
			return api.ChatWork{}, nil
		case <-timeout:
			return api.ChatWork{}, nil
		case <-ctx.Done():
			return api.ChatWork{}, nil
		}
	}
}

// takeChatWork hands Factory k's call p its next piece of work, if there
// is one: a stop first, then a queued turn. A call a newer one has
// replaced ends with none, so it can't take work nobody reads.
func (c *Core) takeChatWork(k heartbeatKey, p *longPoll) (api.ChatWork, bool) {
	cs := c.chats
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.polls[k] != p {
		return api.ChatWork{}, true
	}
	fs := c.settleChat(k.project)
	if stops := cs.stops[k]; len(stops) > 0 {
		cs.stops[k] = stops[1:]
		if len(cs.stops[k]) == 0 {
			delete(cs.stops, k)
		}
		return api.ChatWork{Stop: &api.ChatStop{Conversation: stops[0]}}, true
	}
	cv := cs.all[k.project]
	if cv == nil || !cv.queued || cv.factory != nil && *cv.factory != k {
		return api.ChatWork{}, false
	}
	if f, ok := fs[k]; !ok || f.Status != api.FactoryOnline || !f.Chat {
		return api.ChatWork{}, false
	}
	cv.factory, cv.queued = &k, false
	c.chatChanged(k.project)
	return api.ChatWork{Turn: &api.ChatTurn{
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
func (c *Core) PostChatEvents(_ context.Context, key string, in api.PostChatEventsInput) {
	cs := c.chats
	cs.mu.Lock()
	defer cs.mu.Unlock()
	c.settleChat(key)
	cv := cs.all[key]
	from := heartbeatKey{key, strings.TrimSpace(in.Factory), strings.TrimSpace(in.Host)}
	if cv == nil || cv.id != in.Conversation || cv.factory == nil || *cv.factory != from || !cv.running || cv.queued {
		return
	}
	if (in.SessionID != "" && in.SessionID != cv.sessionID) || (in.ResumeCommand != "" && in.ResumeCommand != cv.resumeCommand) {
		cv.sessionID, cv.resumeCommand = cmp.Or(in.SessionID, cv.sessionID), cmp.Or(in.ResumeCommand, cv.resumeCommand)
		cv.emit(ChatEventSession, api.ChatSessionEvent{SessionID: cv.sessionID, ResumeCommand: cv.resumeCommand})
	}
	reply := &cv.messages[len(cv.messages)-1]
	now := c.now()
	for _, e := range in.Items {
		if e.Time.IsZero() {
			e.Time = now
		}
		e.Text, e.Truncated = cutText(e.Text, e.Truncated)
		cv.lastSeq++
		held := api.HeldSessionEvent{Seq: cv.lastSeq, SessionEvent: e}
		reply.Items = append(reply.Items, held)
		cv.emit(ChatEventItem, held)
	}
	switch in.Outcome {
	case api.ChatDone, api.ChatStopped, api.ChatFailed:
		msg, _ := cutText(in.Error, false)
		if in.Outcome != api.ChatFailed {
			msg = ""
		}
		cv.end(in.Outcome, msg)
		// The Factory has ended the turn, so a stop not yet handed out
		// is moot.
		cs.stops[from] = slices.DeleteFunc(cs.stops[from], func(id string) bool { return id == cv.id })
		if len(cs.stops[from]) == 0 {
			delete(cs.stops, from)
		}
		c.chatChanged(key)
	}
	cv.bound()
	cs.wakeFeeds(key)
}

// conversationProject is the Project whose current Conversation has the
// session ID, for filing its usage.
func (c *Core) conversationProject(sessionID string) (string, bool) {
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

// chatAfterHeartbeat runs after a Heartbeat of the Project's: a Factory
// may now take a waiting turn, or the Conversation's availability may
// have changed.
func (c *Core) chatAfterHeartbeat(key string) {
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
func (c *Core) newConversation(key string, row domain.ModelRow, model, effort string) *conversation {
	cs := c.chats
	if old := cs.all[key]; old != nil && old.running {
		c.stopReply(key, old)
	}
	cv := &conversation{
		// Random, not from c.ids, so Claim IDs stay predictable in tests
		// and an ID never repeats across restarts.
		id:     ids.Random{}.ClaimID(),
		model:  cmp.Or(model, row.Model, DefaultChatModel),
		effort: cmp.Or(effort, row.Effort),
	}
	// Events number on from the old Conversation's, so a follower's
	// Last-Event-ID never matches the new one's. With none (after a
	// restart), they start past any the last service gave.
	if old := cs.all[key]; old != nil {
		cv.base = old.evSeq + 1
	} else {
		cv.base = int(c.activity.started.UnixMilli())
	}
	cv.evSeq = cv.base
	cs.all[key] = cv
	// The new Conversation's notice covers its availability too.
	cs.seen[key] = chatAvailability(cv, c.chatFactories(key))
	c.chatChanged(key)
	return cv
}

// stopReply stops cv's running reply: at once while no Factory has taken
// its turn, else by asking the Factory holding it, once. Callers hold
// cs.mu.
func (c *Core) stopReply(key string, cv *conversation) {
	cs := c.chats
	if cv.queued {
		cv.end(api.ChatStopped, "")
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
func (c *Core) settleChat(key string) map[heartbeatKey]api.FactoryView {
	cs := c.chats
	fs := c.chatFactories(key)
	cv := cs.all[key]
	if cv != nil && cv.running && cv.factory != nil {
		if f, ok := fs[*cv.factory]; !ok || f.Status == api.FactoryOffline {
			cv.end(api.ChatFailed, factoryStoppedAnswering)
			c.chatChanged(key)
		}
	}
	// A Project that has never had a Conversation has nothing to announce,
	// and a post naming any other key adds nothing here.
	a := chatAvailability(cv, fs)
	if prev, announced := cs.seen[key]; (announced || cv != nil) && a != prev {
		c.chatChanged(key)
		cs.seen[key] = a
	}
	return fs
}

// chatFactories is the Project's Factories reporting to this service, as
// their Heartbeats stand now: only they can take its Conversations, which
// are held here.
func (c *Core) chatFactories(key string) map[heartbeatKey]api.FactoryView {
	fs := map[heartbeatKey]api.FactoryView{}
	for _, f := range c.heartbeats.view(key, c.clock.Now()).Factories {
		if f.ReportsTo == "" {
			fs[heartbeatKey{key, f.Name, f.Host}] = f
		}
	}
	return fs
}

// chatRunsOn is the host Chat runs on when no Factory of the Project's fs,
// those reporting here, can take a Conversation: the first, by host, of
// the live services an Online Factory with Chat support reports to. It is
// empty when one here can, or none on a live service can. It reads memory
// only, the services as the watch last read them.
func (c *Core) chatRunsOn(key string, fs map[heartbeatKey]api.FactoryView) string {
	for _, f := range fs {
		if answers(f) {
			return ""
		}
	}
	return c.heartbeats.chatHost(key, c.clock.Now())
}

// chatHost is the first, by host, of the live services other than this
// one that an Online Factory of the Project with Chat support reports to,
// or "" when there is none.
func (hs *heartbeats) chatHost(key string, now time.Time) string {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	first := ""
	for k, h := range hs.all {
		if k.project != key || h.service == hs.self || !h.chat || h.status(now) != api.FactoryOnline {
			continue
		}
		if host, ok := hs.live[h.service]; ok && (first == "" || host < first) {
			first = host
		}
	}
	return first
}

// answers says whether f can answer a Conversation now.
func answers(f api.FactoryView) bool { return f.Status == api.FactoryOnline && f.Chat }

// chatAvailability is cv's availability among the Project's Factories fs.
func chatAvailability(cv *conversation, fs map[heartbeatKey]api.FactoryView) api.ChatAvailability {
	if cv == nil {
		return api.ChatNone
	}
	if cv.factory != nil {
		if f, ok := fs[*cv.factory]; ok && answers(f) {
			return api.ChatAvailable
		}
		return api.ChatUnavailable
	}
	for _, f := range fs {
		if answers(f) {
			return api.ChatAvailable
		}
	}
	return api.ChatUnavailable
}

// unavailableChat is send_chat_message's refusal while the Conversation,
// held by factory or by none yet, is unavailable: it names where Chat
// runs when that is another service.
func (c *Core) unavailableChat(key string, factory *heartbeatKey, fs map[heartbeatKey]api.FactoryView) *api.Error {
	e := &api.Error{Code: api.ActionNotAllowed, AllowedActions: []string{"start_conversation"}}
	if host := c.chatRunsOn(key, fs); host != "" {
		e.AllowedActions = nil
		e.Message = fmt.Sprintf("Chat runs on %s: no Online Factory of %s with Chat support reports to this service; send the Chat Message there", host, key)
		return e
	}
	if factory == nil {
		e.AllowedActions = nil
		e.Message = fmt.Sprintf("no Online Factory of %s has Chat support; start code-central-factory without --no-chat", key)
		return e
	}
	f, ok := fs[*factory]
	switch {
	case !ok || f.Status != api.FactoryOnline:
		e.Message = fmt.Sprintf("the Conversation is unavailable: its Factory %s on %s is %s; start a new Conversation, or send again once it is back Online", factory.name, factory.host, cmp.Or(f.Status, api.FactoryOffline))
	default:
		e.Message = fmt.Sprintf("the Conversation is unavailable: its Factory %s on %s no longer has Chat support; start a new Conversation", factory.name, factory.host)
	}
	return e
}

// conversationView is the Project key's current Conversation cv as reads
// return it.
func (c *Core) conversationView(key string, cv *conversation, fs map[heartbeatKey]api.FactoryView) api.ConversationView {
	v := api.ConversationView{Availability: chatAvailability(cv, fs), Messages: []api.ChatMessageView{}, RunsOn: c.chatRunsOn(key, fs)}
	if cv == nil {
		return v
	}
	v.ID, v.Model, v.Effort = cv.id, cv.model, cv.effort
	v.Running, v.Tail = cv.running, cv.tail
	v.SessionID, v.ResumeCommand = cv.sessionID, cv.resumeCommand
	if cv.factory != nil {
		v.Factory = &api.ChatFactory{Name: cv.factory.name, Host: cv.factory.host}
	}
	for _, m := range cv.messages {
		m.Items = append([]api.HeldSessionEvent(nil), m.Items...)
		v.Messages = append(v.Messages, m)
	}
	return v
}

// chatChanged announces a change in the Project's Conversation.
func (c *Core) chatChanged(key string) {
	c.events.Publish(events.Event{Type: events.ChatChanged, Project: key})
	c.chats.wakeFeeds(key)
}

// end ends cv's running reply with outcome, and error text when failed.
func (cv *conversation) end(outcome api.ChatOutcome, errText string) {
	reply := &cv.messages[len(cv.messages)-1]
	reply.Outcome, reply.Error = outcome, errText
	cv.emit(ChatEventOutcome, api.ChatOutcomeEvent{Outcome: outcome, Error: errText})
	cv.running, cv.queued, cv.stopAsked, cv.turn = false, false, false, ""
}

// bound keeps cv to ChatItemLimit items, dropping the oldest whole Chat
// Messages first, and the oldest items of the one left only when it alone
// is over, and marks it as a tail once it has dropped any.
func (cv *conversation) bound() {
	total := 0
	for _, m := range cv.messages {
		total += chatMessageSize(m)
	}
	for total > ChatItemLimit && len(cv.messages) > 1 {
		total -= chatMessageSize(cv.messages[0])
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

// chatRow is the Project's Chat row of its Models.
func chatRow(r store.Reader, key string) domain.ModelRow {
	p, _ := r.Project(key)
	return p.Models.Chat
}
