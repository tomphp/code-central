# Guide: map-chart

You hold a Claim on the Charting Ticket of a Map. The Map is the Spec this Ticket belongs to, labelled `wayfinder:map`: its body is the human's text, as they typed it. `get_stage_guide` with stage `map-chart` serves this guide again if you lose it.

## What this Ticket produces

Grill the human through Rounds until the Map's destination is clear, then chart the first decisions. The work lands in Code Central, never in the repository:

- **The Map's body.** Rewrite it with `edit_spec`, giving `body`: the human's text under the title, then these level-2 sections in this order, with these exact headings: `## Destination`, `## Notes`, `## Decisions so far` (left empty), `## Not yet specified` and `## Out of scope`.
- **The first decision Tickets.** `create_ticket` each on this Spec with one Wayfinder Label for its type: `wayfinder:grilling` for a decision to grill, `wayfinder:research` for a question to research, `wayfinder:task` for work to do. Leave grilling and task Tickets in `Draft`; give research Tickets `state` `Ready`. Order them with `add_blocker`. Each description says the decision it settles.
- **A resolution comment.** Post it on this Ticket with `comment`: the destination, the Tickets you created and why, and what is still fog.

Write nothing to the repository: no files, no commits and no branches.

## Questions

Every question goes to the human as a Round with `post_round`, never in the terminal. Put each question in a Question of its own, with your recommended answer. After each Round end your turn; `pick_up_answers` gives you the answers when the Ticket is Answered.

The In Review gate replaces any step that would stop to check with the user: the human reviews the Map when you finish, instead of checking with the user along the way. So a Round is for a real open question about the destination, never to approve the chart.

## Prototype this

When an answer starts with "Prototype this", hand that Question to a Prototype:

1. `create_ticket` with the `prototype` Label on the same Spec, with the Question and the human's note as its description, then `approve` it to `Ready`.
2. `add_blocker` so the Prototype blocks this Ticket.
3. `give_up` with a note naming the Prototype.

## Finishing

When the resolution comment is posted, `finish` with a summary that points at it. Give no `branch` or `commit`. The Ticket goes to In Review; stop working it.

## A redo

If this Ticket has earlier Claims, it is a redo. Before anything else, read why the last one ended: the latest rejection comment, Failure Report or give-up note, and any send-back note. Then build on what the earlier Claims left: their Rounds, the Map's body and the Tickets they created stand unless the human said otherwise. Read the Rounds with `get_ticket` and `parts: ["rounds"]`, and the Tickets with `find_tickets` on this Spec. Change them with `edit_ticket`, `edit_spec` and the Blocker operations rather than adding a second batch. In the new finish summary, answer each point of the rejection or note. If a Done Prototype blocks this Ticket, read its resolution comment and carry on from it.
