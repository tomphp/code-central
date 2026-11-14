package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/tomoram/code-central/core"
)

// Chat (Spec code-central-633): the human's Conversation, and the
// Factory's long-poll for its turns and stops.
func init() {
	Routes["get_chat"] = "GET /api/p/{key}/chat"
	Routes["start_conversation"] = "POST /api/p/{key}/start_conversation"
	Routes["send_chat_message"] = "POST /api/p/{key}/send_chat_message"
	Routes["stop_chat_reply"] = "POST /api/p/{key}/stop_chat_reply"
	Routes["next_chat_work"] = "GET /api/p/{key}/chat/next-work"

	handlers["get_chat"] = func(c *core.Core, r *http.Request) (any, error) {
		if err := onlyParams(r.URL.Query()); err != nil {
			return nil, err
		}
		return c.GetChat(r.Context(), r.PathValue("key"))
	}
	handlers["start_conversation"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.StartConversationInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.StartConversation(r.Context(), r.PathValue("key"), in)
	}
	handlers["send_chat_message"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.SendChatMessageInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.SendChatMessage(r.Context(), r.PathValue("key"), in)
	}
	handlers["stop_chat_reply"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.StopChatReplyInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.StopChatReply(r.Context(), r.PathValue("key"), in)
	}
	// next_chat_work holds for up to core.WaitTimeout, as wait does.
	handlers["next_chat_work"] = func(c *core.Core, r *http.Request) (any, error) {
		q := r.URL.Query()
		if err := onlyParams(q, "factory", "host"); err != nil {
			return nil, err
		}
		return c.NextChatWork(r.Context(), r.PathValue("key"), core.NextChatWorkInput{Factory: q.Get("factory"), Host: q.Get("host")})
	}
}

// ChatEventsPath is where the Factory holding a Conversation posts its
// reply's items and outcome. Like SessionEventsPath it is not one of
// core's operations: it updates memory only, and always answers 204, even
// for a post it drops or a body it can't read, so a Factory never fails
// on it.
const ChatEventsPath = "POST /api/p/{key}/chat/events"

func chatEvents(c *core.Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in core.PostChatEventsInput
		if decodeBody(r, &in) == nil {
			c.PostChatEvents(r.Context(), r.PathValue("key"), in)
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

func chatStream(c *core.Core) http.HandlerFunc {
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
		feed, err := c.FollowChat(r.Context(), r.PathValue("key"), after)
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
