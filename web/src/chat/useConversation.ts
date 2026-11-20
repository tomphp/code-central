// Follows a Project's current Conversation while active: the stream sends it
// as it stands and then each message, item and outcome, and the rereads a
// change notice prompts bring its availability, Factory, tail and the host
// Chat runs on up to date, which the stream doesn't carry.
import { useEffect, useState } from "react";
import type { ChatMessage, Conversation } from "../api/types";
import { useApi, useVersion } from "../live";

// withLast changes the last Chat Message, the Claude one a reply fills.
const withLast = (c: Conversation, change: (m: ChatMessage) => ChatMessage): Conversation => {
  const last = c.messages.length - 1;
  if (last < 0 || c.messages[last].from !== "claude") return c;
  return { ...c, messages: c.messages.map((m, i) => (i === last ? change(m) : m)) };
};

// useConversation is the Project's current Conversation as the stream last
// described it, undefined until the stream first sends it. It is dropped
// when inactive, so a reopened pop-up never shows a stale one.
export function useConversation(project: string, active: boolean): Conversation | undefined {
  const api = useApi();
  const version = useVersion();
  const [conversation, setConversation] = useState<Conversation>();

  useEffect(() => {
    if (!active) return;
    const stop = api.subscribeChat(project, {
      onConversation: setConversation,
      // The Conversation as it now stands follows as onConversation, the
      // same as get_chat gives it.
      onReplaced: () => setConversation(undefined),
      // A Claude message is a reply starting; its outcome ends it.
      onMessage: (m) => setConversation((c) => c && { ...c, running: c.running || m.from === "claude", messages: [...c.messages, m] }),
      onItem: (item) => setConversation((c) => c && withLast(c, (m) => ({ ...m, items: [...(m.items ?? []), item] }))),
      onOutcome: (o) => setConversation((c) => c && { ...withLast(c, (m) => ({ ...m, outcome: o.outcome, error: o.error })), running: false }),
      onSession: (s) => setConversation((c) => c && { ...c, session_id: s.session_id, resume_command: s.resume_command }),
    });
    return () => {
      stop();
      setConversation(undefined);
    };
  }, [api, project, active]);

  useEffect(() => {
    if (!active) return;
    let current = true;
    api.getChat(project).then(
      (fresh) =>
        current &&
        setConversation((c) =>
          // Only for the Conversation held: a new one arrives on the stream.
          c && c.id === fresh.id ? { ...c, availability: fresh.availability, factory: fresh.factory, tail: fresh.tail, runs_on: fresh.runs_on } : c,
        ),
      () => {},
    );
    return () => {
      current = false;
    };
  }, [api, project, active, version]);

  return conversation;
}
