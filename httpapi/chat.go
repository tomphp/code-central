package httpapi

import (
	"net/http"

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
