package client

import (
	"context"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ops"
)

// Chat (Spec code-central-633): the human's Conversation, and the
// Factory's long-poll for its turns and its posts of their replies.

// GetChat is the human's get_chat; set Actor to human.
func (c *Client) GetChat(ctx context.Context, key string) (core.ConversationView, error) {
	return call(ctx, c, ops.GetChat, key, core.GetChatInput{})
}

// StartConversation is the human's start_conversation; set Actor to human.
func (c *Client) StartConversation(ctx context.Context, key string, in core.StartConversationInput) (core.ConversationView, error) {
	return call(ctx, c, ops.StartConversation, key, in)
}

// SendChatMessage is the human's send_chat_message; set Actor to human.
func (c *Client) SendChatMessage(ctx context.Context, key string, in core.SendChatMessageInput) (core.ConversationView, error) {
	return call(ctx, c, ops.SendChatMessage, key, in)
}

// StopChatReply is the human's stop_chat_reply; set Actor to human.
func (c *Client) StopChatReply(ctx context.Context, key string) (core.ConversationView, error) {
	return call(ctx, c, ops.StopChatReply, key, core.StopChatReplyInput{})
}

// NextChatWork is the Factory's next_chat_work: it holds for up to
// core.WaitTimeout until there is a turn or a stop for the Factory, and
// returns empty when there is none.
func (c *Client) NextChatWork(ctx context.Context, key string, in core.NextChatWorkInput) (core.ChatWork, error) {
	return call(ctx, c, ops.NextChatWork, key, in)
}

// PostChatEvents is POST /api/p/{key}/chat/events: the Factory holding
// the Conversation posts its reply's items and outcome. The service
// always answers 204, dropping a post it can't use.
func (c *Client) PostChatEvents(ctx context.Context, key string, in core.PostChatEventsInput) error {
	return c.do(ctx, "POST", scoped(key, "chat/events"), in, nil)
}
