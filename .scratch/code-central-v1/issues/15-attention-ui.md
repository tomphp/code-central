# The attention-first UI

Type: prototype
Status: resolved
Blocked by: 05, 06, 07

## Question

What should the UI look like so that what needs the human stands out across many Projects with minimal cognitive load? Prototype the main view, filtering by State and Project, answering a Round of Questions with pre-filled answers, resuming or releasing an `Answered` Ticket, and reading a failure report.

A `Failed` Ticket shows its report's headline and failure count in Needs Me, with the three moves from What a failure report contains beside the report.

A Silent Claim shows in Needs Me with how long it has been quiet, then the resume command, then Release with its warning, as [How agents claim work](09-claiming-work.md) decided.

Build the prototype in the v1 stack, React with TypeScript built by Vite, so it can be kept if it's good. Live updates are SSE events that only name what changed, and the UI refetches after each one ([Process shape and web stack](10-process-and-stack.md)).

[The MCP and HTTP surface](12-api-surface.md) fixes the human's actions the UI can call: `answer` (editable until the Round is complete), `accept_recommended`, `dismiss_question` for one Question inside a Round, `release`, `accept`, `reject` with a required comment, `send_back`, `to_draft`, `cancel`, `move` (to any State except `Failed`) and `rename_project`. The UI also reads `/api/needs-me` across every Project and listens for the `ticket.changed`, `spec.changed` and `project.changed` events, and each Ticket's `needs_me_reason` and `silent_since` are computed by the service. `finish` stores a required summary, which the UI shows with a Ticket in `In Review`.

[The Integration plugin contract](14-integration-contract.md) adds linked Specs and Tickets: show the Link (Integration name and external URL), make the definition fields read-only while the Link isn't detached, show the "closed externally" `needs_me_reason` in Needs Me with cancel, finish or `unlink` beside it, and offer the human-only `unlink` action.

## Answer

v1 ships one view, the **triage inbox**, and is built so other views can be added later as layouts only. The prototype is on the throwaway branch `prototype/attention-ui` (commit `509222c`, run with `cd web && npm install && npm run prototype`). It holds all three variants with mock data and stubbed actions, and is never merged.

**The triage inbox.** Three panes:
- **Left:** every Project with its Needs Me count, then a View list: Needs Me (the default) or any single State, each with a count. Both filters combine.
- **Middle:** the Needs Me list, grouped by reason, one row per Ticket: Ticket ID, title, how long it has waited, and a one-line detail for its reason. Drafts sit below in a collapsed, quiet group. Filtering by a State shows a flat list instead, with a dot on any row that needs the human.
- **Right:** the selected Ticket. Its action panel comes first, then the Ticket itself (Spec, Labels, Link, description, Claim activity, a collapsed history of moves and comments, and `move`).

**Needs Me order.** Grouped by reason in this order, cheapest to unblock an agent first: Questions, `Answered`, Silent Claim, `Failed`, `In Review`, blocked by a Cancelled Ticket, closed externally. Within a group, the longest-waiting Ticket comes first.

**The row detail for each reason:**
- **Questions:** how many are open, out of how many, and the Round number.
- **`Answered`:** when it was answered, and that no session has picked it up.
- **Silent Claim:** how long it has been quiet.
- **`Failed`:** the latest report's headline, plus "failed N×" when there is more than one report.
- **`In Review`:** the first line of the `finish` summary.
- **Blocked:** the Cancelled Blocker with no Replacement.
- **Closed externally:** the Integration, the external state, and the Ticket's State.

**One action panel per reason, independent of any view:**
- **Round:** every Question at once, each answer box pre-filled with the recommendation and labelled as such, Answer and Dismiss per Question, and Accept all recommended. Answers save one by one and can be edited until the Round is complete. Answering all at once was kept for v1; a one-Question-per-step form was prototyped and left out.
- **`Answered` and Silent Claim:** one Resume-then-Release panel. It says how long the Ticket has waited or been quiet, then shows a copyable `claude --resume <id>` (or says no session ID is recorded), then Release, which opens an optional note and the warning that a live session's next write will be rejected and the Claim can't be restored. `Answered` also shows the Round's answers, collapsed.
- **`Failed`:** the failure count, the latest report's four fields, and the three moves: Send back (with an optional note that says no note means nothing has changed), To Draft, and Cancel. Below them is the report's resume command for investigating, and earlier reports, collapsed.
- **`In Review`:** the `finish` summary, Accept, and Reject, which needs a comment.
- **Blocked by a Cancelled Ticket:** remove that Blocker, To Draft, or Cancel.
- **Closed externally:** a link to the external item, then Cancel, Mark Done or Unlink. "Finish" in the question becomes the human's `move` to `Done`, because `finish` needs a Claim.

**Linked items.** A Link badge (Integration and external ID, linking to the URL) shows on rows and in the detail. A note says the title and description are read-only while linked, and Unlink sits in the detail.

**Live updates.** The UI refetches after each SSE event, as decided. The prototype stood in for events with scripted agent writes, and nothing about the layout depended on them.

**Later views, which v1 must not block.** The human wants to switch between views eventually:
- **Focus queue:** one Needs Me item at a time, full width, with `j`/`k` and Rounds answered one Question per step.
- **Project board:** a Project × State grid, where Tickets that need the human are loud and the rest are quiet, with a detail drawer.

To keep them cheap to add later:
- The action panels and the Ticket detail are components shared by every view, so a new view is only a layout.
- The current view is part of the URL (such as `?view=inbox`).
- Neither later view needs anything new from the API. The board reads `find_tickets` per Project.

Considered and rejected:
- **Shipping all three views in v1:** each one adds its own layout, keyboard handling and tests for the same actions, and real use of the inbox should decide what the others are for.
- **The step-by-step Round form inside the inbox for v1:** the all-at-once form is judged in real use first.
