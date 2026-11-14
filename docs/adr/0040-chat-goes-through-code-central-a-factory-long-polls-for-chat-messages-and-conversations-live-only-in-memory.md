# Chat goes through Code Central: a Factory long-polls for Chat Messages, and Conversations live only in memory

The human chats with Claude about a Project from the UI (a **Chat**). The UI posts each **Chat Message** to Code Central, and the Project's Factory fetches it with `next_chat_work`, a long-poll that holds until there is work for that Factory (at most `WaitTimeout`, then empty) and returns the message itself. The Factory runs the turn with `claude -p` (`--session-id` on the first turn, `--resume` after) and posts the reply's items to `post_chat_events` as they arrive. A **Conversation** is held by the one Factory that took its first turn, and Code Central keeps it in memory only, as it keeps Heartbeats.

## Considered Options

- **The Factory subscribing to `/api/events`.** Rejected in favour of a purpose-built long-poll that returns the message itself: the event stream carries notices, not content, and a missed notice would lose a turn.
- **The Factory serving an HTTP port for Chat.** Rejected under [ADR 0008](0008-session-events-are-posted-by-the-factory-and-live-only-in-memory.md) and [ADR 0025](0025-a-factory-announces-itself-with-a-heartbeat-that-lives-only-in-memory.md): the Factory is a client of Code Central, and Code Central never probes it.
- **Polling on `--poll`.** Rejected as too slow: the default is 30 seconds.
- **Storing Conversations.** Rejected for the same reasons as Transcripts and Heartbeats: it is a live view of work in progress, not a record Code Central must keep.

## Consequences

- Conversations are lost when Code Central restarts. The Resume Command still works, since the session lives in the Factory's `claude`.
- A Conversation is tied to one Factory's host and worktree.
- Posting a reply never fails a turn: a post that can't be delivered is dropped.
- A Factory says it can run Conversations with **Chat support** in its Heartbeat (`chat`, false when left out), so Chat is available for a Project while an Online Factory reports it.
