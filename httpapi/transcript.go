package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/tomoram/code-central/core"
)

// SessionEventsPath is where a Factory posts a session's Session Events
// (ADR 0008). Like ping it is not one of core's operations: it updates
// memory only, and always answers 204, even for an unknown session or a
// body it can't read, so a Factory never fails on it.
const SessionEventsPath = "POST /api/session-events"

// TranscriptPath reads one Claim's Transcript. It is outside core's
// operations and outside MCP.
const TranscriptPath = "GET /api/projects/{key}/tickets/{n}/claims/{c}/transcript"

// TranscriptStreamPath follows one Claim's Transcript as Server-Sent
// Events. On connect it sends the held events, then each new one, as
// session_event events whose id is the event's seq; a reconnect's
// Last-Event-ID gets only the events after it. A tail event (data {})
// comes before events that follow a gap: ones dropped from the Transcript
// (or lost to a restart) before the stream could send them, so the client
// holds only the tail of the session. The stream stays open after the
// Claim ends.
// Like the Transcript read, it is outside core's operations and MCP.
const TranscriptStreamPath = "GET /api/projects/{key}/tickets/{n}/claims/{c}/transcript/stream"

func sessionEvents(c *core.Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			SessionID string              `json:"session_id"`
			Events    []core.SessionEvent `json:"events"`
		}
		if decodeBody(r, &in) == nil {
			c.PostSessionEvents(r.Context(), in.SessionID, in.Events)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func transcript(c *core.Core, r *http.Request) (any, error) {
	if err := onlyParams(r.URL.Query()); err != nil {
		return nil, err
	}
	claim, err := claimNumber(r)
	if err != nil {
		return nil, err
	}
	return c.Transcript(r.Context(), r.PathValue("key"), core.Ref(r.PathValue("n")), claim)
}

func claimNumber(r *http.Request) (int, error) {
	claim, err := strconv.Atoi(r.PathValue("c"))
	if err != nil {
		return 0, &core.Error{Code: core.NotFound, Message: "there is no Claim " + strconv.Quote(r.PathValue("c"))}
	}
	return claim, nil
}

func transcriptStream(c *core.Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			WriteError(w, fmt.Errorf("the response can't stream"))
			return
		}
		if err := onlyParams(r.URL.Query()); err != nil {
			WriteError(w, err)
			return
		}
		claim, err := claimNumber(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		// A Last-Event-ID that isn't a seq is no resume: send it all.
		after, _ := strconv.Atoi(r.Header.Get("Last-Event-ID"))
		feed, err := c.FollowTranscript(r.Context(), r.PathValue("key"), core.Ref(r.PathValue("n")), claim, after)
		if err != nil {
			WriteError(w, err)
			return
		}
		defer feed.Stop()
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		// The opening comment tells the client the stream is live.
		fmt.Fprint(w, ": connected\n\n")
		flusher.Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-feed.Wake():
				tail, events := feed.Next()
				if tail {
					fmt.Fprint(w, "event: tail\ndata: {}\n\n")
				}
				for _, e := range events {
					data, _ := json.Marshal(e)
					fmt.Fprintf(w, "id: %d\nevent: session_event\ndata: %s\n\n", e.Seq, data)
				}
				flusher.Flush()
			}
		}
	}
}
