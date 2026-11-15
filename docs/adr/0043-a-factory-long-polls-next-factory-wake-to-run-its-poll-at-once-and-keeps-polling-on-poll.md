# A Factory long-polls `next_factory_wake` to run its poll at once, and keeps polling on `--poll`

A Factory finds work by polling on `--poll` (30 seconds by default), so an approved Ticket, an answered Round or an accepted Ticket waits up to a whole interval. The Factory now also keeps one `next_factory_wake` call open, a long-poll that holds for at most `WaitTimeout` and answers `woken` when a write the poll acts on happens on its Project: a Ticket moving into Ready, Answered, Accepted, Done or Cancelled, a live Claim ended by someone other than its holder, Blockers shrinking, a setting the Factory reads changing, an Import landing, or a Map's `guided` Tag turned on. A wake runs the poll at once. The call carries no work: the Factory still decides what to claim, and `--poll` stays as the fallback and the Heartbeat cadence.

This sits beside [ADR 0040](0040-chat-goes-through-code-central-a-factory-long-polls-for-chat-messages-and-conversations-live-only-in-memory.md), which made the opposite choice: Chat's long-poll returns the message itself, because there a missed notice loses a turn. Here a missed wake loses nothing.

## Considered Options

- **A long-poll that hands out the Claim itself.** Rejected: Code Central would have to learn the Factory's claiming rules (slots, models, pausing, landing and resuming).
- **Waking on every change.** Rejected as too noisy: running agents write constantly, and each wake costs a poll with its `find_tickets` and a possible remote fetch.
- **Subscribing to `/api/events`.** Rejected: the stream carries every change notice for the UI, so the Factory would filter what Code Central already knows how to classify, and its notices are not shaped for this.
- **Shortening `--poll`.** Rejected: it cuts the delay only by spending polls and remote fetches while idle.

## Consequences

- A missed wake, such as a change between two calls, costs at most one `--poll` interval.
- A Factory newer than its Code Central falls back to plain polling: it warns once and stops long-polling for the run.
- There is one extra open connection per Factory and host.
- A newer call from the same Factory and host replaces the older, and a wake reaches every open call on the Project; Claim exclusivity settles the race between Factories.
- The operation is Factory-only, like `next_chat_work`, and optional: a Factory that never calls it just polls.
