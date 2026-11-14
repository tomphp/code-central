package client

import (
	"context"
	"net/url"

	"github.com/tomoram/code-central/core"
)

// Chat (Spec code-central-633): the human's Conversation, and the
// Factory's long-poll for its turns and its posts of their replies.

// GetChat is the human's get_chat; set Actor to human.
func (c *Client) GetChat(ctx context.Context, key string) (core.ConversationView, error) {
	var out core.ConversationView
	return out, c.do(ctx, "GET", scoped(key, "chat"), nil, &out)
}

// StartConversation is the human's start_conversation; set Actor to human.
func (c *Client) StartConversation(ctx context.Context, key string, in core.StartConversationInput) (core.ConversationView, error) {
	var out core.ConversationView
	return out, c.do(ctx, "POST", scoped(key, "start_conversation"), in, &out)
}

// SendChatMessage is the human's send_chat_message; set Actor to human.
func (c *Client) SendChatMessage(ctx context.Context, key string, in core.SendChatMessageInput) (core.ConversationView, error) {
	var out core.ConversationView
	return out, c.do(ctx, "POST", scoped(key, "send_chat_message"), in, &out)
}

// StopChatReply is the human's stop_chat_reply; set Actor to human.
func (c *Client) StopChatReply(ctx context.Context, key string) (core.ConversationView, error) {
	var out core.ConversationView
	return out, c.do(ctx, "POST", scoped(key, "stop_chat_reply"), core.StopChatReplyInput{}, &out)
}

// NextChatWork is the Factory's next_chat_work: it holds for up to
// core.WaitTimeout until there is a turn or a stop for the Factory, and
// returns empty when there is none.
func (c *Client) NextChatWork(ctx context.Context, key string, in core.NextChatWorkInput) (core.ChatWork, error) {
	var out core.ChatWork
	path := scoped(key, "chat/next-work") + "?" + url.Values{"factory": {in.Factory}, "host": {in.Host}}.Encode()
	return out, c.do(ctx, "GET", path, nil, &out)
}

// PostChatEvents is POST /api/p/{key}/chat/events: the Factory holding
// the Conversation posts its reply's items and outcome. The service
// always answers 204, dropping a post it can't use.
func (c *Client) PostChatEvents(ctx context.Context, key string, in core.PostChatEventsInput) error {
	return c.do(ctx, "POST", scoped(key, "chat/events"), in, nil)
}
