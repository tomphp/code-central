// Chat Messages and Conversations for the pop-up's tests and stories, which
// hand it props rather than reaching a Conversation through a Core. Nothing
// but those import this.
import type { ChatItem, ChatMessage, Conversation } from "../api/types";

const T = "2026-11-07T09:00:00Z";
let seq = 0;

export const item = (kind: ChatItem["kind"], text: string, more: Partial<ChatItem> = {}): ChatItem => ({ seq: ++seq, time: T, kind, text, ...more });
export const human = (text: string): ChatMessage => ({ from: "human", time: T, text });
export const claude = (items: ChatItem[], more: Partial<ChatMessage> = {}): ChatMessage => ({ from: "claude", time: T, items, ...more });

// conversation is an available haiku Conversation holding messages.
export const conversation = (messages: ChatMessage[], more: Partial<Conversation> = {}): Conversation => ({
  availability: "available",
  id: "c1",
  model: "haiku",
  running: false,
  tail: false,
  messages,
  ...more,
});

// noConversation is a Project with none yet.
export const noConversation = (more: Partial<Conversation> = {}): Conversation =>
  conversation([], { availability: "none", id: undefined, model: undefined, ...more });
