package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/events"
)

// Chat (Spec code-central-633): the human's get_chat, start_conversation,
// send_chat_message and stop_chat_reply, the Factory's next_chat_work
// long-poll and its posts to /api/p/{key}/chat/events, all over HTTP only.
// A hand-driven Factory takes the turns and posts the replies.
func init() {
	cases["get_chat"] = opCase{
		ok: call{http: httpCall{"GET", "/api/p/acme/chat", ""}},
		check: func(t *testing.T, out map[string]any) {
			t.Helper()
			field("availability", "none")(t, out)
			if m, ok := out["messages"].([]any); !ok || len(m) != 0 {
				t.Fatalf("messages = %v, want []", out["messages"])
			}
		},
		bad:  call{http: httpCall{"GET", "/api/p/nope/chat", ""}},
		code: api.UnknownProject,
	}
	cases["start_conversation"] = opCase{
		setup: chatFactory,
		ok:    call{http: startConversation(`{"model":"opus","effort":"high"}`)},
		check: func(t *testing.T, out map[string]any) {
			t.Helper()
			field("availability", "available")(t, out)
			field("model", "opus")(t, out)
			field("effort", "high")(t, out)
			if out["id"] == "" || out["running"] != false {
				t.Fatalf("conversation %v", out)
			}
		},
		bad:  call{http: startConversation(`{"effort":"extreme"}`)},
		code: api.Invalid,
	}
	cases["send_chat_message"] = opCase{
		setup: chatFactory,
		ok:    call{http: sendChatMessage(`{"text":"What is acme?"}`)},
		check: func(t *testing.T, out map[string]any) {
			t.Helper()
			field("model", "haiku")(t, out)
			if out["running"] != true {
				t.Fatalf("running = %v, want true", out["running"])
			}
			ms, _ := out["messages"].([]any)
			if len(ms) != 2 {
				t.Fatalf("messages = %v, want the human's and the pending reply", out["messages"])
			}
			field("from", "human")(t, ms[0].(map[string]any))
			field("text", "What is acme?")(t, ms[0].(map[string]any))
			field("from", "claude")(t, ms[1].(map[string]any))
		},
		bad:  call{http: sendChatMessage(`{"text":" "}`)},
		code: api.Invalid,
	}
	cases["stop_chat_reply"] = opCase{
		setup: chatMessageSent,
		ok:    call{http: stopChatReply()},
		check: func(t *testing.T, out map[string]any) {
			t.Helper()
			ms, _ := out["messages"].([]any)
			if out["running"] != false || len(ms) != 2 {
				t.Fatalf("conversation %v, want its reply ended", out)
			}
			field("outcome", "stopped")(t, ms[1].(map[string]any))
		},
		bad:  call{http: httpCall{"POST", "/api/p/nope/stop_chat_reply", `{}`}},
		code: api.UnknownProject,
	}
	cases["next_chat_work"] = opCase{
		setup: chatMessageSent,
		ok:    call{http: nextChatWork("f")},
		check: func(t *testing.T, out map[string]any) {
			t.Helper()
			turn, _ := out["turn"].(map[string]any)
			field("text", "What is acme?")(t, turn)
			field("model", "haiku")(t, turn)
			if turn["first"] != true || turn["conversation"] == "" {
				t.Fatalf("turn = %v", out["turn"])
			}
		},
		bad:  call{http: httpCall{"GET", "/api/p/acme/chat/next-work?host=h", ""}},
		code: api.Invalid,
	}
}

func startConversation(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/start_conversation", body}
}

func sendChatMessage(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/send_chat_message", body}
}

func stopChatReply() httpCall { return httpCall{"POST", "/api/p/acme/stop_chat_reply", `{}`} }

func nextChatWork(factory string) httpCall {
	return httpCall{"GET", "/api/p/acme/chat/next-work?factory=" + factory + "&host=h", ""}
}

// chatHeartbeat reports factory on host h, every 60 seconds, so it goes
// Offline three minutes after its last Heartbeat.
func chatHeartbeat(t *testing.T, srv *codecentraltest.Server, factory, state string, chat bool) {
	t.Helper()
	httpOK(t, srv, reportHeartbeat(fmt.Sprintf(`{"factory":%q,"host":"h","state":%q,"interval_seconds":60,"chat":%t}`, factory, state, chat)))
}

// chatFactory makes Factory f an Online Factory of acme with Chat support.
func chatFactory(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	chatHeartbeat(t, srv, "f", "online", true)
}

// chatMessageSent sends a first Chat Message for Factory f to take.
func chatMessageSent(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	chatFactory(t, srv)
	sendChat(t, srv, "What is acme?")
}

func conversationOf(t *testing.T, b []byte) api.ConversationView {
	t.Helper()
	var v api.ConversationView
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("not a Conversation: %v: %s", err, b)
	}
	return v
}

func getChat(t *testing.T, srv *codecentraltest.Server) api.ConversationView {
	t.Helper()
	return conversationOf(t, mustOK(t, srv, httpCall{"GET", "/api/p/acme/chat", ""}))
}

func sendChat(t *testing.T, srv *codecentraltest.Server, text string) api.ConversationView {
	t.Helper()
	body, _ := json.Marshal(api.SendChatMessageInput{Text: text})
	return conversationOf(t, mustOK(t, srv, sendChatMessage(string(body))))
}

// takeWork is factory's next_chat_work when work is already waiting for it.
func takeWork(t *testing.T, srv *codecentraltest.Server, factory string) api.ChatWork {
	t.Helper()
	b := mustOK(t, srv, nextChatWork(factory))
	var w api.ChatWork
	if err := json.Unmarshal(b, &w); err != nil {
		t.Fatalf("not chat work: %v: %s", err, b)
	}
	return w
}

// waitWork opens factory's next_chat_work and returns once it is waiting
// on the clock; its work arrives on the channel.
func waitWork(t *testing.T, srv *codecentraltest.Server, factory string) <-chan api.ChatWork {
	t.Helper()
	timers := srv.Clock.Waiters()
	out := make(chan api.ChatWork, 1)
	go func() {
		req, _ := http.NewRequest("GET", srv.URL+nextChatWork(factory).path, nil)
		var w api.ChatWork
		if resp, err := http.DefaultClient.Do(req); err == nil {
			json.NewDecoder(resp.Body).Decode(&w)
			resp.Body.Close()
		}
		out <- w
	}()
	srv.Clock.BlockUntil(timers + 1)
	return out
}

// received is the work on w, failing the test when there is none.
func received(t *testing.T, w <-chan api.ChatWork) api.ChatWork {
	t.Helper()
	select {
	case got := <-w:
		return got
	case <-time.After(codecentraltest.EventuallyWithin):
		t.Fatal("next_chat_work never returned")
		return api.ChatWork{}
	}
}

// postChat posts to the Chat events path, which always answers 204.
func postChat(t *testing.T, srv *codecentraltest.Server, in api.PostChatEventsInput) {
	t.Helper()
	body, _ := json.Marshal(in)
	postChatBody(t, srv, string(body))
}

func postChatBody(t *testing.T, srv *codecentraltest.Server, body string) {
	t.Helper()
	status, b := httpDo(t, srv, httpCall{"POST", "/api/p/acme/chat/events", body})
	if status != http.StatusNoContent || len(b) != 0 {
		t.Fatalf("POST chat events %s: %d %q, want 204 with no body", body, status, b)
	}
}

// reply posts factory's whole reply to turn: one text item and done.
func reply(t *testing.T, srv *codecentraltest.Server, factory string, turn *api.ChatTurn, text string) {
	t.Helper()
	postChat(t, srv, api.PostChatEventsInput{
		Conversation: turn.Conversation, Factory: factory, Host: "h",
		SessionID: "chat-session", ResumeCommand: "cd /w/acme.worktrees/chat && claude --resume chat-session",
		Items: []api.SessionEvent{{Kind: "text", Text: text}}, Outcome: api.ChatDone,
	})
}

func lastMessage(v api.ConversationView) api.ChatMessageView {
	if len(v.Messages) == 0 {
		return api.ChatMessageView{}
	}
	return v.Messages[len(v.Messages)-1]
}

func TestATurnGoesToExactlyOneOfTwoWaitingFactoriesAndBindsTheConversation(t *testing.T) {
	srv := newServer(t)
	chatHeartbeat(t, srv, "a", "online", true)
	chatHeartbeat(t, srv, "b", "online", true)
	wa, wb := waitWork(t, srv, "a"), waitWork(t, srv, "b")
	sendChat(t, srv, "What is acme?")

	var winner, loser string
	var turn *api.ChatTurn
	var other <-chan api.ChatWork
	select {
	case w := <-wa:
		winner, loser, turn, other = "a", "b", w.Turn, wb
	case w := <-wb:
		winner, loser, turn, other = "b", "a", w.Turn, wa
	case <-time.After(codecentraltest.EventuallyWithin):
		t.Fatal("no Factory was handed the turn")
	}
	if turn == nil || turn.Text != "What is acme?" || !turn.First || turn.SessionID != "" {
		t.Fatalf("turn = %+v, want the first turn", turn)
	}
	srv.Clock.Advance(core.WaitTimeout)
	if w := received(t, other); w.Turn != nil || w.Stop != nil {
		t.Fatalf("%s was handed %+v too", loser, w)
	}
	if v := getChat(t, srv); v.Factory == nil || v.Factory.Name != winner || v.Factory.Host != "h" || v.Availability != api.ChatAvailable {
		t.Fatalf("conversation %+v, want it held by %s", v, winner)
	}

	// The reply's items, outcome, session and Resume Command reach get_chat.
	reply(t, srv, winner, turn, "acme tracks work.")
	v := getChat(t, srv)
	got := lastMessage(v)
	if v.Running || got.Outcome != api.ChatDone || len(got.Items) != 1 || got.Items[0].Text != "acme tracks work." ||
		v.SessionID != "chat-session" || !strings.Contains(v.ResumeCommand, "claude --resume chat-session") {
		t.Fatalf("conversation %+v", v)
	}

	// A later turn goes only to the bound Factory, resuming its session.
	chatHeartbeat(t, srv, "a", "online", true)
	chatHeartbeat(t, srv, "b", "online", true)
	other = waitWork(t, srv, loser)
	sendChat(t, srv, "And its Tickets?")
	next := takeWork(t, srv, winner)
	if next.Turn == nil || next.Turn.First || next.Turn.SessionID != "chat-session" || next.Turn.Text != "And its Tickets?" {
		t.Fatalf("second turn = %+v, want a resume of chat-session", next.Turn)
	}

	// So does a stop.
	httpOK(t, srv, stopChatReply())
	stop := takeWork(t, srv, winner)
	if stop.Stop == nil || stop.Stop.Conversation != turn.Conversation {
		t.Fatalf("work = %+v, want the stop", stop)
	}
	if !getChat(t, srv).Running {
		t.Fatal("the reply ended before its Factory posted stopped")
	}
	postChat(t, srv, api.PostChatEventsInput{Conversation: turn.Conversation, Factory: winner, Host: "h", Outcome: api.ChatStopped})
	if got := lastMessage(getChat(t, srv)); got.Outcome != api.ChatStopped {
		t.Fatalf("reply %+v, want stopped", got)
	}
	srv.Clock.Advance(core.WaitTimeout)
	if w := received(t, other); w.Turn != nil || w.Stop != nil {
		t.Fatalf("%s was handed %+v", loser, w)
	}
}

func TestNextChatWorkTimesOutEmptyOnTheClock(t *testing.T) {
	srv := newServer(t, chatFactory)
	w := waitWork(t, srv, "f")
	srv.Clock.Advance(core.WaitTimeout - time.Second)
	select {
	case got := <-w:
		t.Fatalf("returned before 60 seconds: %+v", got)
	default:
	}
	srv.Clock.Advance(time.Second)
	if got := received(t, w); got.Turn != nil || got.Stop != nil {
		t.Fatalf("got %+v, want no work", got)
	}
}

func TestANewerNextChatWorkFromTheSameFactoryReplacesTheOlder(t *testing.T) {
	srv := newServer(t, chatFactory)
	older := waitWork(t, srv, "f")
	newer := waitWork(t, srv, "f")
	if got := received(t, older); got.Turn != nil {
		t.Fatalf("the replaced call got %+v", got)
	}
	sendChat(t, srv, "What is acme?")
	if got := received(t, newer); got.Turn == nil {
		t.Fatal("the newer call got no turn")
	}
}

func TestATurnIsHandedOutOnce(t *testing.T) {
	srv := newServer(t, chatMessageSent)
	if takeWork(t, srv, "f").Turn == nil {
		t.Fatal("no turn")
	}
	w := waitWork(t, srv, "f")
	srv.Clock.Advance(core.WaitTimeout)
	if got := received(t, w); got.Turn != nil {
		t.Fatalf("the turn was handed out again: %+v", got)
	}
}

func TestAFactoryWithoutChatSupportIsNotHandedATurn(t *testing.T) {
	srv := newServer(t, chatFactory)
	chatHeartbeat(t, srv, "plain", "online", false)
	w := waitWork(t, srv, "plain")
	sendChat(t, srv, "What is acme?")
	srv.Clock.Advance(core.WaitTimeout)
	if got := received(t, w); got.Turn != nil {
		t.Fatalf("a Factory without Chat support got %+v", got)
	}
	if takeWork(t, srv, "f").Turn == nil {
		t.Fatal("the Factory with Chat support got no turn")
	}
}

func TestChatAvailabilityFollowsTheHeartbeats(t *testing.T) {
	srv := newServer(t)
	if v := getChat(t, srv); v.Availability != api.ChatNone {
		t.Fatalf("availability %s, want none", v.Availability)
	}
	if v := conversationOf(t, mustOK(t, srv, startConversation(`{}`))); v.Availability != api.ChatUnavailable {
		t.Fatalf("with no Factory: %s, want unavailable", v.Availability)
	}
	chatFactory(t, srv)
	if v := getChat(t, srv); v.Availability != api.ChatAvailable || v.Factory != nil {
		t.Fatalf("unbound with a Factory: %+v, want available", v)
	}
	turn := chatTurn(t, srv, "What is acme?")
	reply(t, srv, "f", turn, "A Project.")

	for _, step := range []struct {
		state string
		chat  bool
		want  api.ChatAvailability
	}{
		{"draining", true, api.ChatUnavailable},
		{"online", true, api.ChatAvailable},
		{"stopping", true, api.ChatUnavailable},
		{"online", true, api.ChatAvailable},
		{"online", false, api.ChatUnavailable},
		{"online", true, api.ChatAvailable},
	} {
		chatHeartbeat(t, srv, "f", step.state, step.chat)
		if v := getChat(t, srv); v.Availability != step.want {
			t.Fatalf("after %s chat=%t: %s, want %s", step.state, step.chat, v.Availability, step.want)
		}
	}
	// Another Factory doesn't make a held Conversation available.
	chatHeartbeat(t, srv, "g", "online", true)
	srv.Clock.Advance(3 * time.Minute)
	chatHeartbeat(t, srv, "g", "online", true)
	if v := getChat(t, srv); v.Availability != api.ChatUnavailable {
		t.Fatalf("with its Factory Offline: %s, want unavailable", v.Availability)
	}
	chatFactory(t, srv)
	if v := getChat(t, srv); v.Availability != api.ChatAvailable || len(v.Messages) != 2 {
		t.Fatalf("with its Factory back Online: %+v, want available with its messages", v)
	}
}

// chatTurn sends text and has Factory f take its turn.
func chatTurn(t *testing.T, srv *codecentraltest.Server, text string) *api.ChatTurn {
	t.Helper()
	sendChat(t, srv, text)
	w := takeWork(t, srv, "f")
	if w.Turn == nil {
		t.Fatalf("no turn for %q", text)
	}
	return w.Turn
}

// mustOK is httpOK for a result read into a core type: its raw body.
func mustOK(t *testing.T, srv *codecentraltest.Server, c httpCall) []byte {
	t.Helper()
	status, b := httpDo(t, srv, c)
	if status != http.StatusOK {
		t.Fatalf("%s %s: status %d: %s", c.method, c.path, status, b)
	}
	return b
}

func TestSendIsRefusedWhileAReplyRunsAndWhileTheConversationIsUnavailable(t *testing.T) {
	srv := newServer(t)
	e := httpRejected(t, srv, sendChatMessage(`{"text":"Anyone?"}`), api.ActionNotAllowed)
	if !strings.Contains(e.Message, "Chat support") {
		t.Errorf("with no Factory: %q", e.Message)
	}
	if v := getChat(t, srv); v.Availability != api.ChatNone {
		t.Fatalf("a refused send started a Conversation: %+v", v)
	}

	chatFactory(t, srv)
	turn := chatTurn(t, srv, "What is acme?")
	e = httpRejected(t, srv, sendChatMessage(`{"text":"Hello?"}`), api.ActionNotAllowed)
	if !slices.Equal(e.AllowedActions, []string{"stop_chat_reply"}) {
		t.Errorf("while running: %+v", e)
	}
	reply(t, srv, "f", turn, "A Project.")

	chatHeartbeat(t, srv, "f", "draining", true)
	e = httpRejected(t, srv, sendChatMessage(`{"text":"Hello?"}`), api.ActionNotAllowed)
	if !strings.Contains(e.Message, "unavailable") || !strings.Contains(e.Message, "draining") || !slices.Equal(e.AllowedActions, []string{"start_conversation"}) {
		t.Errorf("while unavailable: %+v", e)
	}
	if v := getChat(t, srv); len(v.Messages) != 2 {
		t.Fatalf("a refused send added messages: %+v", v.Messages)
	}
}

func TestAnOverLongChatMessageIsRefused(t *testing.T) {
	srv := newServer(t, chatFactory)
	body, _ := json.Marshal(api.SendChatMessageInput{Text: strings.Repeat("x", core.SessionEventTextLimit+1)})
	if e := httpRejected(t, srv, sendChatMessage(string(body)), api.Invalid); e.Field != "text" {
		t.Fatalf("field %q, want text", e.Field)
	}
	if v := sendChat(t, srv, strings.Repeat("x", core.SessionEventTextLimit)); len(v.Messages) != 2 {
		t.Fatalf("a message of exactly the limit: %+v", v)
	}
}

func TestAConversationDropsWholeOldMessagesAndMarksATail(t *testing.T) {
	srv := newServer(t, chatFactory)
	items := func(n int) []api.SessionEvent {
		out := make([]api.SessionEvent, n)
		for i := range out {
			out[i] = api.SessionEvent{Kind: "tool_call", Tool: "Bash", Text: fmt.Sprint(i)}
		}
		return out
	}
	first := chatTurn(t, srv, "one")
	postChat(t, srv, api.PostChatEventsInput{Conversation: first.Conversation, Factory: "f", Host: "h", SessionID: "s", Items: items(1500), Outcome: api.ChatDone})
	if v := getChat(t, srv); v.Tail || len(v.Messages) != 2 {
		t.Fatalf("under the bound: tail %t, %d messages", v.Tail, len(v.Messages))
	}

	second := chatTurn(t, srv, "two")
	long := strings.Repeat("y", core.SessionEventTextLimit+10)
	postChat(t, srv, api.PostChatEventsInput{Conversation: second.Conversation, Factory: "f", Host: "h", Items: append(items(599), api.SessionEvent{Kind: "text", Text: long})})
	v := getChat(t, srv)
	if !v.Tail || len(v.Messages) != 2 || v.Messages[0].Text != "two" || len(v.Messages[1].Items) != 600 {
		t.Fatalf("over the bound: tail %t, messages %d, first %q", v.Tail, len(v.Messages), v.Messages[0].Text)
	}
	cut := v.Messages[1].Items[599]
	if !cut.Truncated || len(cut.Text) != core.SessionEventTextLimit {
		t.Fatalf("the long item: truncated %t, %d bytes", cut.Truncated, len(cut.Text))
	}
	if !v.Running {
		t.Fatal("dropping messages ended the running reply")
	}
}

func TestChatPostsFromTheWrongFactoryOrForAReplacedConversationAreDropped(t *testing.T) {
	srv := newServer(t, chatFactory)
	chatHeartbeat(t, srv, "g", "online", true)
	turn := chatTurn(t, srv, "What is acme?")
	postChat(t, srv, api.PostChatEventsInput{Conversation: turn.Conversation, Factory: "g", Host: "h", Items: []api.SessionEvent{{Kind: "text", Text: "Not mine."}}, Outcome: api.ChatDone})
	if v := getChat(t, srv); !v.Running || len(lastMessage(v).Items) != 0 {
		t.Fatalf("a post from the wrong Factory was kept: %+v", v)
	}
	for _, body := range []string{`{`, `not json`, `{}`, `{"conversation":7}`} {
		postChatBody(t, srv, body)
	}

	// A new Conversation stops the old one's running reply first.
	fresh := conversationOf(t, mustOK(t, srv, startConversation(`{}`)))
	if fresh.ID == turn.Conversation || len(fresh.Messages) != 0 || fresh.Running {
		t.Fatalf("new Conversation %+v", fresh)
	}
	if w := takeWork(t, srv, "f"); w.Stop == nil || w.Stop.Conversation != turn.Conversation {
		t.Fatalf("work = %+v, want the old reply's stop", w)
	}
	reply(t, srv, "f", turn, "Too late.")
	if v := getChat(t, srv); v.ID != fresh.ID || len(v.Messages) != 0 || v.SessionID != "" {
		t.Fatalf("a post for the replaced Conversation was kept: %+v", v)
	}
}

func TestARunningReplyFailsWhenItsFactoryGoesOffline(t *testing.T) {
	srv := newServer(t, chatFactory)
	// A turn handed out but never answered.
	chatTurn(t, srv, "What is acme?")
	srv.Clock.Advance(3 * time.Minute)
	v := getChat(t, srv)
	if got := lastMessage(v); v.Running || got.Outcome != api.ChatFailed || got.Error != "the Factory stopped answering" {
		t.Fatalf("reply %+v, want failed", got)
	}

	// A reply that had begun, when its Factory stops.
	chatFactory(t, srv)
	turn := chatTurn(t, srv, "Again?")
	postChat(t, srv, api.PostChatEventsInput{Conversation: turn.Conversation, Factory: "f", Host: "h", Items: []api.SessionEvent{{Kind: "text", Text: "Let me"}}})
	chatHeartbeat(t, srv, "f", "stopping", true)
	if got := lastMessage(getChat(t, srv)); got.Outcome != api.ChatFailed || len(got.Items) != 1 {
		t.Fatalf("reply %+v, want failed keeping its items", got)
	}
	// A late post for it is dropped.
	chatFactory(t, srv)
	reply(t, srv, "f", turn, "done now")
	if got := lastMessage(getChat(t, srv)); got.Outcome != api.ChatFailed || len(got.Items) != 1 {
		t.Fatalf("reply %+v, want the late post dropped", got)
	}
}

func TestStopEndsAReplyNoFactoryHasTakenAtOnceAndIsANoOpWithNoneRunning(t *testing.T) {
	srv := newServer(t, chatFactory)
	if v := conversationOf(t, mustOK(t, srv, stopChatReply())); v.Availability != api.ChatNone {
		t.Fatalf("stop with no Conversation: %+v", v)
	}
	sendChat(t, srv, "What is acme?")
	if v := conversationOf(t, mustOK(t, srv, stopChatReply())); v.Running || lastMessage(v).Outcome != api.ChatStopped {
		t.Fatalf("stop before any Factory took the turn: %+v", v)
	}
	w := waitWork(t, srv, "f")
	srv.Clock.Advance(core.WaitTimeout)
	if got := received(t, w); got.Turn != nil || got.Stop != nil {
		t.Fatalf("the stopped turn was handed out: %+v", got)
	}
	if v := conversationOf(t, mustOK(t, srv, stopChatReply())); lastMessage(v).Outcome != api.ChatStopped {
		t.Fatalf("a second stop changed the reply: %+v", v)
	}
}

func TestAStopIsDroppedOnceTheFactoryHasEndedTheReply(t *testing.T) {
	srv := newServer(t, chatFactory)
	turn := chatTurn(t, srv, "What is acme?")
	httpOK(t, srv, stopChatReply())
	// The reply ends before the Factory next asks for work.
	reply(t, srv, "f", turn, "A Project.")
	w := waitWork(t, srv, "f")
	srv.Clock.Advance(core.WaitTimeout)
	if got := received(t, w); got.Stop != nil {
		t.Fatalf("a stop for an ended reply was handed out: %+v", got)
	}
}

func TestANewConversationTakesTheChatRowOrHaiku(t *testing.T) {
	srv := newServer(t, chatFactory)
	if v := conversationOf(t, mustOK(t, srv, startConversation(`{}`))); v.Model != "haiku" || v.Effort != "" {
		t.Fatalf("with no Chat row: model %q effort %q, want haiku and none", v.Model, v.Effort)
	}
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/set_models", `{"default":{"model":"opus","effort":"max"},"chat":{"model":"sonnet","effort":"low"}}`})
	if v := conversationOf(t, mustOK(t, srv, startConversation(`{}`))); v.Model != "sonnet" || v.Effort != "low" {
		t.Fatalf("with the Chat row: model %q effort %q, want sonnet and low", v.Model, v.Effort)
	}
	if v := conversationOf(t, mustOK(t, srv, startConversation(`{"model":"claude-opus-5-5"}`))); v.Model != "claude-opus-5-5" || v.Effort != "low" {
		t.Fatalf("with a picked model: model %q effort %q", v.Model, v.Effort)
	}
	for field, body := range map[string]string{
		"effort": `{"effort":"extreme"}`,
		"model":  `{"model":"opus\nsonnet"}`,
	} {
		if e := httpRejected(t, srv, startConversation(body), api.Invalid); e.Field != field {
			t.Errorf("%s: field %q", body, e.Field)
		}
	}

	// The first Chat Message with no Conversation starts one from the Chat row.
	srv = newServer(t, chatFactory)
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/set_models", `{"chat":{"model":"sonnet","effort":"low"}}`})
	if v := sendChat(t, srv, "What is acme?"); v.Model != "sonnet" || v.Effort != "low" {
		t.Fatalf("auto-started: model %q effort %q", v.Model, v.Effort)
	}
	if turn := takeWork(t, srv, "f").Turn; turn == nil || turn.Model != "sonnet" || turn.Effort != "low" {
		t.Fatalf("turn %+v, want sonnet and low", turn)
	}
}

func TestEveryConversationChangeSendsChatChanged(t *testing.T) {
	srv := newServer(t, chatFactory)
	notices, stop := srv.Core.Events().Subscribe()
	defer stop()
	next := func(what string) {
		t.Helper()
		for {
			select {
			case e := <-notices:
				if e.Type == events.ChatChanged {
					if e.Project != "acme" || e.ID != "" {
						t.Fatalf("%s: notice %+v", what, e)
					}
					return
				}
			case <-time.After(codecentraltest.EventuallyWithin):
				t.Fatalf("%s: no chat.changed", what)
			}
		}
	}
	mustOK(t, srv, startConversation(`{}`))
	next("a new Conversation")
	turn := chatTurn(t, srv, "What is acme?")
	next("a reply starting")
	reply(t, srv, "f", turn, "A Project.")
	next("a reply ending")
	for len(notices) > 0 {
		<-notices
	}
	chatHeartbeat(t, srv, "f", "draining", true)
	next("the Conversation becoming unavailable")
}

func TestChatUsageCountsAtProjectScopeUnderTheKindChat(t *testing.T) {
	srv := newServer(t, chatFactory)
	turn := chatTurn(t, srv, "What is acme?")
	reply(t, srv, "f", turn, "A Project.")
	postResponses(t, srv, "chat-session", response{time: at(1), kind: "chat", model: "claude-haiku-5-5", input: 10, write: 100})

	u := readUsage(t, srv, "?project=acme")
	if len(u.Sessions) != 1 {
		t.Fatalf("sessions %+v, want the Conversation's", u.Sessions)
	}
	s := u.Sessions[0]
	if s.Project != "acme" || s.Ticket != "" || s.Claim != 0 || s.Kind != "chat" || s.SessionID != "chat-session" {
		t.Fatalf("session %+v, want acme's chat session with no Ticket", s)
	}
	if len(u.Kinds) != 1 || u.Kinds[0].Kind != "chat" {
		t.Fatalf("kinds %+v, want chat", u.Kinds)
	}
	p, err := srv.Core.GetProject(context.Background(), "acme")
	if err != nil || !slices.Contains(p.ReportedModels.ModelIDs, "claude-haiku-5-5") {
		t.Fatalf("reported models %+v, %v, want the chat model", p.ReportedModels.ModelIDs, err)
	}
}
