package core

import (
	"context"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/tomoram/code-central/store"
)

// Session Events and Transcripts (ADR 0008). A Factory posts what a
// Claim's session did; Code Central holds the last TranscriptLimit of them
// per Claim, in memory only. It never interprets their tool or text,
// and reads their kind only to show the latest event short (see
// latestEvent). Like a ping, posting is not one of the operations: it never fails
// and writes nothing to the store.

// TranscriptLimit is how many Session Events a Transcript holds; the
// oldest are dropped first.
const TranscriptLimit = 2000

// SessionEventTextLimit is the most bytes of text a Session Event keeps;
// longer text is cut on arrival.
const SessionEventTextLimit = 64 << 10

// SessionEventKind is a Session Event's kind, one of SessionEventKinds as
// the Factory posts them. Code Central doesn't check it.
type SessionEventKind string

// The kinds of Session Event.
const (
	// EventText is the agent's text.
	EventText SessionEventKind = "text"
	// EventToolCall is a tool call, its text a summary of the call.
	EventToolCall SessionEventKind = "tool_call"
	// EventToolResult is a tool call's result.
	EventToolResult SessionEventKind = "tool_result"
	// EventFactory is a Factory log line about the Claim.
	EventFactory SessionEventKind = "factory"
	// EventPermissionDenials is a Chat reply's item listing the calls
	// its turn had denied, from its result's permission_denials.
	EventPermissionDenials SessionEventKind = "permission_denials"
)

// SessionEventKinds are the kinds of Session Event.
func SessionEventKinds() []SessionEventKind {
	return []SessionEventKind{EventText, EventToolCall, EventToolResult, EventFactory, EventPermissionDenials}
}

// SessionEvent is one thing a Claim's session did, as posted: agent text,
// a tool call, a tool result or a Factory log line. Tool and Text are
// opaque strings.
type SessionEvent struct {
	Time      time.Time        `json:"time"`
	Kind      SessionEventKind `json:"kind"`
	Text      string           `json:"text"`
	Tool      string           `json:"tool,omitempty"`
	ToolUseID string           `json:"tool_use_id,omitempty"`
	IsError   bool             `json:"is_error,omitempty"`
	Truncated bool             `json:"truncated,omitempty"`
}

// HeldSessionEvent is a Session Event in a Transcript, with its sequence
// number, which rises within the Transcript and is the SSE id its stream
// resumes from.
type HeldSessionEvent struct {
	Seq int `json:"seq"`
	SessionEvent
}

// TranscriptView is a Claim's Transcript as the read returns it: the held
// events, oldest first. Tail is true when it holds only part of the
// session: events were dropped from its front, as it keeps the last
// TranscriptLimit, or it began after the Claim did, as none are kept
// from before Code Central's last restart, or on another service before
// its session moved here. Host is the host of another service on the
// Store holding the Transcript, its Reporting Service, while that service
// is running: then Events is empty.
type TranscriptView struct {
	Events []HeldSessionEvent `json:"events"`
	Tail   bool               `json:"tail"`
	Host   string             `json:"host,omitempty"`
}

// transcripts is every Claim's Transcript, keyed by Project, Ticket and
// Claim number.
type transcripts struct {
	mu  sync.Mutex
	all map[markKey]*transcript
	// feeds is each Claim's followers (FollowTranscript).
	feeds map[markKey]map[*TranscriptFeed]struct{}
}

type transcript struct {
	events  []HeldSessionEvent
	lastSeq int
	tail    bool
}

// PostSessionEvents is POST /api/session-events {session_id, events}: it
// adds the events to the Transcript of the newest Claim with that session
// ID, live or ended, and counts as a ping. With no such Claim they are
// dropped.
func (c *Core) PostSessionEvents(ctx context.Context, sessionID string, events []SessionEvent) {
	if sessionID == "" {
		return
	}
	c.Ping(ctx, sessionID)
	if len(events) == 0 {
		return
	}
	k, started, ok := c.newestClaim(sessionID)
	if !ok {
		return
	}
	moved := false
	if !c.transcripts.holds(k) {
		moved = c.startHolding(k)
	}
	c.hold(k, started, moved, events)
	c.latestEventChanged(k.project, k.ticket)
}

// holds says whether this service holds Claim k's Transcript.
func (ts *transcripts) holds(k markKey) bool {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.all[k] != nil
}

// hold adds events to Claim k's Transcript, starting it if need be: as a
// tail when the Claim started before the service did, or moved is true.
func (c *Core) hold(k markKey, started time.Time, moved bool, events []SessionEvent) {
	ts := c.transcripts
	ts.mu.Lock()
	defer ts.mu.Unlock()
	tr := ts.all[k]
	if tr == nil {
		tr = &transcript{}
		if moved || started.Before(c.activity.started) {
			// A Claim older than the service lost its earlier events, and
			// one whose session moved here left them on another service.
			// Its seqs start past any the last service gave, so a
			// follower resuming from one of those misses none of the new
			// ones, and past 1, so one from the start hears it is a tail.
			tr.tail, tr.lastSeq = true, int(c.activity.started.UnixMilli())
		}
		ts.all[k] = tr
	}
	for _, e := range events {
		e.Text, e.Truncated = cutText(e.Text, e.Truncated)
		tr.lastSeq++
		tr.events = append(tr.events, HeldSessionEvent{Seq: tr.lastSeq, SessionEvent: e})
	}
	if over := len(tr.events) - TranscriptLimit; over > 0 {
		tr.events = tr.events[over:]
		tr.tail = true
	}
	for f := range ts.feeds[k] {
		select {
		case f.wake <- struct{}{}:
		default: // already woken
		}
	}
}

// newestClaim is the most recently started Claim with the session ID, in
// any Project, and when it started.
func (c *Core) newestClaim(sessionID string) (markKey, time.Time, bool) {
	var sc store.SessionClaim
	found := false
	c.store.View(func(r store.Reader) error {
		sc, found = r.ClaimBySession(sessionID)
		return nil
	})
	return markKey{sc.Project, sc.Ticket, sc.Claim.Number}, sc.Claim.Started, found
}

// cutText cuts text to SessionEventTextLimit bytes of whole runes, and
// says whether it is now cut.
func cutText(text string, truncated bool) (string, bool) {
	if len(text) <= SessionEventTextLimit {
		return text, truncated
	}
	n := SessionEventTextLimit
	for n > 0 && !utf8.RuneStart(text[n]) {
		n--
	}
	return text[:n], true
}

// Transcript is GET /api/projects/{key}/tickets/{n}/claims/{c}/transcript:
// the Session Events held for Claim number claim, oldest first. An unknown
// Project, Ticket or Claim is not_found; a Claim with none held has an
// empty Transcript.
func (c *Core) Transcript(_ context.Context, key string, ticket Ref, claim int) (TranscriptView, error) {
	k, err := c.claimKey(key, "transcript", ticket, claim)
	if err != nil {
		return TranscriptView{}, err
	}
	if host := c.heldElsewhere(k); host != "" {
		return TranscriptView{Events: []HeldSessionEvent{}, Host: host}, nil
	}
	ts := c.transcripts
	ts.mu.Lock()
	defer ts.mu.Unlock()
	v := TranscriptView{Events: []HeldSessionEvent{}}
	if tr := ts.all[k]; tr != nil {
		v.Events = append(v.Events, tr.events...)
		v.Tail = tr.tail
	}
	return v, nil
}

// claimKey is Claim number claim of a Ticket, as a Transcript's key. An
// unknown Project, Ticket or Claim is not_found.
func (c *Core) claimKey(key, op string, ticket Ref, claim int) (markKey, error) {
	var k markKey
	err := c.store.View(func(r store.Reader) error {
		if _, ok := r.Project(key); !ok {
			return notFound("there is no Project %q", key)
		}
		n, err := ticket.number(key, "ticket")
		if err != nil {
			return notFound("there is no Ticket %s", ticket)
		}
		t, err := ticketNumbered(r, key, op, "ticket", n, store.Claims)
		if err != nil {
			return err
		}
		for _, cl := range t.Claims {
			if cl.Number == claim {
				k = markKey{key, n, claim}
				return nil
			}
		}
		return notFound("%s has no Claim %d", DisplayID(key, n), claim)
	})
	return k, err
}

// A TranscriptFeed follows one Claim's Transcript: the stream read
// (GET .../transcript/stream) sends what Next gives each time Wake fires.
// It holds no events of its own, only the last seq it gave, so a slow
// follower costs nothing and can't fall behind a full buffer.
type TranscriptFeed struct {
	ts    *transcripts
	k     markKey
	wake  chan struct{}
	after int
	host  string
}

// FollowTranscript starts a feed of Claim number claim's Transcript after
// seq after (0 for all of it), as a reconnect's Last-Event-ID asks. A seq
// beyond the Transcript's newest is none it gave, so the feed starts from
// the beginning. It stays open
// after the Claim ends, because a late Factory line still arrives. When
// another service holds the Transcript (see heldElsewhere), the feed has
// ended already: Host names that service's host, and it never wakes. An
// unknown Project, Ticket or Claim is not_found. Stop ends it.
func (c *Core) FollowTranscript(_ context.Context, key string, ticket Ref, claim, after int) (*TranscriptFeed, error) {
	k, err := c.claimKey(key, "follow_transcript", ticket, claim)
	if err != nil {
		return nil, err
	}
	ts := c.transcripts
	if host := c.heldElsewhere(k); host != "" {
		return &TranscriptFeed{ts: ts, k: k, wake: make(chan struct{}), host: host}, nil
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	last := 0
	if tr := ts.all[k]; tr != nil {
		last = tr.lastSeq
	}
	if after < 0 || after > last {
		after = 0
	}
	f := &TranscriptFeed{ts: ts, k: k, wake: make(chan struct{}, 1), after: after}
	if ts.feeds[k] == nil {
		ts.feeds[k] = map[*TranscriptFeed]struct{}{}
	}
	ts.feeds[k][f] = struct{}{}
	f.wake <- struct{}{} // the held events are waiting already
	return f, nil
}

// Wake fires when Next may have events to give.
func (f *TranscriptFeed) Wake() <-chan struct{} { return f.wake }

// Host is the host of the service holding the Transcript when that is
// another one, and the feed has ended; empty for a feed of this service's.
func (f *TranscriptFeed) Host() string { return f.host }

// Next gives the held events after the last it gave, oldest first, and
// says whether a tail notice is due before them: when events between the
// last it gave and the oldest held were dropped before it could give them.
func (f *TranscriptFeed) Next() (tail bool, events []HeldSessionEvent) {
	f.ts.mu.Lock()
	defer f.ts.mu.Unlock()
	tr := f.ts.all[f.k]
	if tr == nil {
		return false, nil
	}
	if len(tr.events) > 0 && tr.events[0].Seq > f.after+1 {
		tail = true
	}
	for _, e := range tr.events {
		if e.Seq > f.after {
			events = append(events, e)
		}
	}
	if len(events) > 0 {
		f.after = events[len(events)-1].Seq
	}
	return tail, events
}

// Stop ends the feed.
func (f *TranscriptFeed) Stop() {
	f.ts.mu.Lock()
	defer f.ts.mu.Unlock()
	delete(f.ts.feeds[f.k], f)
	if len(f.ts.feeds[f.k]) == 0 {
		delete(f.ts.feeds, f.k)
	}
}
