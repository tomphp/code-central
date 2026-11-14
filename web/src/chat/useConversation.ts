// Follows a Project's current Conversation while active: the stream sends it
// as it stands and then each message, item and outcome, and the rereads a
// change notice prompts bring its availability, Factory and tail up to date,
// which the stream doesn't carry.
import { useEffect, useState } from "react";
import type { Api } from "../api/api";
import type { ChatMessage, Conversation } from "../api/types";

const NONE: Conversation = { availability: "none", running: false, tail: false, messages: [] };

// withLast changes the last Chat Message, the Claude one a reply fills.
const withLast = (c: Conversation, change: (m: ChatMessage) => ChatMessage): Conversation => {
  const last = c.messages.length - 1;
  if (last < 0 || c.messages[last].from !== "claude") return c;
  return { ...c, messages: c.messages.map((m, i) => (i === last ? change(m) : m)) };
};

export function useConversation(api: Api, project: string, active: boolean, version: number): Conversation | undefined {
  const [conversation, setConversation] = useState<Conversation>();

  useEffect(() => {
    if (!active) return;
    return api.subscribeChat(project, {
      onConversation: setConversation,
      onReplaced: () => setConversation(NONE),
      // A Claude message is a reply starting; its outcome ends it.
      onMessage: (m) => setConversation((c) => c && { ...c, running: c.running || m.from === "claude", messages: [...c.messages, m] }),
      onItem: (item) => setConversation((c) => c && withLast(c, (m) => ({ ...m, items: [...(m.items ?? []), item] }))),
      onOutcome: (o) => setConversation((c) => c && { ...withLast(c, (m) => ({ ...m, outcome: o.outcome, error: o.error })), running: false }),
      onSession: (s) => setConversation((c) => c && { ...c, session_id: s.session_id, resume_command: s.resume_command }),
    });
  }, [api, project, active]);

  useEffect(() => {
    if (!active) return;
    let current = true;
    api.getChat(project).then(
      (fresh) =>
        current &&
        setConversation((c) =>
          // Only for the Conversation held: a new one arrives on the stream.
          c && c.id === fresh.id ? { ...c, availability: fresh.availability, factory: fresh.factory, tail: fresh.tail } : c,
        ),
      () => {},
    );
    return () => {
      current = false;
    };
  }, [api, project, active, version]);

  return conversation;
}
