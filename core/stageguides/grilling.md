# Stage Guide: grilling

You hold a Claim on the grilling Stage of a Requirement. The Requirement is the Spec this Ticket belongs to: its body is the human's text, as they typed it. `get_stage_guide` with stage `grilling` serves this guide again if you lose it.

## What this Ticket produces

Grill the human on the Requirement until every decision, new term and open point is settled. The work lands in Code Central, never in the repository:

- **A new title.** Retitle the Spec with `edit_spec`, giving only `title`: a short name for the work, not the human's first line.
- **A resolution comment.** Post it on this Ticket with `comment`. It lists every decision with its reason, every new term with its meaning, every new ADR the work needs, and every open point left for the spec Stage. The Spec Stage writes the Spec from it, and the tickets Stage assigns writing the terms and ADRs to an implementation Ticket.
- Leave the Spec's body as the Requirement's text. The spec Stage replaces it.

If the Requirement is already clear, finish with no Round, and say so in the resolution comment, with the decisions it already makes.

A Stage never changes repository files and pushes no branch. Read the repository as much as you need, but write nothing to it, not even the glossary or an ADR: record those in the resolution comment.

## Questions

Every question goes to the human as a Round with `post_round`, never in the terminal. Put each question in a Question of its own, with your recommended answer. Ask in as few Rounds as you can, and after each one end your turn; `pick_up_answers` gives you the answers when the Ticket is Answered.

The In Review gate replaces any step that would stop to check with the user: the human reviews the resolution when you finish, instead of checking with the user along the way. So a Round is for a real open question only, never to confirm a summary or ask whether to carry on.

## Finishing

When the resolution comment is posted, `finish` with a short summary that points at the resolution comment: how many Rounds it took and the two or three decisions that matter most. Give no `branch` or `commit`: a Stage has none. The Ticket goes to In Review; stop working it.

## A redo

If this Ticket has earlier Claims, it is a redo. Before anything else, read why the last one ended: the latest rejection comment, Failure Report or give-up note, and any send-back note. Then build on what the earlier Claims left: their Rounds, answers and resolution comment stand unless the human said otherwise, so don't ask again what is already answered. Read their Rounds with `get_ticket` and `parts: ["rounds"]`: the Ticket `claim` returns leaves them out. Post a new resolution comment that replaces the old one, and answer each point of the rejection or note in the new finish summary.
