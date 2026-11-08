# Stage Guide: tickets

You hold a Claim on the tickets Stage of a Requirement. The Requirement is the Spec this Ticket belongs to, and its body is now the full Spec. `get_stage_guide` with stage `tickets` serves this guide again if you lose it.

## What this Ticket produces

The implementation Tickets that deliver the Spec, as thin vertical slices, each one demoable or testable on its own. Create each with `create_ticket` on this Spec:

- `state` `Ready`, so it can be claimed once its Blockers are done.
- Each one blocked by this Ticket: put its ID in `blocked_by`, so no implementation starts before the human accepts the breakdown. Add the Blockers between them as the work needs, in `blocked_by` or with `add_blocker`.
- A description that holds its acceptance criteria and points at the Spec sections behind them. Give no Stage Label.

If the Spec records new terms or ADRs, assign writing them to an implementation Ticket, usually the first.

A Stage never changes repository files and pushes no branch. Read the repository as much as you need, but write nothing to it.

## Questions

Every question goes to the human as a Round with `post_round`, never in the terminal, each with your recommended answer; after each one end your turn, and `pick_up_answers` when the Ticket is Answered.

The In Review gate replaces any step that would stop to check with the user: the human reviews the breakdown when you finish, instead of checking with the user along the way. So a Round is for a real open question only, never to approve the breakdown.

## Finishing

`finish` with a summary that is the numbered breakdown with its blocking edges: each Ticket's ID and title, and which Tickets it is blocked by. Give no `branch` or `commit`: a Stage has none. The Ticket goes to In Review; stop working it.

## A redo

If this Ticket has earlier Claims, it is a redo. Before anything else, read why the last one ended: the latest rejection comment, Failure Report or give-up note, and any send-back note. Then build on what the earlier Claims left: find the Tickets they created on this Spec with `find_tickets`. Change them with `edit_ticket` and the Blocker operations, or `cancel` one with Replacements naming the Tickets that take its work over. A redo never adds a second batch. In the new finish summary, answer each point of the rejection or note.
