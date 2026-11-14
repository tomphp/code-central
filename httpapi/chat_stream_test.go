package httpapi_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// chatFrame is one SSE event on a Conversation stream.
type chatFrame struct {
	id, name, data string
}

type chatFixture struct {
	h    http.Handler
	hs   string
	turn *core.ChatTurn
}

func newChatFixture(t *testing.T) *chatFixture {
	t.Helper()
	clk := clock.NewFake(time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC))
	h := asHuman(serve(store.NewMemory(), clk))
	f := &chatFixture{h: h, hs: liveServer(t, h).URL}
	f.ok(t, "POST", "/api/projects/create_project", `{"key":"acme"}`)
	f.ok(t, "POST", "/api/p/acme/report_heartbeat", `{"factory":"f","host":"h","state":"online","interval_seconds":60,"chat":true}`)
	return f
}

func (f *chatFixture) ok(t *testing.T, method, path, body string) []byte {
	t.Helper()
	rec := do(f.h, method, path, body)
	if rec.Code != 200 {
		t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body)
	}
	return rec.Body.Bytes()
}

// send sends a Chat Message, and has Factory f take its turn when none
// has yet, so it holds the Conversation and may post to it.
func (f *chatFixture) send(t *testing.T, text string) {
	t.Helper()
	got := make(chan core.ChatWork, 1)
	go func() {
		var w core.ChatWork
		json.Unmarshal(do(f.h, "GET", "/api/p/acme/chat/next-work?factory=f&host=h", "").Body.Bytes(), &w)
		got <- w
	}()
	// The call may not be waiting yet; the turn is queued either way.
	f.ok(t, "POST", "/api/p/acme/send_chat_message", `{"text":`+strconv.Quote(text)+`}`)
	select {
	case w := <-got:
		if w.Turn == nil {
			t.Fatalf("work %+v, want a turn", w)
		}
		f.turn = w.Turn
	case <-time.After(time.Second):
		t.Fatal("the Factory was handed no turn")
	}
}

func (f *chatFixture) post(t *testing.T, items []string, outcome string) {
	t.Helper()
	var evs []string
	for _, s := range items {
		evs = append(evs, `{"kind":"text","text":`+strconv.Quote(s)+`}`)
	}
	body := `{"conversation":"` + f.turn.Conversation + `","factory":"f","host":"h","items":[` + strings.Join(evs, ",") + `],"outcome":"` + outcome + `"}`
	if rec := do(f.h, "POST", "/api/p/acme/chat/events", body); rec.Code != 204 {
		t.Fatalf("chat events: %d %s", rec.Code, rec.Body)
	}
}

func (f *chatFixture) stream(t *testing.T, lastID string) *bufio.Reader {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	header := http.Header{}
	if lastID != "" {
		header.Set("Last-Event-ID", lastID)
	}
	return openSSE(t, ctx, f.hs+"/api/p/acme/chat/stream", header)
}

func nextChatFrame(t *testing.T, r *bufio.Reader) chatFrame {
	t.Helper()
	got := make(chan chatFrame, 1)
	failed := make(chan error, 1)
	go func() {
		var f chatFrame
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				failed <- err
				return
			}
			line = strings.TrimRight(line, "\n")
			switch {
			case line == "":
				got <- f
				return
			case strings.HasPrefix(line, "id: "):
				f.id = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "event: "):
				f.name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				f.data = strings.TrimPrefix(line, "data: ")
			}
		}
	}()
	select {
	case f := <-got:
		return f
	case err := <-failed:
		t.Fatalf("stream ended: %v", err)
	case <-time.After(time.Second):
		t.Fatal("no event within a second")
	}
	return chatFrame{}
}

// opening reads the opening conversation event.
func opening(t *testing.T, r *bufio.Reader) (chatFrame, core.ConversationView) {
	t.Helper()
	fr := nextChatFrame(t, r)
	var v core.ConversationView
	if fr.name != "conversation" {
		t.Fatalf("got %s %s, want the conversation", fr.name, fr.data)
	}
	json.Unmarshal([]byte(fr.data), &v)
	return fr, v
}

// wantItems reads one item event per text, ids rising by one.
func wantChatItems(t *testing.T, r *bufio.Reader, texts ...string) (last int) {
	t.Helper()
	prev := 0
	for _, text := range texts {
		fr := nextChatFrame(t, r)
		var it core.HeldSessionEvent
		json.Unmarshal([]byte(fr.data), &it)
		id, _ := strconv.Atoi(fr.id)
		if fr.name != "item" || it.Text != text || id == 0 || (prev != 0 && id != prev+1) {
			t.Fatalf("got %s id %s %s, want item %q after %d", fr.name, fr.id, fr.data, text, prev)
		}
		prev = id
	}
	return prev
}

func wantOutcome(t *testing.T, r *bufio.Reader, outcome core.ChatOutcome) int {
	t.Helper()
	fr := nextChatFrame(t, r)
	var o core.ChatOutcomeEvent
	json.Unmarshal([]byte(fr.data), &o)
	id, _ := strconv.Atoi(fr.id)
	if fr.name != "outcome" || o.Outcome != outcome || id == 0 {
		t.Fatalf("got %s id %s %s, want outcome %s", fr.name, fr.id, fr.data, outcome)
	}
	return id
}

func wantMessages(t *testing.T, r *bufio.Reader, from ...string) {
	t.Helper()
	for _, want := range from {
		fr := nextChatFrame(t, r)
		var m core.ChatMessageView
		json.Unmarshal([]byte(fr.data), &m)
		if fr.name != "message" || fr.id == "" || m.From != want {
			t.Fatalf("got %s id %s %s, want a message from %s", fr.name, fr.id, fr.data, want)
		}
	}
}

func TestAChatStreamOpensWithNoConversationThenWithTheOneThatStarts(t *testing.T) {
	f := newChatFixture(t)
	r := f.stream(t, "")
	fr, v := opening(t, r)
	if fr.id != "" || v.Availability != core.ChatNone {
		t.Fatalf("opening %+v %+v, want none with no id", fr, v)
	}
	f.ok(t, "POST", "/api/p/acme/start_conversation", `{}`)
	fr = nextChatFrame(t, r)
	if fr.name != "conversation" || fr.id == "" {
		t.Fatalf("got %+v, want the new Conversation", fr)
	}
}

func TestAChatStreamOpensWithTheConversationAsItStandsThenSendsEachNewItemAndOutcome(t *testing.T) {
	f := newChatFixture(t)
	f.send(t, "hello")
	f.post(t, []string{"one"}, "")

	r := f.stream(t, "")
	fr, v := opening(t, r)
	if len(v.Messages) != 2 || v.Messages[0].Text != "hello" || len(v.Messages[1].Items) != 1 || !v.Running || fr.id == "" {
		t.Fatalf("opening %+v %+v, want the Conversation as it stands", fr, v)
	}
	f.post(t, []string{"two", "three"}, "")
	wantChatItems(t, r, "two", "three")
	f.post(t, nil, "done")
	wantOutcome(t, r, core.ChatDone)

	// A second message streams as two Chat Messages, then its items.
	f.send(t, "again")
	wantMessages(t, r, "human", "claude")
	f.post(t, []string{"four"}, "")
	wantChatItems(t, r, "four")
}

func TestAChatStreamWithLastEventIDResumesWithNoGapsOrRepeats(t *testing.T) {
	f := newChatFixture(t)
	f.send(t, "hello")
	r := f.stream(t, "")
	fr, _ := opening(t, r)
	f.post(t, []string{"one", "two"}, "")
	last := wantChatItems(t, r, "one", "two")
	// The connection drops; more arrives while it is down.
	f.post(t, []string{"three"}, "failed")

	r = f.stream(t, strconv.Itoa(last))
	wantChatItems(t, r, "three")
	wantOutcome(t, r, core.ChatFailed)
	if fr.id == "" {
		t.Fatal("the opening had no id to resume from")
	}

	// Resuming from the opening's id gets everything after it.
	r = f.stream(t, fr.id)
	wantChatItems(t, r, "one", "two", "three")
}

func TestAChatStreamSendsTheConversationAgainWhenTheEventsItMissedAreGone(t *testing.T) {
	f := newChatFixture(t)
	f.send(t, "hello")
	r := f.stream(t, "")
	fr, _ := opening(t, r)
	var batch []string
	for i := 0; i < core.ChatItemLimit+5; i++ {
		batch = append(batch, strconv.Itoa(i))
	}
	f.post(t, batch, "")

	_, v := opening(t, f.stream(t, fr.id))
	if !v.Tail || len(v.Messages) == 0 {
		t.Fatalf("got %+v, want the Conversation, as a tail", v)
	}
}

func TestAChatStreamSendsReplacedWhenTheConversationIsReplaced(t *testing.T) {
	f := newChatFixture(t)
	f.send(t, "hello")
	r := f.stream(t, "")
	old, _ := opening(t, r)

	f.ok(t, "POST", "/api/p/acme/start_conversation", `{}`)
	// The old reply was stopped; the stream says so, then replaced.
	for {
		fr := nextChatFrame(t, r)
		if fr.name == "replaced" {
			if fr.id != "" {
				t.Fatalf("replaced has id %s, want none", fr.id)
			}
			break
		}
		if fr.name != "outcome" {
			t.Fatalf("got %+v before replaced", fr)
		}
	}
	_, v := opening(t, r)
	if len(v.Messages) != 0 {
		t.Fatalf("got %+v, want the new, empty Conversation", v)
	}

	// A client that reconnects with an id from the old Conversation is
	// told too, and its id never matches an event of the new one.
	r = f.stream(t, old.id)
	if fr := nextChatFrame(t, r); fr.name != "replaced" {
		t.Fatalf("got %+v, want replaced", fr)
	}
	opening(t, r)
}

func TestAChatStreamOfAnUnknownProjectIsNotFound(t *testing.T) {
	f := newChatFixture(t)
	rec := do(f.h, "GET", "/api/p/nope/chat/stream", "")
	var e core.Error
	json.Unmarshal(rec.Body.Bytes(), &e)
	if rec.Code != 404 || e.Code != core.UnknownProject {
		t.Fatalf("%d %s, want 404 unknown_project", rec.Code, rec.Body)
	}
}
