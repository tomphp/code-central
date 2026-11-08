package httpapi_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
)

// frame is one SSE event on a Transcript stream.
type frame struct {
	id, name string
	event    heldEvent
}

// transcriptStream opens Claim claim of acme-<ticket>'s Transcript stream,
// sending Last-Event-ID when lastID isn't empty, and returns a reader
// positioned after the opening comment, so the stream is live when it
// returns.
func transcriptStream(t *testing.T, url string, ticket, claim int, lastID string) *bufio.Reader {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/api/projects/acme/tickets/%d/claims/%d/transcript/stream", url, ticket, claim), nil)
	if lastID != "" {
		req.Header.Set("Last-Event-ID", lastID)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("status %d, content type %q", res.StatusCode, res.Header.Get("Content-Type"))
	}
	r := bufio.NewReader(res.Body)
	if line, err := r.ReadString('\n'); err != nil || !strings.HasPrefix(line, ":") {
		t.Fatalf("opening line %q, %v", line, err)
	}
	r.ReadString('\n')
	return r
}

// liveServer serves h until the test ends, closing it after the streams
// the test opened, which Close would otherwise wait on.
func liveServer(t *testing.T, h http.Handler) *httptest.Server {
	hs := httptest.NewServer(h)
	t.Cleanup(hs.Close)
	return hs
}

// nextFrame reads one SSE event, failing the test if none comes within a
// second.
func nextFrame(t *testing.T, r *bufio.Reader) frame {
	t.Helper()
	got := make(chan frame, 1)
	failed := make(chan error, 1)
	go func() {
		var f frame
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
				json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &f.event)
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
	return frame{}
}

// wantEvents reads one session_event per text, each with its seq as its
// SSE id.
func wantEvents(t *testing.T, r *bufio.Reader, first int, texts ...string) {
	t.Helper()
	for i, text := range texts {
		seq := first + i
		f := nextFrame(t, r)
		if f.name != "session_event" || f.id != fmt.Sprint(seq) || f.event.Seq != seq || f.event.Text != text {
			t.Fatalf("got %s id %s %+v, want session_event id %d %q", f.name, f.id, f.event, seq, text)
		}
	}
}

func wantTail(t *testing.T, r *bufio.Reader) {
	t.Helper()
	if f := nextFrame(t, r); f.name != "tail" || f.id != "" {
		t.Fatalf("got %s id %q, want a tail notice", f.name, f.id)
	}
}

func TestATranscriptStreamSendsTheHeldEventsThenEachNewOne(t *testing.T) {
	f := newTranscriptFixture(t)
	hs := liveServer(t, f.h)
	f.claim(t, 2, "s1")
	f.events(t, "s1", "one", "two")

	r := transcriptStream(t, hs.URL, 2, 1, "")
	wantEvents(t, r, 1, "one", "two")
	f.events(t, "s1", "three")
	f.events(t, "s1", "four", "five")
	wantEvents(t, r, 3, "three", "four", "five")

	// A second follower of the same Claim gets the same, from the start.
	again := transcriptStream(t, hs.URL, 2, 1, "")
	wantEvents(t, again, 1, "one", "two", "three", "four", "five")
}

func TestATranscriptStreamWithLastEventIDResumesWithNoGapsOrRepeats(t *testing.T) {
	f := newTranscriptFixture(t)
	hs := liveServer(t, f.h)
	f.claim(t, 2, "s1")
	f.events(t, "s1", "one", "two")

	r := transcriptStream(t, hs.URL, 2, 1, "")
	wantEvents(t, r, 1, "one", "two")
	// The connection drops; events arrive while it is down.
	f.events(t, "s1", "three", "four")

	r = transcriptStream(t, hs.URL, 2, 1, "2")
	wantEvents(t, r, 3, "three", "four")
	f.events(t, "s1", "five")
	wantEvents(t, r, 5, "five")

	// Resuming at the newest event waits for the next one.
	r = transcriptStream(t, hs.URL, 2, 1, "5")
	f.events(t, "s1", "six")
	wantEvents(t, r, 6, "six")
}

func TestATranscriptStreamSendsATailNoticeWhenEarlierEventsWereDropped(t *testing.T) {
	f := newTranscriptFixture(t)
	hs := liveServer(t, f.h)
	f.claim(t, 2, "s1")
	var batch []string
	for i := 1; i <= core.TranscriptLimit+2; i++ {
		batch = append(batch, fmt.Sprint(i))
	}
	f.events(t, "s1", batch...)

	// Events 1 and 2 are gone: the notice comes first, then what is held.
	r := transcriptStream(t, hs.URL, 2, 1, "")
	wantTail(t, r)
	wantEvents(t, r, 3, "3", "4")

	r = transcriptStream(t, hs.URL, 2, 1, "2001")
	wantTail(t, r)
	wantEvents(t, r, 2002, "2002")
	f.events(t, "s1", "2003")
	wantEvents(t, r, 2003, "2003") // the notice is sent once
}

func TestATranscriptStreamSendsNoTailNoticeForAWholeTranscript(t *testing.T) {
	f := newTranscriptFixture(t)
	hs := liveServer(t, f.h)
	f.claim(t, 2, "s1")
	f.events(t, "s1", "one")
	wantEvents(t, transcriptStream(t, hs.URL, 2, 1, ""), 1, "one")
	f.events(t, "s1", "two")
	wantEvents(t, transcriptStream(t, hs.URL, 2, 1, "1"), 2, "two")
}

func TestATranscriptStreamBegunAfterARestartIsATailAndStartsAgain(t *testing.T) {
	f := newTranscriptFixture(t)
	f.claim(t, 2, "s1")
	f.events(t, "s1", "one", "two", "three")

	// Code Central restarts mid-Claim, and seqs start again from 1.
	f.clock.Advance(time.Minute)
	f.h = serve(f.store, f.clock)
	hs := liveServer(t, f.h)

	// A follower that held seq 3 before the restart, and one opened afresh,
	// both get the new Transcript from its start, as a tail.
	resumed := transcriptStream(t, hs.URL, 2, 1, "3")
	fresh := transcriptStream(t, hs.URL, 2, 1, "")
	f.events(t, "s1", "after the restart")
	for _, r := range []*bufio.Reader{resumed, fresh} {
		wantTail(t, r)
		wantEvents(t, r, 1, "after the restart")
	}
}

func TestATranscriptStreamStaysOpenAfterTheClaimEnds(t *testing.T) {
	f := newTranscriptFixture(t)
	hs := liveServer(t, f.h)
	id := f.claim(t, 2, "s1")

	r := transcriptStream(t, hs.URL, 2, 1, "")
	f.events(t, "s1", "working")
	wantEvents(t, r, 1, "working")
	f.ok(t, "POST", "/api/p/acme/tickets/2/finish", fmt.Sprintf(`{"claim_id":%q,"summary":"done"}`, id))
	f.events(t, "s1", "turn ended")
	wantEvents(t, r, 2, "turn ended")
}

func TestATranscriptStreamOfAnUnknownProjectTicketOrClaimIsNotFound(t *testing.T) {
	f := newTranscriptFixture(t)
	f.claim(t, 2, "s1")
	for _, path := range []string{
		"/api/projects/nope/tickets/2/claims/1/transcript/stream",
		"/api/projects/acme/tickets/9/claims/1/transcript/stream",
		"/api/projects/acme/tickets/3/claims/1/transcript/stream",
		"/api/projects/acme/tickets/2/claims/2/transcript/stream",
		"/api/projects/acme/tickets/2/claims/x/transcript/stream",
	} {
		rec := do(f.h, "GET", path, "")
		var e core.Error
		json.Unmarshal(rec.Body.Bytes(), &e)
		if rec.Code != 404 || e.Code != core.NotFound {
			t.Errorf("%s: %d %s, want 404 not_found", path, rec.Code, rec.Body)
		}
	}
}

func TestATranscriptStreamEndsWhenTheRequestContextEnds(t *testing.T) {
	f := newTranscriptFixture(t)
	f.claim(t, 2, "s1")
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/api/projects/acme/tickets/2/claims/1/transcript/stream", nil).WithContext(ctx)
	done := make(chan struct{})
	go func() {
		f.h.ServeHTTP(httptest.NewRecorder(), req)
		close(done)
	}()
	cancel()
	<-done // a hang here fails the test by timeout
}
