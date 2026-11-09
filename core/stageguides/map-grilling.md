# Guide: map-grilling

You hold a Claim on a grilling Ticket of a Map. The Map is the Spec this Ticket belongs to, labelled `wayfinder:map`; this Ticket's description names the one decision to settle. Read the Map's Destination and Decisions so far with `get_spec` and `sections`. `get_stage_guide` with stage `map-grilling` serves this guide again if you lose it.

## What this Ticket produces

Grill the human on this one decision until it is settled. The work lands in Code Central, never in the repository:

- **The Map's body.** Read it whole with `get_spec`, then `edit_spec` with `body`, changing only these sections: add one entry to `## Decisions so far`, a list item with the decision and a link to this Ticket; and update `## Not yet specified` and `## Out of scope` as they change. Keep the other sections and every heading as they are.
- **Follow-on Tickets.** If the decision opens new ones, `create_ticket` each on this Spec with one Wayfinder Label (`wayfinder:grilling`, `wayfinder:research` or `wayfinder:task`), grilling and task in `Draft`, research `Ready`, and order them with `add_blocker`.
- **A resolution comment.** Post it on this Ticket with `comment`: the decision with its reason, and any Tickets you created.

Write nothing to the repository: no files, no commits and no branches.

## Questions

Every question goes to the human as a Round with `post_round`, never in the terminal. Put each question in a Question of its own, with your recommended answer. After each Round end your turn; `pick_up_answers` gives you the answers when the Ticket is Answered.

The In Review gate replaces any step that would stop to check with the user: the human reviews the decision when you finish, instead of checking with the user along the way. So a Round is for a real open question only, never to confirm a summary.

## Prototype this

When an answer starts with "Prototype this", hand that Question to a Prototype:

1. `create_ticket` with the `prototype` Label on the same Spec, with the Question and the human's note as its description, then `approve` it to `Ready`.
2. `add_blocker` so the Prototype blocks this Ticket.
3. `give_up` with a note naming the Prototype.

## Finishing

When the resolution comment is posted, `finish` with a summary that states the decision and points at the resolution comment. Give no `branch` or `commit`. The Ticket goes to In Review; stop working it.

## A redo

If this Ticket has earlier Claims, it is a redo. Before anything else, read why the last one ended: the latest rejection comment, Failure Report or give-up note, and any send-back note. Then build on what the earlier Claims left: their Rounds and answers stand unless the human said otherwise, so don't ask again what is already answered. Read them with `get_ticket` and `parts: ["rounds"]`. Replace this Ticket's entry in Decisions so far rather than adding a second, and post a new resolution comment. In the new finish summary, answer each point of the rejection or note. If a Done Prototype blocks this Ticket, read its resolution comment and carry on from it.
