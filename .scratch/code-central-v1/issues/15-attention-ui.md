# The attention-first UI

Type: prototype
Blocked by: 05, 06, 07

## Question

What should the UI look like so that what needs the human stands out across many Projects with minimal cognitive load? Prototype the main view, filtering by State and Project, answering a Round of Questions with pre-filled answers, resuming or releasing an `Answered` Ticket, and reading a failure report.

A `Failed` Ticket shows its report's headline and failure count in Needs Me, with the three moves from What a failure report contains beside the report.

A Silent Claim shows in Needs Me with how long it has been quiet, then the resume command, then Release with its warning, as [How agents claim work](09-claiming-work.md) decided.

Build the prototype in the v1 stack, React with TypeScript built by Vite, so it can be kept if it's good. Live updates are SSE events that only name what changed, and the UI refetches after each one ([Process shape and web stack](10-process-and-stack.md)).

[The MCP and HTTP surface](12-api-surface.md) fixes the human's actions the UI can call: `answer` (editable until the Round is complete), `accept_recommended`, `dismiss_question` for one Question inside a Round, `release`, `accept`, `reject` with a required comment, `send_back`, `to_draft`, `cancel`, `move` (to any State except `Failed`) and `rename_project`. The UI also reads `/api/needs-me` across every Project and listens for the `ticket.changed`, `spec.changed` and `project.changed` events, and each Ticket's `needs_me_reason` and `silent_since` are computed by the service. `finish` stores a required summary, which the UI shows with a Ticket in `In Review`.

[The Integration plugin contract](14-integration-contract.md) adds linked Specs and Tickets: show the Link (Integration name and external URL), make the definition fields read-only while the Link isn't detached, show the "closed externally" `needs_me_reason` in Needs Me with cancel, finish or `unlink` beside it, and offer the human-only `unlink` action.
