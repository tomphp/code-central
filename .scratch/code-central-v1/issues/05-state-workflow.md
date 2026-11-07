# The State workflow

Type: grilling
Status: resolved

## Question

What is the exact set of States, what are the allowed transitions, and which are made by the human versus an agent? Does a Spec have its own State or derive one from its Tickets? Can Tickets block each other? Which States count as "needs me"?

## Answer

There is one fixed workflow, the same for every Project. Agents can read it but not change it.

Amended by [How agents ask and the human answers](06-question-model.md), which added `Answered` so that an answered Ticket keeps its claim instead of returning to `Ready`, and by [What a failure report contains](07-failure-reports.md), which added `Failed` to `Draft`, made `Failed` an exception to the human moving a Ticket anywhere, and required a note on a clean give-up, and by [How agents claim work](09-claiming-work.md), which added Silent Claims to Needs Me and named the human releasing an `In Progress` Ticket.

| State | Meaning |
| --- | --- |
| `Draft` | Still being defined, so no agent should pick it up |
| `Ready` | An agent can claim it |
| `In Progress` | An agent has claimed it and is working |
| `Needs Answer` | The agent is stopped on an unanswered Question |
| `Answered` | Every Question is answered and the asking session has not yet picked the work up |
| `Failed` | The agent stopped because something broke |
| `In Review` | The work is finished and waiting for the human to accept it |
| `Done` | Finished |
| `Cancelled` | Abandoned |

| From | To | Who | When |
| --- | --- | --- | --- |
| `Draft` | `Ready` | Human or agent | Approves the work for agents |
| `Ready` | `Draft` | Human or agent | Defers it, or finds it isn't ready |
| `Ready` | `In Progress` | Agent | Claims it |
| `In Progress` | `Ready` | Agent | Gives up cleanly, saying where the work stands |
| `In Progress` | `Ready` | Human | Releases the Claim, usually a Silent Claim |
| `In Progress` | `Needs Answer` | Agent | Posts a Question |
| `Needs Answer` | `Answered` | Automatic | Last open Question is answered |
| `Answered` | `In Progress` | Agent | The claim holder picks up the answers |
| `Answered` | `Ready` | Human | Releases the claim, so a new agent can take it |
| `In Progress` | `Failed` | Agent | Posts a Failure Report, which ends the claim |
| `Failed` | `Ready` | Human | Sends it back after reading the report, with an optional note |
| `Failed` | `Draft` | Human | The Ticket needs rewriting before an agent tries again |
| `In Progress` | `In Review` | Agent | Finishes the work |
| `In Review` | `Done` | Human | Accepts it |
| `In Review` | `Ready` | Human | Rejects it, with a comment saying why |
| `Draft`, `Ready` | `Cancelled` | Agent | Only by naming Replacements |
| Any except `Done` / `Cancelled` | `Cancelled` | Human | Abandons it |

Rules:

- **Creating Tickets.** An agent can create a Ticket in `Draft` or `Ready`.
- **Enforcement.** Agent moves are validated against the workflow, and the human can move any Ticket to any State except `Failed`, which only the claim holder can enter by posting a Failure Report. A rejected move returns the moves allowed from that State.
- **Actor.** Every write declares `human` or `agent`, the default is `agent`, and only the web UI sends `human`. With no authentication this guards against mistakes, not misuse.
- **Required content.** `Needs Answer` and `Failed` can only be entered by posting the Question or the Failure Report, never set directly. A clean give-up must say where the work stands.
- **Review.** Every Ticket passes through `In Review` in v1. A way to skip it can be added later without changing stored data.
- **Spec State.** A Spec has no stored State, only a summary derived from its Tickets, and a Spec with no Tickets shows as `Draft`. Any work on a Spec, including grilling, is a Ticket on it.
- **Blocking.** Blocking is a many-to-many link between Tickets, across Specs but within one Project. It is computed rather than a State: a blocked `Ready` Ticket stays `Ready` but isn't claimable. Links that would form a cycle are rejected.
- **Clearing a blocker.** Only `Done` clears a blocker. A `Cancelled` blocker keeps the Ticket blocked until the human deals with it.
- **Replacements.** A `Cancelled` Ticket can name one or more Replacements, and anything it was blocking becomes blocked by them instead. This one link covers `Superseded by` and `Broken into`. `Deferred` is `Draft` with a reason.
- **Needs me.** `Needs Answer`, `Answered`, `Failed` and `In Review`, plus any Ticket blocked by a `Cancelled` Ticket with no Replacements, and any Silent Claim. `Draft` is a separate, quieter list.
- **Publishing the workflow.** The MCP server serves the whole workflow from the same table it enforces: States, meanings, moves, who can make them, required content and what counts as needs me. A repository being set up points at this instead of copying it, so the description can't go stale.

Considered and rejected: delivery-only States with flags for "has an open Question" and "has failed", because a Ticket's position would then need several fields to read. A separate workflow on Specs, because a grilling Ticket covers the same need with one workflow. Per-Project workflows, because every Factory, the UI and each Integration would have to cope with unknown States.
