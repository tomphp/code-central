package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// transcriptFixture is the HTTP API over a fake clock, with Project acme,
// its Spec acme-1 and Tickets acme-2 and acme-3 Ready to claim.
type transcriptFixture struct {
	h     http.Handler
	clock *clock.Fake
	store store.Store
}

func newTranscriptFixture(t *testing.T) transcriptFixture {
	t.Helper()
	s := store.NewMemory()
	clk := clock.NewFake(time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC))
	f := transcriptFixture{h: serve(s, clk), clock: clk, store: s}
	f.ok(t, "POST", "/api/projects/create_project", `{"key":"acme"}`)
	f.ok(t, "POST", "/api/p/acme/create_spec", `{"title":"S"}`)
	f.ok(t, "POST", "/api/p/acme/create_ticket", `{"spec":1,"title":"A","state":"Ready"}`)
	f.ok(t, "POST", "/api/p/acme/create_ticket", `{"spec":1,"title":"B","state":"Ready"}`)
	return f
}

func (f transcriptFixture) ok(t *testing.T, method, path, body string) []byte {
	t.Helper()
	rec := do(f.h, method, path, body)
	if rec.Code != 200 {
		t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body)
	}
	return rec.Body.Bytes()
}

// claim claims Ticket n for session and returns the Claim ID.
func (f transcriptFixture) claim(t *testing.T, n int, session string) string {
	t.Helper()
	var res core.ClaimResult
	json.Unmarshal(f.ok(t, "POST", "/api/p/acme/claim", fmt.Sprintf(`{"ticket":%d,"session_id":%q}`, n, session)), &res)
	return res.ClaimID
}

func (f transcriptFixture) post(t *testing.T, body string) {
	t.Helper()
	if rec := do(f.h, "POST", "/api/session-events", body); rec.Code != 204 || rec.Body.Len() != 0 {
		t.Fatalf("session-events %s: %d %q, want 204 with no body", body, rec.Code, rec.Body)
	}
}

// events posts one text event per text for session.
func (f transcriptFixture) events(t *testing.T, session string, texts ...string) {
	t.Helper()
	var evs []string
	for _, text := range texts {
		evs = append(evs, fmt.Sprintf(`{"time":"2026-11-07T09:00:00Z","kind":"text","text":%q}`, text))
	}
	f.post(t, fmt.Sprintf(`{"session_id":%q,"events":[%s]}`, session, strings.Join(evs, ",")))
}

type heldEvent struct {
	Seq       int       `json:"seq"`
	Time      time.Time `json:"time"`
	Kind      string    `json:"kind"`
	Text      string    `json:"text"`
	Tool      string    `json:"tool"`
	ToolUseID string    `json:"tool_use_id"`
	IsError   bool      `json:"is_error"`
	Truncated bool      `json:"truncated"`
}

type transcript struct {
	Events []heldEvent `json:"events"`
	Tail   bool        `json:"tail"`
}

func (f transcriptFixture) transcript(t *testing.T, ticket, claim int) transcript {
	t.Helper()
	body := f.ok(t, "GET", fmt.Sprintf("/api/projects/acme/tickets/%d/claims/%d/transcript", ticket, claim), "")
	var tr transcript
	if err := json.Unmarshal(body, &tr); err != nil {
		t.Fatal(err)
	}
	if tr.Events == nil {
		t.Fatalf("events is %s, want a list", body)
	}
	return tr
}

func texts(tr transcript) []string {
	out := []string{}
	for _, e := range tr.Events {
		out = append(out, e.Text)
	}
	return out
}

func (f transcriptFixture) lastActivity(t *testing.T, ticket, claim int) time.Time {
	t.Helper()
	var v core.TicketView
	json.Unmarshal(f.ok(t, "GET", fmt.Sprintf("/api/p/acme/tickets/%d", ticket), ""), &v)
	return v.Claims[claim-1].LastActivity
}

func TestSessionEventsAlwaysAnswer204(t *testing.T) {
	f := newTranscriptFixture(t)
	f.claim(t, 2, "s1")
	for _, body := range []string{
		`{"session_id":"s1","events":[{"time":"2026-11-07T09:00:00Z","kind":"text","text":"hi"}]}`,
		`{"session_id":"unknown","events":[{"time":"2026-11-07T09:00:00Z","kind":"text","text":"hi"}]}`,
		`{"session_id":"s1","events":[]}`,
		`{"session_id":"s1"}`,
		`{}`,
		`{`,
		`{"other":1}`,
		`{"session_id":"s1","events":[{"kind":7}]}`,
	} {
		f.post(t, body)
	}
	if rec := do(f.h, "GET", "/api/session-events", ""); rec.Code == 204 {
		t.Error("GET /api/session-events should not post: GET never writes")
	}
	if got := texts(f.transcript(t, 2, 1)); strings.Join(got, ",") != "hi" {
		t.Fatalf("Transcript %q, want only the readable batch for s1", got)
	}
}

func TestSessionEventsKeepEveryFieldInOrderWithARisingSeq(t *testing.T) {
	f := newTranscriptFixture(t)
	f.claim(t, 2, "s1")
	f.post(t, `{"session_id":"s1","events":[
		{"time":"2026-11-07T09:00:01Z","kind":"text","text":"Running the tests."},
		{"time":"2026-11-07T09:00:02Z","kind":"tool_call","text":"Bash: go test ./...","tool":"Bash","tool_use_id":"tu1"}
	]}`)
	f.post(t, `{"session_id":"s1","events":[
		{"time":"2026-11-07T09:00:03Z","kind":"tool_result","text":"FAIL","tool":"Bash","tool_use_id":"tu1","is_error":true,"truncated":true},
		{"time":"2026-11-07T09:00:04Z","kind":"anything at all","text":"opaque"}
	]}`)

	tr := f.transcript(t, 2, 1)
	at := func(s int) time.Time { return time.Date(2026, 11, 7, 9, 0, s, 0, time.UTC) }
	want := []heldEvent{
		{Seq: 1, Time: at(1), Kind: "text", Text: "Running the tests."},
		{Seq: 2, Time: at(2), Kind: "tool_call", Text: "Bash: go test ./...", Tool: "Bash", ToolUseID: "tu1"},
		{Seq: 3, Time: at(3), Kind: "tool_result", Text: "FAIL", Tool: "Bash", ToolUseID: "tu1", IsError: true, Truncated: true},
		{Seq: 4, Time: at(4), Kind: "anything at all", Text: "opaque"},
	}
	if len(tr.Events) != len(want) {
		t.Fatalf("events %+v, want %+v", tr.Events, want)
	}
	for i := range want {
		if tr.Events[i] != want[i] {
			t.Errorf("event %d: %+v, want %+v", i, tr.Events[i], want[i])
		}
	}
	if tr.Tail {
		t.Error("a whole Transcript is not a tail")
	}

	body := string(f.ok(t, "GET", "/api/projects/acme/tickets/2/claims/1/transcript", ""))
	if strings.Count(body, "tool_use_id") != 2 || strings.Contains(body, `"is_error":false`) {
		t.Fatalf("optional fields should be left out when unset: %s", body)
	}
}

func TestSessionEventsGoToTheNewestClaimWithThatSessionEvenAnEndedOne(t *testing.T) {
	f := newTranscriptFixture(t)
	id := f.claim(t, 2, "s1")
	f.events(t, "s1", "first")
	f.clock.Advance(time.Minute)
	f.ok(t, "POST", "/api/p/acme/tickets/2/finish", fmt.Sprintf(`{"claim_id":%q,"summary":"done"}`, id))
	f.events(t, "s1", "turn ended") // the Claim has ended: still its Transcript

	f.clock.Advance(time.Minute)
	f.claim(t, 3, "s1") // the same session claims again: now the newest
	f.events(t, "s1", "on the next one")
	f.events(t, "nobody", "dropped")

	if got := strings.Join(texts(f.transcript(t, 2, 1)), ","); got != "first,turn ended" {
		t.Fatalf("acme-2 Claim 1: %q", got)
	}
	if got := strings.Join(texts(f.transcript(t, 3, 1)), ","); got != "on the next one" {
		t.Fatalf("acme-3 Claim 1: %q", got)
	}
}

func TestSessionEventsCountAsActivityOnlyOnLiveClaims(t *testing.T) {
	f := newTranscriptFixture(t)
	id := f.claim(t, 2, "s1")
	f.clock.Advance(10 * time.Minute)
	posted := f.clock.Now()
	f.events(t, "s1", "working")
	if got := f.lastActivity(t, 2, 1); !got.Equal(posted) {
		t.Fatalf("live Claim's last_activity %v, want the post at %v", got, posted)
	}
	f.clock.Advance(5 * time.Minute)
	posted = f.clock.Now()
	f.post(t, `{"session_id":"s1","events":[]}`) // an empty batch is still a ping
	if got := f.lastActivity(t, 2, 1); !got.Equal(posted) {
		t.Fatalf("live Claim's last_activity %v, want the empty post at %v", got, posted)
	}

	// Finishing ends the Claim; a later post still lands on it, but marks
	// no activity.
	f.clock.Advance(10 * time.Minute)
	f.ok(t, "POST", "/api/p/acme/tickets/2/finish", fmt.Sprintf(`{"claim_id":%q,"summary":"done"}`, id))
	ended := f.lastActivity(t, 2, 1)
	f.clock.Advance(10 * time.Minute)
	f.events(t, "s1", "after the end")
	if got := f.lastActivity(t, 2, 1); !got.Equal(ended) {
		t.Fatalf("ended Claim's last_activity %v, want it unchanged at %v", got, ended)
	}
	if got := strings.Join(texts(f.transcript(t, 2, 1)), ","); got != "working,after the end" {
		t.Fatalf("Transcript %q", got)
	}
}

func TestATranscriptKeepsTheLast2000EventsAndIsThenATail(t *testing.T) {
	f := newTranscriptFixture(t)
	f.claim(t, 2, "s1")
	var batch []string
	for i := 1; i <= core.TranscriptLimit; i++ {
		batch = append(batch, fmt.Sprint(i))
	}
	f.events(t, "s1", batch...)
	if tr := f.transcript(t, 2, 1); len(tr.Events) != 2000 || tr.Tail {
		t.Fatalf("%d events, tail %v: want all 2,000 and no tail", len(tr.Events), tr.Tail)
	}

	f.events(t, "s1", "2001", "2002")
	tr := f.transcript(t, 2, 1)
	if len(tr.Events) != 2000 || !tr.Tail {
		t.Fatalf("%d events, tail %v: want 2,000 and a tail", len(tr.Events), tr.Tail)
	}
	first, last := tr.Events[0], tr.Events[len(tr.Events)-1]
	if first.Seq != 3 || first.Text != "3" || last.Seq != 2002 || last.Text != "2002" {
		t.Fatalf("held %d %q .. %d %q, want 3 .. 2002", first.Seq, first.Text, last.Seq, last.Text)
	}
}

func TestASessionEventsTextIsCutTo64KB(t *testing.T) {
	f := newTranscriptFixture(t)
	f.claim(t, 2, "s1")
	exact := strings.Repeat("a", core.SessionEventTextLimit)
	// A multi-byte rune straddling the limit is dropped whole.
	over := strings.Repeat("b", core.SessionEventTextLimit-1) + "é" + "tail"
	f.events(t, "s1", exact, over)

	tr := f.transcript(t, 2, 1)
	if e := tr.Events[0]; e.Text != exact || e.Truncated {
		t.Fatalf("a 64 KB text was changed: %d bytes, truncated %v", len(e.Text), e.Truncated)
	}
	if e := tr.Events[1]; e.Text != strings.Repeat("b", core.SessionEventTextLimit-1) || !e.Truncated {
		t.Fatalf("an over-long text: %d bytes, truncated %v; want it cut to whole runes within 64 KB", len(e.Text), e.Truncated)
	}
}

func TestATranscriptBegunAfterItsClaimIsATail(t *testing.T) {
	f := newTranscriptFixture(t)
	f.claim(t, 2, "s1")
	f.events(t, "s1", "before the restart")

	// Code Central restarts mid-Claim: memory is lost, the store isn't.
	f.clock.Advance(time.Minute)
	f.h = serve(f.store, f.clock)
	if tr := f.transcript(t, 2, 1); len(tr.Events) != 0 || tr.Tail {
		t.Fatalf("after a restart with no events: %+v, want none and no tail", tr)
	}
	f.events(t, "s1", "after the restart")
	tr := f.transcript(t, 2, 1)
	if got := strings.Join(texts(tr), ","); got != "after the restart" || !tr.Tail {
		t.Fatalf("Transcript %q, tail %v: want only the new event, as a tail", got, tr.Tail)
	}
}

func TestTranscriptOfAnUnknownProjectTicketOrClaimIsNotFound(t *testing.T) {
	f := newTranscriptFixture(t)
	f.claim(t, 2, "s1")
	for _, path := range []string{
		"/api/projects/nope/tickets/2/claims/1/transcript",
		"/api/projects/acme/tickets/9/claims/1/transcript",
		"/api/projects/acme/tickets/3/claims/1/transcript",
		"/api/projects/acme/tickets/2/claims/2/transcript",
		"/api/projects/acme/tickets/2/claims/x/transcript",
	} {
		rec := do(f.h, "GET", path, "")
		var e core.Error
		json.Unmarshal(rec.Body.Bytes(), &e)
		if rec.Code != 404 || e.Code != core.NotFound {
			t.Errorf("%s: %d %s, want 404 not_found", path, rec.Code, rec.Body)
		}
	}
}
