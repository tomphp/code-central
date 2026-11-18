package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ops"
)

// ChatEventsPath is where the Factory holding a Conversation posts its
// reply's items and outcome. Like SessionEventsPath it is not one of
// core's operations: it updates memory only, and always answers 204, even
// for a post it drops or a body it can't read, so a Factory never fails
// on it.
const ChatEventsPath = "POST /api/p/{key}/chat/events"

func chatEvents(s ops.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in core.PostChatEventsInput
		if decodeBody(r, &in) == nil {
			s.PostChatEvents(r.Context(), r.PathValue("key"), in)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ChatStreamPath follows the Project's current Conversation as
// Server-Sent Events, modelled on the Transcript stream. It opens with a
// conversation event holding the Conversation as it stands (or none), then
// sends each message, item, session and outcome event with its sequence
// number as the event id; a reconnect's Last-Event-ID gets only the events
// after it. When the Conversation is replaced it sends a replaced event
// (the client rereads get_chat), then the new Conversation. Like the
// Transcript stream it is outside core's operations and MCP.
const ChatStreamPath = "GET /api/p/{key}/chat/stream"

func chatStream(s ops.Service) http.HandlerFunc {
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
		// A Last-Event-ID that isn't a seq is no resume: send it all.
		after, _ := strconv.Atoi(r.Header.Get("Last-Event-ID"))
		feed, err := s.FollowChat(r.Context(), r.PathValue("key"), after)
		if err != nil {
			WriteError(w, err)
			return
		}
		defer feed.Stop()
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, ": connected\n\n")
		flusher.Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-feed.Wake():
				for _, f := range feed.Next() {
					data, _ := json.Marshal(f.Data)
					if f.ID > 0 {
						fmt.Fprintf(w, "id: %d\n", f.ID)
					}
					fmt.Fprintf(w, "event: %s\ndata: %s\n\n", f.Event, data)
				}
				flusher.Flush()
			}
		}
	}
}
